package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

type fakeWorkloadIdentityExchanger struct {
	session auth.WorkloadIdentitySession
	err     error
}

func (f fakeWorkloadIdentityExchanger) Exchange(ctx context.Context, provider, rawToken string, requestedScopes []string) (auth.WorkloadIdentitySession, error) {
	if f.err != nil {
		return auth.WorkloadIdentitySession{}, f.err
	}
	return f.session, nil
}

func TestExchangeWorkloadIdentityToken(t *testing.T) {
	mem := memory.New()
	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", time.Hour, "hardwareops", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	logger := log.New(io.Discard, "", 0)
	session := auth.WorkloadIdentitySession{
		Provider:  "github-actions",
		Subject:   "repo:example/app:ref:refs/heads/main",
		Name:      "example/app@refs/heads/main",
		Scopes:    []string{"artifact.publish"},
		Metadata:  map[string]string{"repository": "example/app"},
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	body, _ := json.Marshal(WorkloadIdentityExchangeRequest{
		Provider: "github-actions",
		IDToken:  "oidc-token",
		Scopes:   []string{"artifact.publish"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/workload-identity/exchange", bytes.NewReader(body))
	ExchangeWorkloadIdentityToken(logger, manager, fakeWorkloadIdentityExchanger{session: session}, mem, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp WorkloadIdentityExchangeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected token in response")
	}
	events, err := mem.ListAuditEvents(store.AuditEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 || events[0].Action != "auth.workload_identity.exchange" {
		t.Fatalf("expected workload identity exchange audit event, got %+v", events)
	}
}

func TestExchangeWorkloadIdentityTokenFailure(t *testing.T) {
	mem := memory.New()
	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", time.Hour, "hardwareops", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	logger := log.New(io.Discard, "", 0)
	body, _ := json.Marshal(WorkloadIdentityExchangeRequest{
		Provider: "github-actions",
		IDToken:  "oidc-token",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/workload-identity/exchange", bytes.NewReader(body))
	ExchangeWorkloadIdentityToken(logger, manager, fakeWorkloadIdentityExchanger{err: errors.New("claim mismatch")}, mem, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
