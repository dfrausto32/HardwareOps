package planexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/parcel/agent/internal/plan"
)

type Context struct {
	WorkingDir string
	RenderRoot string
	Logger     *log.Logger
	Env        []string
}

type StepResult struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Error      string    `json:"error"`
}

const (
	statusSuccess = "success"
	statusFailed  = "error"
)

func Execute(p plan.Plan, ctx Context) ([]StepResult, error) {
	results := make([]StepResult, 0, len(p.Steps))
	for _, step := range p.Steps {
		res := StepResult{
			ID:        step.ID,
			Type:      step.Type,
			StartedAt: time.Now().UTC(),
		}

		err := executeStep(step, ctx)
		res.FinishedAt = time.Now().UTC()
		if err != nil {
			res.Status = statusFailed
			res.Error = err.Error()
			results = append(results, res)
			return results, err
		}

		res.Status = statusSuccess
		results = append(results, res)
	}
	return results, nil
}

func ExecuteHealth(h plan.Health, ctx Context) error {
	switch h.Type {
	case "probe.http":
		return healthProbeHTTP(h)
	default:
		return fmt.Errorf("unsupported health type %q", h.Type)
	}
}

func executeStep(step plan.Step, ctx Context) error {
	switch step.Type {
	case "file.copy":
		return stepFileCopy(step, ctx)
	case "file.render":
		return stepFileRender(step, ctx)
	case "symlink.switch":
		return stepSymlinkSwitch(step, ctx)
	case "probe.http":
		return stepProbeHTTP(step, ctx)
	case "script.preApply":
		return stepScriptPreApply(step, ctx)
	default:
		return fmt.Errorf("unsupported step type %q", step.Type)
	}
}

