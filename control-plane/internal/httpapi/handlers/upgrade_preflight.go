package handlers

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/parcel/control-plane/internal/upgrade"
)

type PreflightCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type PreflightResponse struct {
	OK        bool             `json:"ok"`
	Timestamp time.Time        `json:"timestamp"`
	Checks    []PreflightCheck `json:"checks"`
}

func GetUpgradePreflight(runner *upgrade.Runner, updatesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := BuildUpgradePreflight(runner, updatesDir)
		writeJSON(w, resp)
	}
}

func BuildUpgradePreflight(runner *upgrade.Runner, updatesDir string) PreflightResponse {
	resp := PreflightResponse{Timestamp: time.Now().UTC()}
	if runner == nil || !runner.Enabled() {
		resp.Checks = append(resp.Checks, PreflightCheck{
			Name:    "Upgrade runner",
			Status:  "error",
			Message: "Upgrade runner not configured",
		})
		resp.OK = false
		return resp
	}

	status := runner.Status()
	runnerCfg := runner.Config()
	addCheck := func(name, status, msg string) {
		resp.Checks = append(resp.Checks, PreflightCheck{Name: name, Status: status, Message: msg})
	}

	if runnerCfg.Mode != "" {
		addCheck("Runner mode", "ok", runnerCfg.Mode)
	}
	if strings.EqualFold(runnerCfg.Mode, "remote") {
		if runnerCfg.RemoteURL == "" {
			addCheck("Remote runner URL", "error", "UPGRADE_RUNNER_URL not set")
		} else if _, err := url.ParseRequestURI(runnerCfg.RemoteURL); err != nil {
			addCheck("Remote runner URL", "error", "invalid URL")
		} else {
			addCheck("Remote runner URL", "ok", runnerCfg.RemoteURL)
			if status.State == "failed" && strings.TrimSpace(status.Error) != "" {
				addCheck("Remote runner connectivity", "error", status.Error)
			} else {
				addCheck("Remote runner connectivity", "ok", "reachable")
			}
		}
	}
	if strings.EqualFold(runnerCfg.Mode, "docker") {
		if _, err := exec.LookPath("docker"); err != nil {
			addCheck("Docker CLI", "error", "docker not found in PATH")
		} else {
			addCheck("Docker CLI", "ok", "docker available")
		}
		if err := dockerDaemonOK(); err != nil {
			addCheck("Docker daemon", "error", err.Error())
		} else {
			addCheck("Docker daemon", "ok", "reachable")
		}
		minVer := strings.TrimSpace(os.Getenv("MIN_DOCKER_API"))
		if minVer == "" {
			minVer = "1.44"
		}
		if ver, err := dockerClientAPIVersion(); err != nil {
			addCheck("Docker API", "warn", "unable to detect")
		} else if compareVersions(ver, minVer) < 0 {
			addCheck("Docker API", "error", fmt.Sprintf("client %s < required %s", ver, minVer))
		} else {
			addCheck("Docker API", "ok", ver)
		}

		if runnerCfg.Image == "" {
			if detected := detectSelfImage(); detected != "" {
				addCheck("Runner image", "ok", detected)
			} else {
				addCheck("Runner image", "error", "UPGRADE_RUNNER_IMAGE not set and auto-detect failed")
			}
		} else {
			addCheck("Runner image", "ok", runnerCfg.Image)
		}
		if img := firstNonEmpty(runnerCfg.Image, detectSelfImage()); img != "" {
			if err := dockerImageInspect(img); err != nil {
				addCheck("Runner image inspect", "error", err.Error())
			} else {
				addCheck("Runner image inspect", "ok", "image available")
			}
		}
		if src := detectMountSource("/stack"); src == "" {
			addCheck("Stack mount", "error", "stack mount not detected; ensure /stack is mounted into control-plane")
		} else {
			addCheck("Stack mount", "ok", src)
		}
		envCandidates := []string{
			"/stack/.env.onprem",
			"/stack/.env.onprem.example",
			"/stack/control-plane.env",
			"/stack/deploy/compose/.env.onprem.example",
		}
		foundEnv := ""
		for _, candidate := range envCandidates {
			if fileExists(candidate) {
				foundEnv = candidate
				break
			}
		}
		if foundEnv == "" {
			addCheck("Env file", "error", "No env file found in /stack")
		} else {
			addCheck("Env file", "ok", foundEnv)
		}
	}

	if !strings.EqualFold(runnerCfg.Mode, "remote") {
		if status.Command == "" {
			addCheck("Apply command", "error", "UPGRADE_APPLY_CMD not set")
		} else {
			addCheck("Apply command", "ok", status.Command)
		}
		if status.WorkingDir != "" {
			if stat, err := os.Stat(status.WorkingDir); err != nil || !stat.IsDir() {
				addCheck("Working dir", "error", "Invalid working dir")
			} else {
				addCheck("Working dir", "ok", status.WorkingDir)
			}
		} else {
			addCheck("Working dir", "warn", "No working dir set")
		}
	}

	if updatesDir != "" {
		if stat, err := os.Stat(updatesDir); err != nil || !stat.IsDir() {
			addCheck("Updates dir", "error", "Missing updates dir")
		} else {
			pattern := filepath.Join(updatesDir, "parcel-upgrade-*.tar.gz")
			matches, _ := filepath.Glob(pattern)
			if len(matches) == 0 {
				addCheck("Upgrade bundle", "warn", "No upgrade bundles found")
			} else {
				addCheck("Upgrade bundle", "ok", strings.Join(matches, ", "))
				if ok, missing, err := checkBundleContents(matches[0]); err != nil {
					addCheck("Bundle contents", "warn", err.Error())
				} else if !ok {
					addCheck("Bundle contents", "error", "Missing required files: "+strings.Join(missing, ", "))
				} else {
					addCheck("Bundle contents", "ok", "compose + env present")
				}
			}
		}
	} else {
		addCheck("Updates dir", "warn", "UPGRADE_UPDATES_DIR not set")
	}

	if strings.EqualFold(runnerCfg.Mode, "docker") {
		if sock := "/var/run/docker.sock"; fileExists(sock) {
			addCheck("Docker socket", "ok", sock)
		} else {
			addCheck("Docker socket", "warn", "Docker socket not mounted")
		}
	}

	if dir := firstNonEmpty(status.WorkingDir, updatesDir); dir != "" {
		if free, err := freeSpace(dir); err == nil {
			if free < 2*1024*1024*1024 {
				addCheck("Disk space", "warn", "Low free space (<2GB)")
			} else {
				addCheck("Disk space", "ok", "Sufficient free space")
			}
		}
	}

	addCheck("Platform", "ok", runtime.GOOS+"/"+runtime.GOARCH)

	resp.OK = true
	for _, check := range resp.Checks {
		if check.Status == "error" {
			resp.OK = false
			break
		}
	}
	return resp
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dockerDaemonOK() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return errors.New("docker daemon not reachable")
	}
	return nil
}

