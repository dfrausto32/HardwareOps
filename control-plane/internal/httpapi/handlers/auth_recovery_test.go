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

func TestPasswordResetTokenIssueAndComplete(t *testing.T) {
	mem := memory.New()
	adminHash, err := auth.HashPassword("admin-password")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	userHash, err := auth.HashPassword("user-password")
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	adminUser := store.User{
		UserID:       "11111111-1111-1111-1111-111111111111",
		Email:        "admin@example.com",
		PasswordHash: adminHash,
		RolesJSON:    []byte(`["admin"]`),
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	}
	targetUser := store.User{
		UserID:       "22222222-2222-2222-2222-222222222222",
		Email:        "operator@example.com",
		PasswordHash: userHash,
		RolesJSON:    []byte(`["operator"]`),
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	}
	if err := mem.CreateUser(adminUser); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := mem.CreateUser(targetUser); err != nil {
		t.Fatalf("create target user: %v", err)
	}
	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "hardwareops", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	adminToken, _, err := manager.IssueToken(adminUser)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	logger := log.New(io.Discard, "", 0)

	createBody, err := json.Marshal(CreatePasswordResetTokenRequest{TTLMinutes: 30, Reason: "helpdesk"})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/users/"+targetUser.UserID+"/password-reset-token", bytes.NewReader(createBody))
	createReq = withURLParam(createReq, "userId", targetUser.UserID)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRec := httptest.NewRecorder()
	manager.Middleware(CreatePasswordResetToken(logger, manager, mem, false)).ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating password reset token, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var createResp CreatePasswordResetTokenResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Token == "" {
		t.Fatalf("expected reset token in response")
	}

	completeBody, err := json.Marshal(PasswordResetTokenCompleteRequest{
		Email:       targetUser.Email,
		ResetToken:  createResp.Token,
		NewPassword: "user-password-updated",
	})
	if err != nil {
		t.Fatalf("marshal complete request: %v", err)
	}
	completeRec := httptest.NewRecorder()
	CompletePasswordResetToken(logger, manager, mem, false).ServeHTTP(
		completeRec,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset/complete", bytes.NewReader(completeBody)),
	)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 completing password reset token, got %d: %s", completeRec.Code, completeRec.Body.String())
	}
	if _, _, _, err := manager.Authenticate(targetUser.Email, "user-password-updated"); err != nil {
		t.Fatalf("authenticate with updated password: %v", err)
	}
	if _, _, _, err := manager.Authenticate(targetUser.Email, "user-password"); err == nil {
		t.Fatalf("expected old password to fail after reset token completion")
	}

	reuseRec := httptest.NewRecorder()
	CompletePasswordResetToken(logger, manager, mem, false).ServeHTTP(
		reuseRec,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset/complete", bytes.NewReader(completeBody)),
	)
	if reuseRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 reusing password reset token, got %d", reuseRec.Code)
	}
}