func stepFileCopy(step plan.Step, ctx Context) error {
	src, err := stringParam(step.Params, "src")
	if err != nil {
		return err
	}
	dest, err := stringParam(step.Params, "dest")
	if err != nil {
		return err
	}

	srcPath := resolvePath(ctx.WorkingDir, src)
	destPath := resolvePath(ctx.RenderRoot, dest)

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("src is not a file: %s", src)
	}

	mode := info.Mode().Perm()
	if paramMode, ok, err := intParam(step.Params, "mode"); err != nil {
		return err
	} else if ok {
		if paramMode < 0 {
			return fmt.Errorf("mode must be >= 0")
		}
		mode = os.FileMode(paramMode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func stepFileRender(step plan.Step, ctx Context) error {
	templatePath, err := stringParam(step.Params, "template")
	if err != nil {
		return err
	}
	dest, err := stringParam(step.Params, "dest")
	if err != nil {
		return err
	}

	srcPath := resolvePath(ctx.WorkingDir, templatePath)
	destPath := resolvePath(ctx.RenderRoot, dest)

	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	vars := map[string]any{}
	if raw, ok := step.Params["vars"]; ok {
		varMap, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("vars must be a map")
		}
		vars = varMap
	}

	tpl, err := template.New(filepath.Base(srcPath)).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return nil
}

func stepSymlinkSwitch(step plan.Step, ctx Context) error {
	link, err := stringParam(step.Params, "link")
	if err != nil {
		return err
	}
	target, err := stringParam(step.Params, "target")
	if err != nil {
		return err
	}

	linkPath := resolvePath(ctx.RenderRoot, link)
	targetPath := resolvePath(ctx.RenderRoot, target)

	tmpLink := linkPath + ".tmp"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(targetPath, tmpLink); err != nil {
		return err
	}
	return os.Rename(tmpLink, linkPath)
}

func stepProbeHTTP(step plan.Step, ctx Context) error {
	url, err := stringParam(step.Params, "url")
	if err != nil {
		return err
	}

	expectStatus := 200
	if v, ok, err := intParam(step.Params, "expectStatus"); err != nil {
		return err
	} else if ok {
		expectStatus = v
	}

	timeoutSec := 10
	if v, ok, err := intParam(step.Params, "timeoutSec"); err != nil {
		return err
	} else if ok {
		timeoutSec = v
	}
	if step.TimeoutSec != nil {
		timeoutSec = *step.TimeoutSec
	}
	if timeoutSec < 0 {
		return fmt.Errorf("timeoutSec must be >= 0")
	}
	return probeHTTP(url, expectStatus, time.Duration(timeoutSec)*time.Second)
}

func stepScriptPreApply(step plan.Step, ctx Context) error {
	command, err := stringParam(step.Params, "command")
	if err != nil {
		return err
	}
	commandPath := resolvePath(ctx.WorkingDir, command)

	args, err := stringListParam(step.Params, "args")
	if err != nil {
		return err
	}

	timeoutSec := 60
	if v, ok, err := intParam(step.Params, "timeoutSec"); err != nil {
		return err
	} else if ok {
		timeoutSec = v
	}
	if step.TimeoutSec != nil {
		timeoutSec = *step.TimeoutSec
	}
	if timeoutSec < 0 {
		return fmt.Errorf("timeoutSec must be >= 0")
	}

	ctxTimeout := context.Background()
	cancel := func() {}
	if timeoutSec > 0 {
		ctxTimeout, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	}
	defer cancel()

	// Reject commands with path traversal components.
	if strings.Contains(command, "..") {
		return fmt.Errorf("preApply command must not contain '..': %s", command)
	}
	// Reject absolute paths (commands must be relative, inside the artifact).
	if filepath.IsAbs(command) {
		return fmt.Errorf("preApply command must be a relative path: %s", command)
	}
	// Verify the resolved path is inside WorkingDir.
	absWorking, err := filepath.Abs(ctx.WorkingDir)
	if err != nil {
		return fmt.Errorf("resolve working dir: %w", err)
	}
	absCommand, err := filepath.Abs(commandPath)
	if err != nil {
		return fmt.Errorf("resolve command path: %w", err)
	}
	if !strings.HasPrefix(absCommand, absWorking+string(filepath.Separator)) {
		return fmt.Errorf("preApply command path escapes artifact directory: %s", command)
	}
	// Command must be a regular file, not a symlink or directory.
	info, err := os.Lstat(commandPath)
	if err != nil {
		return fmt.Errorf("stat preApply command: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("preApply command must be a regular file: %s", command)
	}

	cmd := exec.CommandContext(ctxTimeout, commandPath, args...)
	cmd.Dir = ctx.WorkingDir
	// Build a minimal base environment to prevent LD_PRELOAD, PATH, and similar
	// injection attacks via the parent process environment.
	baseEnv := []string{"PATH=/usr/bin:/bin:/usr/local/bin"}
	if home := os.Getenv("HOME"); home != "" {
		baseEnv = append(baseEnv, "HOME="+home)
	}
	if user := os.Getenv("USER"); user != "" {
		baseEnv = append(baseEnv, "USER="+user)
	}
	cmd.Env = append(baseEnv, ctx.Env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg != "" {
			return fmt.Errorf("preApply failed: %w: %s", err, msg)
		}
		return fmt.Errorf("preApply failed: %w", err)
	}
	return nil
}

func healthProbeHTTP(h plan.Health) error {
	expectStatus := 200
	if h.ExpectStatus != nil {
		expectStatus = *h.ExpectStatus
	}

	timeoutSec := 30
	if h.TimeoutSec != nil {
		timeoutSec = *h.TimeoutSec
	}
	intervalSec := 2
	if h.IntervalSec != nil {
		intervalSec = *h.IntervalSec
	}

	timeout := time.Duration(timeoutSec) * time.Second
	interval := time.Duration(intervalSec) * time.Second
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		remaining := time.Until(deadline)
		if timeout == 0 {
			remaining = 0
		}
		reqTimeout := remaining
		if reqTimeout <= 0 {
			reqTimeout = time.Second
		}

		if err := probeHTTP(h.URL, expectStatus, reqTimeout); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if timeout == 0 {
			break
		}
		remaining = time.Until(deadline)
		if remaining <= 0 {
			break
		}
		sleep := interval
		if sleep <= 0 {
			break
		}
		if sleep > remaining {
			sleep = remaining
		}
		time.Sleep(sleep)
	}

	if lastErr != nil {
		return fmt.Errorf("health check failed: %w", lastErr)
	}
	return fmt.Errorf("health check failed")
}

func stringListParam(params map[string]any, key string) ([]string, error) {
	val, ok := params[key]
	if !ok {
		return nil, nil
	}
	switch v := val.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("params.%s must be list of strings", key)
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []string{v}, nil
	default:
		return nil, fmt.Errorf("params.%s must be list of strings", key)
	}
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func stringParam(params map[string]any, key string) (string, error) {
	val, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing params.%s", key)
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("params.%s must be string", key)
	}
	if s == "" {
		return "", fmt.Errorf("params.%s empty", key)
	}
	return s, nil
}

func intParam(params map[string]any, key string) (int, bool, error) {
	val, ok := params[key]
	if !ok {
		return 0, false, nil
	}
	switch v := val.(type) {
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	case uint64:
		return int(v), true, nil
	case float64:
		return int(v), true, nil
	default:
		return 0, true, fmt.Errorf("params.%s must be number", key)
	}
}

func probeHTTP(url string, expectStatus int, timeout time.Duration) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectStatus {
		return fmt.Errorf("unexpected status: got %d want %d", resp.StatusCode, expectStatus)
	}
	return nil
}
