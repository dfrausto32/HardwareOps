package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	totplib "github.com/pquerna/otp/totp"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// totpTestEnv holds shared state for TOTP tests.
type totpTestEnv struct {
	mem     *memory.Store
	mgr     *auth.Manager
	encKey  []byte
	user    store.User
	token   string // JWT for the user
	logger  *log.Logger
}

func newTOTPTestEnv(t *testing.T) *totpTestEnv {
	t.Helper()
	mem := memory.New()
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := store.User{
		UserID:       "totp-user-1",
		Email:        "totp@example.com",
		PasswordHash: hash,
		RolesJSON:    []byte(`["viewer"]`),
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	}
	if err := mem.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	mgr, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "parcel", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	tok, _, err := mgr.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	encKey := make([]byte, 32)
	for i := range encKey {
		encKey[i] = byte(i + 1)
	}
	return &totpTestEnv{
		mem:    mem,
		mgr:    mgr,
		encKey: encKey,
		user:   user,
		token:  tok,
		logger: log.New(io.Discard, "", 0),
	}
}

func (e *totpTestEnv) authedRequest(method, url string, body []byte) *http.Request {
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	return req
}

// ── Enroll ────────────────────────────────────────────────────────────────────

func TestTOTPEnroll_Success(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPEnroll(e.logger, e.mgr, e.mem, e.encKey, false))

	req := e.authedRequest(http.MethodPost, "/auth/totp/enroll", []byte(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp TOTPEnrollResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Secret == "" {
		t.Error("expected non-empty secret")
	}
	if resp.OTPURI == "" {
		t.Error("expected non-empty OTPURI")
	}
}

func TestTOTPEnroll_NoEncKey(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPEnroll(e.logger, e.mgr, e.mem, nil, false))

	req := e.authedRequest(http.MethodPost, "/auth/totp/enroll", []byte(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestTOTPEnroll_AuthDisabled(t *testing.T) {
	handler := TOTPEnroll(nil, nil, nil, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/auth/totp/enroll", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when auth disabled, got %d", w.Code)
	}
}

// ── Confirm ───────────────────────────────────────────────────────────────────

func TestTOTPConfirm_Success(t *testing.T) {
	e := newTOTPTestEnv(t)

	// Enroll first to get a secret stored on the user.
	enrollHandler := e.mgr.Middleware(TOTPEnroll(e.logger, e.mgr, e.mem, e.encKey, false))
	enrollReq := e.authedRequest(http.MethodPost, "/auth/totp/enroll", []byte(`{}`))
	enrollW := httptest.NewRecorder()
	enrollHandler.ServeHTTP(enrollW, enrollReq)
	if enrollW.Code != http.StatusOK {
		t.Fatalf("enroll failed: %d %s", enrollW.Code, enrollW.Body.String())
	}
	var enrollResp TOTPEnrollResponse
	_ = json.NewDecoder(enrollW.Body).Decode(&enrollResp)

	// Generate a valid TOTP code for the enrolled secret.
	code, err := totplib.GenerateCode(enrollResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate TOTP code: %v", err)
	}

	confirmHandler := e.mgr.Middleware(TOTPConfirm(e.logger, e.mgr, e.mem, e.encKey, false))
	body, _ := json.Marshal(TOTPConfirmRequest{Code: code})
	confirmReq := e.authedRequest(http.MethodPost, "/auth/totp/confirm", body)
	confirmW := httptest.NewRecorder()
	confirmHandler.ServeHTTP(confirmW, confirmReq)

	if confirmW.Code != http.StatusOK {
		t.Fatalf("expected 200 on confirm, got %d: %s", confirmW.Code, confirmW.Body.String())
	}
	var confirmResp TOTPConfirmResponse
	_ = json.NewDecoder(confirmW.Body).Decode(&confirmResp)
	if !confirmResp.Enabled {
		t.Error("expected Enabled=true after confirm")
	}
}

func TestTOTPConfirm_WrongCode(t *testing.T) {
	e := newTOTPTestEnv(t)

	// Enroll.
	enrollHandler := e.mgr.Middleware(TOTPEnroll(e.logger, e.mgr, e.mem, e.encKey, false))
	enrollReq := e.authedRequest(http.MethodPost, "/auth/totp/enroll", []byte(`{}`))
	enrollW := httptest.NewRecorder()
	enrollHandler.ServeHTTP(enrollW, enrollReq)
	if enrollW.Code != http.StatusOK {
		t.Fatalf("enroll failed: %d", enrollW.Code)
	}

	confirmHandler := e.mgr.Middleware(TOTPConfirm(e.logger, e.mgr, e.mem, e.encKey, false))
	body, _ := json.Marshal(TOTPConfirmRequest{Code: "000000"})
	req := e.authedRequest(http.MethodPost, "/auth/totp/confirm", body)
	w := httptest.NewRecorder()
	confirmHandler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong code, got %d", w.Code)
	}
}

func TestTOTPConfirm_NotEnrolled(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPConfirm(e.logger, e.mgr, e.mem, e.encKey, false))
	body, _ := json.Marshal(TOTPConfirmRequest{Code: "123456"})
	req := e.authedRequest(http.MethodPost, "/auth/totp/confirm", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when TOTP not enrolled, got %d", w.Code)
	}
}

