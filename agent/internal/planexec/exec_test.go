package planexec

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hardwareops/agent/internal/plan"
)

func TestExecuteOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := plan.Plan{
		Version: "v1",
		Steps: []plan.Step{
			{
				ID:     "copy-1",
				Type:   "file.copy",
				OnFail: "abort",
				Params: map[string]any{"src": "a.txt", "dest": "out/a.txt"},
			},
			{
				ID:     "copy-2",
				Type:   "file.copy",
				OnFail: "abort",
				Params: map[string]any{"src": "a.txt", "dest": "out/b.txt"},
			},
		},
	}

	results, err := Execute(p, Context{WorkingDir: root, RenderRoot: root})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "copy-1" || results[1].ID != "copy-2" {
		t.Fatalf("unexpected order: %+v", results)
	}
}

func TestFileCopyAndSymlinkSwitch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dir", "target.txt"), []byte("t"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	p := plan.Plan{
		Version: "v1",
		Steps: []plan.Step{
			{
				ID:     "copy",
				Type:   "file.copy",
				OnFail: "abort",
				Params: map[string]any{"src": "src.txt", "dest": "dest.txt"},
			},
			{
				ID:     "link",
				Type:   "symlink.switch",
				OnFail: "abort",
				Params: map[string]any{"link": "current", "target": "dir"},
			},
		},
	}

	_, err := Execute(p, Context{WorkingDir: root, RenderRoot: root})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "dest.txt")); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}

	link, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if link != filepath.Join(root, "dir") {
		t.Fatalf("unexpected symlink target: %s", link)
	}
}

func TestFailingStepStops(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := plan.Plan{
		Version: "v1",
		Steps: []plan.Step{
			{
				ID:     "copy",
				Type:   "file.copy",
				OnFail: "abort",
				Params: map[string]any{"src": "src.txt", "dest": "dest.txt"},
			},
			{
				ID:     "probe",
				Type:   "probe.http",
				OnFail: "abort",
				Params: map[string]any{"url": "http://127.0.0.1:1"},
			},
			{
				ID:     "copy-2",
				Type:   "file.copy",
				OnFail: "abort",
				Params: map[string]any{"src": "src.txt", "dest": "dest2.txt"},
			},
		},
	}

	results, err := Execute(p, Context{WorkingDir: root, RenderRoot: root})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if _, err := os.Stat(filepath.Join(root, "dest2.txt")); err == nil {
		t.Fatalf("expected dest2.txt to not exist")
	}
}

func TestProbeHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := plan.Plan{
		Version: "v1",
		Steps: []plan.Step{
			{
				ID:     "probe",
				Type:   "probe.http",
				OnFail: "abort",
				Params: map[string]any{"url": server.URL, "timeoutSec": 1},
			},
		},
	}

	_, err := Execute(p, Context{WorkingDir: t.TempDir(), RenderRoot: t.TempDir()})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestScriptPreApply(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "preapply.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env sh\nset -e\nprintf \"%s\" \"$HWOPS_ARTIFACT_VERSION\" > preapply.out\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	p := plan.Plan{
		Version: "v1",
		Steps: []plan.Step{
			{
				ID:     "pre",
				Type:   "script.preApply",
				OnFail: "abort",
				Params: map[string]any{"command": "preapply.sh"},
			},
		},
	}

	_, err := Execute(p, Context{
		WorkingDir: root,
		RenderRoot: root,
		Env:        []string{"HWOPS_ARTIFACT_VERSION=1.2.3"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "preapply.out"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "1.2.3" {
		t.Fatalf("unexpected output: %s", string(data))
	}
}
