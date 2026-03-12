package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestIssueAndParseWorkloadIdentityToken(t *testing.T) {
	manager, err := NewManager(ModeLocal, "0123456789abcdef0123456789abcdef", time.Hour, "hardwareops-test", memory.New())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	token, exp, err := manager.IssueWorkloadIdentityToken(WorkloadIdentitySession{
		Provider:  "github-actions",
		Subject:   "repo:example/app:ref:refs/heads/main",
		Name:      "example/app@refs/heads/main",
		Scopes:    []string{"artifact.publish"},
		Metadata:  map[string]string{"repository": "example/app"},
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("issue workload token: %v", err)
	}
	if exp.IsZero() {
		t.Fatal("expected expiry")
	}
	got, ok, err := manager.workloadTokenFromToken(token)
	if err != nil {
		t.Fatalf("parse workload token: %v", err)
	}
	if !ok {
		t.Fatal("expected workload token to parse")
	}
	if got.AuthMethod != "workload_identity" {
		t.Fatalf("expected workload_identity auth method, got %q", got.AuthMethod)
	}
	if !HasScope(got.Scopes, "artifact.publish") {
		t.Fatalf("expected artifact.publish scope, got %v", got.Scopes)
	}
}

func TestMiddlewareAcceptsWorkloadIdentityToken(t *testing.T) {
	manager, err := NewManager(ModeLocal, "0123456789abcdef0123456789abcdef", time.Hour, "hardwareops-test", memory.New())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	token, _, err := manager.IssueWorkloadIdentityToken(WorkloadIdentitySession{
		Provider:  "github-actions",
		Subject:   "repo:example/app:ref:refs/heads/main",
		Name:      "example/app@refs/heads/main",
		Scopes:    []string{"artifact.publish"},
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("issue workload token: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc, ok := ServiceTokenFromContext(r.Context())
		if !ok {
			t.Fatal("expected workload identity service token in context")
		}
		if svc.AuthMethod != "workload_identity" {
			t.Fatalf("expected workload_identity auth method, got %q", svc.AuthMethod)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}