// ── Verify ────────────────────────────────────────────────────────────────────

func TestTOTPVerify_InvalidPendingToken(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := TOTPVerify(e.logger, e.mgr, e.mem, e.encKey, false)

	body, _ := json.Marshal(TOTPVerifyRequest{PendingToken: "not-a-jwt", Code: "123456"})
	req := httptest.NewRequest(http.MethodPost, "/auth/totp/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid pending token, got %d", w.Code)
	}
}

func TestTOTPVerify_MissingFields(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := TOTPVerify(e.logger, e.mgr, e.mem, e.encKey, false)

	cases := []string{
		`{}`,
		`{"pendingToken":"tok"}`,
		`{"code":"123456"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/auth/totp/verify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

func TestTOTPVerify_Success(t *testing.T) {
	e := newTOTPTestEnv(t)

	// Full flow: enroll → confirm → get pending token (simulate login) → verify.

	// 1. Enroll.
	enrollHandler := e.mgr.Middleware(TOTPEnroll(e.logger, e.mgr, e.mem, e.encKey, false))
	enrollW := httptest.NewRecorder()
	enrollHandler.ServeHTTP(enrollW, e.authedRequest(http.MethodPost, "/auth/totp/enroll", []byte(`{}`)))
	var enrollResp TOTPEnrollResponse
	_ = json.NewDecoder(enrollW.Body).Decode(&enrollResp)

	// 2. Confirm with valid code.
	code, err := totplib.GenerateCode(enrollResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	confirmHandler := e.mgr.Middleware(TOTPConfirm(e.logger, e.mgr, e.mem, e.encKey, false))
	confirmBody, _ := json.Marshal(TOTPConfirmRequest{Code: code})
	confirmW := httptest.NewRecorder()
	confirmHandler.ServeHTTP(confirmW, e.authedRequest(http.MethodPost, "/auth/totp/confirm", confirmBody))
	if confirmW.Code != http.StatusOK {
		t.Fatalf("confirm failed: %d %s", confirmW.Code, confirmW.Body.String())
	}

	// 3. Get a pending TOTP token (as if the user just logged in with password).
	pendingToken, _, err := e.mgr.IssueTOTPPendingToken(e.user.UserID)
	if err != nil {
		t.Fatalf("IssueTOTPPendingToken: %v", err)
	}

	// 4. Verify with a fresh valid TOTP code.
	code2, _ := totplib.GenerateCode(enrollResp.Secret, time.Now())
	verifyHandler := TOTPVerify(e.logger, e.mgr, e.mem, e.encKey, false)
	verifyBody, _ := json.Marshal(TOTPVerifyRequest{PendingToken: pendingToken, Code: code2})
	verifyReq := httptest.NewRequest(http.MethodPost, "/auth/totp/verify", bytes.NewReader(verifyBody))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyW := httptest.NewRecorder()
	verifyHandler.ServeHTTP(verifyW, verifyReq)

	if verifyW.Code != http.StatusOK {
		t.Fatalf("expected 200 on verify, got %d: %s", verifyW.Code, verifyW.Body.String())
	}
	var resp LoginResponse
	if err := json.NewDecoder(verifyW.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected full JWT token after verify")
	}
}

// ── Disable ───────────────────────────────────────────────────────────────────

func TestTOTPDisable_CorrectPassword(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPDisable(e.logger, e.mgr, e.mem, false))

	body, _ := json.Marshal(TOTPDisableRequest{Password: "correct-password"})
	req := e.authedRequest(http.MethodPost, "/auth/totp/disable", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTOTPDisable_WrongPassword(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPDisable(e.logger, e.mgr, e.mem, false))

	body, _ := json.Marshal(TOTPDisableRequest{Password: "wrong-password"})
	req := e.authedRequest(http.MethodPost, "/auth/totp/disable", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", w.Code)
	}
}

func TestTOTPDisable_MissingPassword(t *testing.T) {
	e := newTOTPTestEnv(t)
	handler := e.mgr.Middleware(TOTPDisable(e.logger, e.mgr, e.mem, false))

	req := e.authedRequest(http.MethodPost, "/auth/totp/disable", []byte(`{}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing password, got %d", w.Code)
	}
}
