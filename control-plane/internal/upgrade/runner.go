package upgrade

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Enabled         bool      `json:"enabled"`
	Running         bool      `json:"running"`
	State           string    `json:"state"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
	ExitCode        int       `json:"exitCode,omitempty"`
	Error           string    `json:"error,omitempty"`
	LogPath         string    `json:"logPath,omitempty"`
	Command         string    `json:"command,omitempty"`
	WorkingDir      string    `json:"workingDir,omitempty"`
	TriggeredAt     time.Time `json:"triggeredAt,omitempty"`
	RunnerContainer string    `json:"runnerContainer,omitempty"`
}

type Runner struct {
	mu       sync.RWMutex
	applyCmd string
	workDir  string
	logDir   string
	logger   func(string, ...interface{})
	status   Status
	mode     string
	image    string
	certsDir string
	env      map[string]string
	remote   remoteConfig
}

type remoteConfig struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewRunner(applyCmd, workDir, logDir string, logger func(string, ...interface{})) *Runner {
	cmd := strings.TrimSpace(applyCmd)
	if cmd == "" {
		return nil
	}
	r := &Runner{
		applyCmd: cmd,
		workDir:  strings.TrimSpace(workDir),
		logDir:   strings.TrimSpace(logDir),
		logger:   logger,
		mode:     "local",
	}
	r.status = Status{
		Enabled:    true,
		Running:    false,
		State:      "idle",
		Command:    cmd,
		WorkingDir: r.workDir,
	}
	return r
}

type RunnerConfig struct {
	Mode      string
	Image     string
	CertsDir  string
	RemoteURL string
}

func (r *Runner) Enabled() bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(r.mode, "remote") {
		return strings.TrimSpace(r.remote.baseURL) != ""
	}
	return strings.TrimSpace(r.applyCmd) != ""
}

func (r *Runner) Status() Status {
	if r == nil {
		return Status{Enabled: false, State: "disabled"}
	}
	if strings.EqualFold(r.mode, "remote") {
		if status, err := r.remoteStatus(); err == nil {
			r.mu.Lock()
			r.status = status
			r.mu.Unlock()
			return status
		} else {
			r.mu.Lock()
			r.status.Enabled = true
			r.status.Running = false
			r.status.State = "failed"
			r.status.Error = err.Error()
			r.status.FinishedAt = time.Now().UTC()
			status := r.status
			r.mu.Unlock()
			return status
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *Runner) Config() RunnerConfig {
	if r == nil {
		return RunnerConfig{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RunnerConfig{
		Mode:      r.mode,
		Image:     r.image,
		CertsDir:  r.certsDir,
		RemoteURL: r.remote.baseURL,
	}
}

func (r *Runner) ConfigureDocker(image, certsDir string, env map[string]string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mode = "docker"
	img := strings.TrimSpace(image)
	if img == "" {
		img = detectSelfImage()
	}
	r.image = strings.TrimSpace(img)
	r.certsDir = strings.TrimSpace(certsDir)
	r.env = env
}

func (r *Runner) ConfigureRemote(baseURL, token string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mode = "remote"
	r.remote.baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	r.remote.token = strings.TrimSpace(token)
	r.remote.client = &http.Client{Timeout: 15 * time.Second}
	r.status.Enabled = r.remote.baseURL != ""
	r.status.State = "idle"
	r.status.Command = "remote"
	r.status.WorkingDir = ""
	r.status.RunnerContainer = "maintenance-runner"
}

func (r *Runner) Start() (Status, error) {
	if r == nil || !r.Enabled() {
		return Status{Enabled: false, State: "disabled"}, errors.New("upgrade runner not configured")
	}
	if strings.EqualFold(r.mode, "remote") {
		status, err := r.remoteStart()
		if err != nil {
			r.mu.Lock()
			r.status.Enabled = true
			r.status.Running = false
			r.status.State = "failed"
			r.status.Error = err.Error()
			r.status.ExitCode = 1
			r.status.FinishedAt = time.Now().UTC()
			status = r.status
			r.mu.Unlock()
			return status, err
		}
		r.mu.Lock()
		r.status = status
		r.mu.Unlock()
		return status, nil
	}
	if strings.EqualFold(r.mode, "docker") {
		if err := r.validateDockerInputs(); err != nil {
			r.mu.Lock()
			r.status.Enabled = true
			r.status.Running = false
			r.status.State = "failed"
			r.status.Error = err.Error()
			r.status.ExitCode = 1
			r.status.FinishedAt = time.Now().UTC()
			status := r.status
			r.mu.Unlock()
			return status, err
		}
	}
	r.mu.Lock()
	if r.status.Running {
		defer r.mu.Unlock()
		return r.status, errors.New("upgrade already running")
	}
	now := time.Now().UTC()
	r.status.Enabled = true
	r.status.Running = true
	r.status.State = "running"
	r.status.Error = ""
	r.status.ExitCode = 0
	r.status.StartedAt = now
	r.status.TriggeredAt = now
	r.status.FinishedAt = time.Time{}
	r.status.LogPath = r.nextLogPath(now)
	r.status.Command = r.applyCmd
	r.status.WorkingDir = r.workDir
	if strings.EqualFold(r.mode, "docker") {
		r.status.RunnerContainer = "parcel-upgrade-runner"
	} else {
		r.status.RunnerContainer = ""
	}
	statusSnapshot := r.status
	r.mu.Unlock()

	go r.run(statusSnapshot.LogPath, statusSnapshot.RunnerContainer)
	return statusSnapshot, nil
}

func (r *Runner) nextLogPath(start time.Time) string {
	dir := r.logDir
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	filename := fmt.Sprintf("upgrade-%s.log", start.Format("20060102-150405"))
	return filepath.Join(dir, filename)
}

func (r *Runner) run(logPath string, runnerContainer string) {
	startedAt := time.Now().UTC()
	mode := "local"
	image := ""
	certsDir := ""
	env := map[string]string{}
	r.mu.RLock()
	if r.mode != "" {
		mode = r.mode
	}
	image = r.image
	certsDir = r.certsDir
	for k, v := range r.env {
		env[k] = v
	}
	r.mu.RUnlock()

	if r.logger != nil {
		if mode == "docker" {
			r.logger("upgrade apply start mode=docker image=%q log=%s", image, logPath)
		} else {
			r.logger("upgrade apply start cmd=%q log=%s", r.applyCmd, logPath)
		}
	}

	exitCode := 0
	runErr := error(nil)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		runErr = fmt.Errorf("open log file: %w", err)
		exitCode = 1
	} else {
		defer logFile.Close()
		var cmd *exec.Cmd
		if mode == "docker" {
			cmd, runErr = buildDockerCmd(r.applyCmd, image, certsDir, runnerContainer, env)
		} else {
			cmd = exec.Command("/bin/sh", "-c", r.applyCmd)
			if r.workDir != "" {
				cmd.Dir = r.workDir
			}
		}
		if runErr == nil && cmd != nil {
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			if err := cmd.Start(); err != nil {
				runErr = fmt.Errorf("start upgrade cmd: %w", err)
				exitCode = 1
			} else if err := cmd.Wait(); err != nil {
				runErr = fmt.Errorf("upgrade cmd failed: %w", err)
				if cmd.ProcessState != nil {
					exitCode = cmd.ProcessState.ExitCode()
				} else {
					exitCode = 1
				}
			} else if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		} else if runErr != nil {
			exitCode = 1
		}
	}

	finishedAt := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Running = false
	r.status.FinishedAt = finishedAt
	r.status.ExitCode = exitCode
	if runErr != nil {
		r.status.State = "failed"
		r.status.Error = runErr.Error()
		if r.logger != nil {
			r.logger("upgrade apply failed after %s: %v", finishedAt.Sub(startedAt), runErr)
		}
	} else {
		r.status.State = "success"
		r.status.Error = ""
		if r.logger != nil {
			r.logger("upgrade apply success after %s", finishedAt.Sub(startedAt))
		}
	}
}

func buildDockerCmd(applyCmd, image, certsDir, runnerContainer string, env map[string]string) (*exec.Cmd, error) {
	img := strings.TrimSpace(image)
	if img == "" {
		if detected := detectSelfImage(); detected != "" {
			img = detected
		}
	}
	if img == "" {
		return nil, errors.New("upgrade runner image not configured")
	}
	stackHost := detectMountSource("/stack")
	if stackHost == "" {
		return nil, errors.New("stack mount not detected; ensure /stack is mounted into control-plane")
	}

	name := strings.TrimSpace(runnerContainer)
	if name == "" {
		name = "parcel-upgrade-runner"
	}
	args := []string{
		"run",
		"--rm",
		"--name", name,
		"--entrypoint", "/bin/sh",
		"-w", "/stack",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", fmt.Sprintf("%s:/stack", stackHost),
	}
	if certsDir = strings.TrimSpace(certsDir); certsDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/certs:ro", certsDir))
	}
	for key, val := range env {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, val))
	}
	args = append(args, img, "-c", applyCmd)
	cmd := exec.Command("docker", args...)
	return cmd, nil
}

func (r *Runner) validateDockerInputs() error {
	stackHost := detectMountSource("/stack")
	if stackHost == "" {
		return errors.New("stack mount not detected; ensure /stack is mounted into control-plane")
	}
	img := strings.TrimSpace(r.image)
	if img == "" {
		img = detectSelfImage()
	}
	if img == "" {
		return errors.New("upgrade runner image not configured")
	}
	if strings.ContainsAny(img, "<>") {
		return fmt.Errorf("upgrade runner image invalid: %s", img)
	}
	if err := exec.Command("docker", "image", "inspect", img).Run(); err != nil {
		return fmt.Errorf("upgrade runner image not found/invalid: %s", img)
	}
	updatesDir := strings.TrimSpace(os.Getenv("UPGRADE_UPDATES_DIR"))
	if updatesDir == "" {
		updatesDir = "/stack/updates"
	}
	if stat, err := os.Stat(updatesDir); err != nil || !stat.IsDir() {
		return fmt.Errorf("updates dir missing: %s", updatesDir)
	}
	matches, _ := filepath.Glob(filepath.Join(updatesDir, "parcel-upgrade-*.tar.gz"))
	if len(matches) == 0 {
		return fmt.Errorf("no upgrade bundle found in %s", updatesDir)
	}
	envCandidates := []string{
		"/stack/.env.onprem",
		"/stack/.env.onprem.example",
		"/stack/control-plane.env",
		"/stack/deploy/compose/.env.onprem.example",
	}
	for _, candidate := range envCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return nil
		}
	}
	return errors.New("no env file found in /stack (expected .env.onprem or control-plane.env)")
}

func (r *Runner) remoteStart() (Status, error) {
	if r == nil {
		return Status{}, errors.New("upgrade runner not configured")
	}
	r.mu.RLock()
	baseURL := strings.TrimSpace(r.remote.baseURL)
	token := r.remote.token
	client := r.remote.client
	r.mu.RUnlock()
	if baseURL == "" {
		return Status{}, errors.New("upgrade remote runner URL not configured")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/upgrade/start", bytes.NewReader([]byte("{}")))
	if err != nil {
		return Status{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return Status{}, errors.New(msg)
	}
	var status Status
	if err := json.Unmarshal(body, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (r *Runner) remoteStatus() (Status, error) {
	if r == nil {
		return Status{}, errors.New("upgrade runner not configured")
	}
	r.mu.RLock()
	baseURL := strings.TrimSpace(r.remote.baseURL)
	token := r.remote.token
	client := r.remote.client
	r.mu.RUnlock()
	if baseURL == "" {
		return Status{}, errors.New("upgrade remote runner URL not configured")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return Status{}, fmt.Errorf("invalid remote runner URL: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/upgrade/status", nil)
	if err != nil {
		return Status{}, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return Status{}, errors.New(msg)
	}
	var status Status
	if err := json.Unmarshal(body, &status); err != nil {
		return Status{}, err
	}
	return status, nil
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
