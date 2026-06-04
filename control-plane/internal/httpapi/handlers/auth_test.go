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

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestLoginBackoffBlocksAndRecovers(t *testing.T) {
	mem := memory.New()
	passwordHash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	err = mem.CreateUser(store.User{
		UserID:       "user-1",
		Email:        "admin@example.com",
		PasswordHash: passwordHash,
		RolesJSON:    []byte(`["admin"]`),
		AuthProvider: "local",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	manager, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "parcel", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	backoff := auth.NewLoginBackoff(auth.LoginBackoffConfig{
		Enabled:   true,
		Threshold: 2,
		BaseDelay: 50 * time.Millisecond,
		MaxDelay:  500 * time.Millisecond,
		Window:    10 * time.Minute,
	})
	handler := Login(log.New(io.Discard, "", 0), manager, mem, false, backoff)

	// Drive the handler's clock deterministically. The previous version relied
	// on real wall-clock time, which made the "immediate retry is blocked"
	// assertion race against password-hashing latency on slow CI runners (the
	// 50ms backoff could elapse before the next request ran, yielding 200
	// instead of 429).
	current := time.Now().UTC()
	restore := setLoginClock(func() time.Time { return current })
	defer restore()

	// First invalid login: unauthorized, no backoff header yet.
	resp1 := doLoginRequest(t, handler, "admin@example.com", "bad-password")
	if resp1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on first failure, got %d", resp1.Code)
	}
	if got := resp1.Header().Get("Retry-After"); got != "" {
		t.Fatalf("unexpected retry header on first failure: %q", got)
	}

	// Second invalid login: still unauthorized, now backoff is armed.
	resp2 := doLoginRequest(t, handler, "admin@example.com", "bad-password")
	if resp2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on second failure, got %d", resp2.Code)
	}
	if got := resp2.Header().Get("Retry-After"); got == "" {
		t.Fatalf("expected retry header on second failure")
	}

	// Retry while the backoff window is still active is blocked.
	resp3 := doLoginRequest(t, handler, "admin@example.com", "correct-password")
	if resp3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 while backoff active, got %d", resp3.Code)
	}

	// Advance past the backoff delay.
	current = current.Add(70 * time.Millisecond)

	// After the window, correct credentials succeed.
	resp4 := doLoginRequest(t, handler, "admin@example.com", "correct-password")
	if resp4.Code != http.StatusOK {
		t.Fatalf("expected 200 after backoff window, got %d", resp4.Code)
	}
	var loginResp LoginResponse
	if err := json.Unmarshal(resp4.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatalf("expected token in login response")
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "auth.login", Limit: 20})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected auth.login audit events")
	}
}

// setLoginClock overrides the package clock used by the Login handler for
// backoff bookkeeping and returns a function that restores the previous clock.
func setLoginClock(fn func() time.Time) func() {
	prev := loginClock
	loginClock = fn
	return func() { loginClock = prev }
}

func doLoginRequest(t *testing.T, handler http.HandlerFunc, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
