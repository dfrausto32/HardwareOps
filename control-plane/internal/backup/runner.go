package backup

import (
	"errors"
	"fmt"
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
	name     string
	applyCmd string
	workDir  string
	logDir   string
	logger   func(string, ...interface{})
	status   Status
	mode     string
	image    string
	certsDir string
	env      map[string]string
}

func NewRunner(name, applyCmd, workDir, logDir string, logger func(string, ...interface{})) *Runner {
	cmd := strings.TrimSpace(applyCmd)
	if cmd == "" {
		return nil
	}
	n := strings.TrimSpace(name)
	if n == "" {
		n = "backup"
	}
	r := &Runner{
		name:     n,
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
	Mode     string
	Image    string
	CertsDir string
}

func (r *Runner) Enabled() bool {
	return r != nil && strings.TrimSpace(r.applyCmd) != ""
}

func (r *Runner) Status() Status {
	if r == nil {
		return Status{Enabled: false, State: "disabled"}
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
		Mode:     r.mode,
		Image:    r.image,
		CertsDir: r.certsDir,
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

func (r *Runner) Start() (Status, error) {
	return r.start(nil)
}

func (r *Runner) StartWithEnv(extra map[string]string) (Status, error) {
	return r.start(extra)
}

func (r *Runner) start(extra map[string]string) (Status, error) {
	if r == nil || !r.Enabled() {
		return Status{Enabled: false, State: "disabled"}, errors.New("backup runner not configured")
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
		return r.status, errors.New("runner already running")
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
		r.status.RunnerContainer = fmt.Sprintf("hardwareops-%s-runner", r.name)
	} else {
		r.status.RunnerContainer = ""
	}
	statusSnapshot := r.status
	r.mu.Unlock()

	go r.run(statusSnapshot.LogPath, statusSnapshot.RunnerContainer, extra)
	return statusSnapshot, nil
}

func (r *Runner) nextLogPath(start time.Time) string {
	dir := r.logDir
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	filename := fmt.Sprintf("%s-%s.log", r.name, start.Format("20060102-150405"))
	return filepath.Join(dir, filename)
}

func (r *Runner) run(logPath string, runnerContainer string, extra map[string]string) {
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
	for k, v := range extra {
		env[k] = v
	}

	if r.logger != nil {
		if mode == "docker" {
			r.logger("%s start mode=docker image=%q log=%s", r.name, image, logPath)
		} else {
			r.logger("%s start cmd=%q log=%s", r.name, r.applyCmd, logPath)
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
				runErr = fmt.Errorf("start runner cmd: %w", err)
				exitCode = 1
			} else if err := cmd.Wait(); err != nil {
				runErr = fmt.Errorf("runner cmd failed: %w", err)
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
			r.logger("%s failed after %s: %v", r.name, finishedAt.Sub(startedAt), runErr)
		}
	} else {
		r.status.State = "success"
		r.status.Error = ""
		if r.logger != nil {
			r.logger("%s success after %s", r.name, finishedAt.Sub(startedAt))
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
		return nil, errors.New("backup runner image not configured")
	}
	stackHost := detectMountSource("/stack")
	if stackHost == "" {
		return nil, errors.New("stack mount not detected; ensure /stack is mounted into control-plane")
	}

	name := strings.TrimSpace(runnerContainer)
	if name == "" {
		name = "hardwareops-backup-runner"
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
		return errors.New("backup runner image not configured")
	}
	if strings.ContainsAny(img, "<>") {
		return fmt.Errorf("backup runner image invalid: %s", img)
	}
	if err := exec.Command("docker", "image", "inspect", img).Run(); err != nil {
		return fmt.Errorf("backup runner image not found/invalid: %s", img)
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
