package handlers

import (
	"bytes"
	"encoding/json"
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

func TestRecoveryCodesGenerateAndResetPassword(t *testing.T) {
	mem := memory.New()
	passwordHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := store.User{
		UserID:       "user-1",
		Email:        "admin@example.com",
		PasswordHash: passwordHash,
		RolesJSON:    []byte(`["admin"]`),
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	}
	if err := mem.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "hardwareops", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	token, _, err := manager.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	logger := log.New(io.Discard, "", 0)

	generate := manager.Middleware(GenerateRecoveryCodes(logger, manager, mem, false))
	generateReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery-codes/generate", bytes.NewReader([]byte(`{}`)))
	generateReq.Header.Set("Authorization", "Bearer "+token)
	generateRec := httptest.NewRecorder()
	generate.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 generating recovery codes, got %d: %s", generateRec.Code, generateRec.Body.String())
	}
	var generateResp GenerateRecoveryCodesResponse
	if err := json.Unmarshal(generateRec.Body.Bytes(), &generateResp); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if len(generateResp.Codes) != recoveryCodeCount {
		t.Fatalf("expected %d recovery codes, got %d", recoveryCodeCount, len(generateResp.Codes))
	}
	storedUser, ok, err := mem.GetUser(user.UserID)
	if err != nil || !ok {
		t.Fatalf("get user after generate: ok=%t err=%v", ok, err)
	}
	if len(storedUser.RecoveryCodesJSON) == 0 || string(storedUser.RecoveryCodesJSON) == "[]" {
		t.Fatalf("expected stored recovery code hashes")
	}

	resetBody, err := json.Marshal(RecoveryCodeResetRequest{
		Email:        user.Email,
		RecoveryCode: generateResp.Codes[0],
		NewPassword:  "new-password-1",
	})
	if err != nil {
		t.Fatalf("marshal reset request: %v", err)
	}
	resetRec := httptest.NewRecorder()
	ResetPasswordWithRecoveryCode(logger, manager, mem, false).ServeHTTP(
		resetRec,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery-codes/reset", bytes.NewReader(resetBody)),
	)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected 200 resetting password, got %d: %s", resetRec.Code, resetRec.Body.String())
	}
	if _, _, _, err := manager.Authenticate(user.Email, "new-password-1"); err != nil {
		t.Fatalf("authenticate with new password: %v", err)
	}
	if _, _, _, err := manager.Authenticate(user.Email, "old-password"); err == nil {
		t.Fatalf("expected old password to fail after recovery reset")
	}

	reuseRec := httptest.NewRecorder()
	ResetPasswordWithRecoveryCode(logger, manager, mem, false).ServeHTTP(
		reuseRec,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery-codes/reset", bytes.NewReader(resetBody)),
	)
	if reuseRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 reusing recovery code, got %d", reuseRec.Code)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected recovery-code audit events, got %d", len(events))
	}
}
