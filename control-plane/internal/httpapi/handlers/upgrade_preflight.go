package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hardwareops/control-plane/internal/upgrade"
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
	if strings.EqualFold(runnerCfg.Mode, "docker") {
		if runnerCfg.Image == "" {
			addCheck("Runner image", "warn", "UPGRADE_RUNNER_IMAGE not set (will attempt auto-detect)")
		} else {
			addCheck("Runner image", "ok", runnerCfg.Image)
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

	if updatesDir != "" {
		if stat, err := os.Stat(updatesDir); err != nil || !stat.IsDir() {
			addCheck("Updates dir", "error", "Missing updates dir")
		} else {
			pattern := filepath.Join(updatesDir, "hardwareops-upgrade-*.tar.gz")
			matches, _ := filepath.Glob(pattern)
			if len(matches) == 0 {
				addCheck("Upgrade bundle", "warn", "No upgrade bundles found")
			} else {
				addCheck("Upgrade bundle", "ok", strings.Join(matches, ", "))
			}
		}
	} else {
		addCheck("Updates dir", "warn", "UPGRADE_UPDATES_DIR not set")
	}

	if sock := "/var/run/docker.sock"; fileExists(sock) {
		addCheck("Docker socket", "ok", sock)
	} else {
		addCheck("Docker socket", "warn", "Docker socket not mounted")
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