func dockerClientAPIVersion() (string, error) {
	out, err := exec.Command("docker", "version", "--format", "{{.Client.APIVersion}}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for len(as) < len(bs) {
		as = append(as, "0")
	}
	for len(bs) < len(as) {
		bs = append(bs, "0")
	}
	for i := 0; i < len(as); i++ {
		ai := parseVersionPart(strings.TrimSpace(as[i]))
		bi := parseVersionPart(strings.TrimSpace(bs[i]))
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func parseVersionPart(raw string) int {
	if raw == "" {
		return 0
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func dockerImageInspect(image string) error {
	if strings.TrimSpace(image) == "" {
		return errors.New("image not set")
	}
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		return fmt.Errorf("image not found: %s", image)
	}
	return nil
}

func detectSelfImage() string {
	container := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if container == "" {
		return ""
	}
	out, err := exec.Command("docker", "inspect", container, "--format", "{{.Config.Image}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectMountSource(dest string) string {
	container := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if container == "" {
		return ""
	}
	format := fmt.Sprintf("{{range .Mounts}}{{if eq .Destination %q}}{{.Source}}{{end}}{{end}}", dest)
	out, err := exec.Command("docker", "inspect", container, "--format", format).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func checkBundleContents(path string) (bool, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return false, nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	required := map[string]bool{
		"docker-compose.onprem.bundle.yml": false,
		".env.onprem.example":              false,
	}
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return false, nil, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		for req := range required {
			if strings.HasSuffix(name, "/"+req) || name == req {
				required[req] = true
			}
		}
	}
	missing := []string{}
	for k, ok := range required {
		if !ok {
			missing = append(missing, k)
		}
	}
	return len(missing) == 0, missing, nil
}

func freeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
