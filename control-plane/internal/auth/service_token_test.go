package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestServiceTokenFromToken(t *testing.T) {
	mem := memory.New()
	manager, err := NewManager(ModeLocal, "test-secret", time.Hour, "test-issuer", mem)
	if err != nil {
		t.Fatalf("new auth manager: %v", err)
	}

	now := time.Now().UTC()
	testCases := []struct {
		name      string
		scopes    []string
		expiresAt time.Time
		revoked   bool
		wantOK    bool
	}{
		{
			name:      "active token normalizes scopes",
			scopes:    []string{"ARTIFACT.PUBLISH", "artifact.publish", ""},
			expiresAt: now.Add(time.Hour),
			wantOK:    true,
		},
		{
			name:      "revoked token rejected",
			scopes:    []string{"artifact.publish"},
			expiresAt: now.Add(time.Hour),
			revoked:   true,
			wantOK:    false,
		},
		{
			name:      "expired token rejected",
			scopes:    []string{"artifact.publish"},
			expiresAt: now.Add(-time.Hour),
			wantOK:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rawToken, tokenHash, err := GenerateVoucherToken()
			if err != nil {
				t.Fatalf("generate service token: %v", err)
			}

			scopesJSON, err := json.Marshal(tc.scopes)
			if err != nil {
				t.Fatalf("marshal scopes: %v", err)
			}

			record := store.ServiceToken{
				TokenID:    uuid.NewString(),
				Name:       "svc",
				TokenHash:  tokenHash,
				ScopesJSON: scopesJSON,
				ExpiresAt:  tc.expiresAt,
				CreatedAt:  now,
			}
			if tc.revoked {
				record.RevokedAt = now
				record.RevokedBy = "test"
			}
			if err := mem.CreateServiceToken(record); err != nil {
				t.Fatalf("create service token: %v", err)
			}

			got, ok, err := manager.serviceTokenFromToken(rawToken)
			if err != nil {
				t.Fatalf("serviceTokenFromToken: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%t, got %t", tc.wantOK, ok)
			}
			if !tc.wantOK {
				return
			}
			if got.AuthMethod != "service_token" {
				t.Fatalf("expected auth_method service_token, got %q", got.AuthMethod)
			}
			if !HasScope(got.Scopes, "artifact.publish") {
				t.Fatalf("expected artifact.publish scope in %v", got.Scopes)
			}
			if len(got.Scopes) != 1 || got.Scopes[0] != "artifact.publish" {
				t.Fatalf("expected normalized scopes, got %v", got.Scopes)
			}
		})
	}
}
