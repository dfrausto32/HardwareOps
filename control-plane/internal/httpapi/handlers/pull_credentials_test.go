package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hardwareops/control-plane/internal/artifactingest"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestGetPullCredentialStatus(t *testing.T) {
	mgr, err := artifactingest.NewPullCredentialManager("", `{"repo-a":{"authorization":"Bearer test"}}`, "", "", "", "", "")
	if err != nil {
		t.Fatalf("new pull credential manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/pull-credentials", nil)
	w := httptest.NewRecorder()
	GetPullCredentialStatus(log.New(io.Discard, "", 0), mgr).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var status artifactingest.PullCredentialStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.Configured || !status.ResolverAvailable {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.CredentialRefCount != 1 || len(status.CredentialRefs) != 1 || status.CredentialRefs[0] != "repo-a" {
		t.Fatalf("unexpected refs: %+v", status)
	}
}

func TestReloadPullCredentials(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	st := memory.New()

	dir := t.TempDir()
	path := filepath.Join(dir, "pull-creds.json")
	if err := os.WriteFile(path, []byte(`{"repo-a":{"authorization":"Bearer old"}}`), 0o600); err != nil {
		t.Fatalf("write creds file: %v", err)
	}
	mgr, err := artifactingest.NewPullCredentialManager(path, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("new pull credential manager: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"repo-b":{"authorization":"Bearer new"}}`), 0o600); err != nil {
		t.Fatalf("update creds file: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull-credentials/reload", nil)
	w := httptest.NewRecorder()
	ReloadPullCredentials(logger, st, mgr, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var status artifactingest.PullCredentialStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.CredentialRefCount != 1 || len(status.CredentialRefs) != 1 || status.CredentialRefs[0] != "repo-b" {
		t.Fatalf("unexpected refs after reload: %+v", status)
	}

	events, err := st.ListAuditEvents(store.AuditEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Action == "artifact.pull_credentials.reload" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact.pull_credentials.reload audit event")
	}
}
