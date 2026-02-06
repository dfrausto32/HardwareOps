package upgrade

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
	Enabled     bool      `json:"enabled"`
	Running     bool      `json:"running"`
	State       string    `json:"state"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	FinishedAt  time.Time `json:"finishedAt,omitempty"`
	ExitCode    int       `json:"exitCode,omitempty"`
	Error       string    `json:"error,omitempty"`
	LogPath     string    `json:"logPath,omitempty"`
	Command     string    `json:"command,omitempty"`
	WorkingDir  string    `json:"workingDir,omitempty"`
	TriggeredAt time.Time `json:"triggeredAt,omitempty"`
}

type Runner struct {
	mu       sync.RWMutex
	applyCmd string
	workDir  string
	logDir   string
	logger   func(string, ...interface{})
	status   Status
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

func (r *Runner) Start() (Status, error) {
	if r == nil || !r.Enabled() {
		return Status{Enabled: false, State: "disabled"}, errors.New("upgrade runner not configured")
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
	statusSnapshot := r.status
	r.mu.Unlock()

	go r.run(statusSnapshot.LogPath)
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

func (r *Runner) run(logPath string) {
	startedAt := time.Now().UTC()
	if r.logger != nil {
		r.logger("upgrade apply start cmd=%q log=%s", r.applyCmd, logPath)
	}

	exitCode := 0
	runErr := error(nil)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		runErr = fmt.Errorf("open log file: %w", err)
		exitCode = 1
	} else {
		defer logFile.Close()
		cmd := exec.Command("/bin/sh", "-c", r.applyCmd)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if r.workDir != "" {
			cmd.Dir = r.workDir
		}
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
