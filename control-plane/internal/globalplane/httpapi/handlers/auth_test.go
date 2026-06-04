package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// fakeAuthStore is a minimal loginStore for auth handler tests.
type fakeAuthStore struct {
	users       map[string]store.User
	auditEvents []store.AuditEvent
}

func newFakeAuthStore(email, password string) *fakeAuthStore {
	hash, _ := auth.HashPassword(password)
	rolesJSON, _ := json.Marshal([]string{"admin"})
	return &fakeAuthStore{
		users: map[string]store.User{
			email: {
				UserID:       "user-1",
				Email:        email,
				PasswordHash: hash,
				RolesJSON:    rolesJSON,
				AuthProvider: "local",
			},
		},
	}
}

func (f *fakeAuthStore) GetUser(id string) (store.User, bool, error) {
	for _, u := range f.users {
		if u.UserID == id {
			return u, true, nil
		}
	}
	return store.User{}, false, nil
}
func (f *fakeAuthStore) GetUserByEmail(email string) (store.User, bool, error) {
	u, ok := f.users[email]
	return u, ok, nil
}
func (f *fakeAuthStore) SetUserLastLogin(_ string, _ time.Time) error { return nil }
func (f *fakeAuthStore) GetServiceTokenByTokenHash(_ string) (store.ServiceToken, bool, error) {
	return store.ServiceToken{}, false, nil
}
func (f *fakeAuthStore) SetServiceTokenLastUsed(_ string, _ time.Time) error { return nil }
func (f *fakeAuthStore) CreateAuditEvent(ev store.AuditEvent) error {
	f.auditEvents = append(f.auditEvents, ev)
	return nil
}

func newTestAuthManager(t *testing.T, st auth.AuthStore) *auth.Manager {
	t.Helper()
	mgr, err := auth.NewManager("local", "test-jwt-secret-global-plane!", 1*time.Hour, "parcel-global", st)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	return mgr
}

func TestLogin_Success(t *testing.T) {
	const email, password = "admin@example.com", "correct-password"
	st := newFakeAuthStore(email, password)
	mgr := newTestAuthManager(t, st)

	body := `{"email":"admin@example.com","password":"correct-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	Login(silentLogger(), mgr, st, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp loginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	// Verify audit event was written.
	if len(st.auditEvents) == 0 {
		t.Fatal("expected audit event, got none")
	}
	ev := st.auditEvents[len(st.auditEvents)-1]
	if ev.Action != "auth.login" || ev.Status != "success" {
		t.Errorf("unexpected audit event: action=%s status=%s", ev.Action, ev.Status)
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	const email = "admin@example.com"
	st := newFakeAuthStore(email, "correct-password")
	mgr := newTestAuthManager(t, st)

	body := `{"email":"admin@example.com","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	Login(silentLogger(), mgr, st, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// Audit event should record the failure.
	if len(st.auditEvents) == 0 {
		t.Fatal("expected audit event for failed login")
	}
	ev := st.auditEvents[len(st.auditEvents)-1]
	if ev.Action != "auth.login" || ev.Status != "error" {
		t.Errorf("unexpected audit event: action=%s status=%s", ev.Action, ev.Status)
	}
}

func TestLogin_BackoffBlocks(t *testing.T) {
	const email = "admin@example.com"
	st := newFakeAuthStore(email, "correct-password")
	mgr := newTestAuthManager(t, st)

	backoff := auth.NewLoginBackoff(auth.LoginBackoffConfig{
		Enabled:   true,
		Threshold: 2,
		BaseDelay: 10 * time.Second,
		MaxDelay:  30 * time.Second,
		Window:    60 * time.Minute,
	})

	fixedNow := time.Now().UTC()
	origClock := loginClock
	loginClock = func() time.Time { return fixedNow }
	defer func() { loginClock = origClock }()

	wrongBody := `{"email":"admin@example.com","password":"wrong"}`

	// Exhaust threshold (2 failures → backoff kicks in on 3rd attempt).
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(wrongBody))
		w := httptest.NewRecorder()
		Login(silentLogger(), mgr, st, false, backoff).ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// Third attempt should be blocked.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(wrongBody))
	w := httptest.NewRecorder()
	Login(silentLogger(), mgr, st, false, backoff).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on backoff, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestLogin_EmailNormalization(t *testing.T) {
	const email = "admin@example.com"
	st := newFakeAuthStore(email, "pw")
	mgr := newTestAuthManager(t, st)

	// Send with uppercase and spaces — should still authenticate.
	body := `{"email":"  ADMIN@EXAMPLE.COM  ","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	Login(silentLogger(), mgr, st, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_MissingFields(t *testing.T) {
	st := newFakeAuthStore("a@b.com", "pw")
	mgr := newTestAuthManager(t, st)

	cases := []string{
		`{}`,
		`{"email":"a@b.com"}`,
		`{"password":"pw"}`,
		`{"email":"","password":"pw"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		w := httptest.NewRecorder()
		Login(silentLogger(), mgr, st, false, nil).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

func TestLogin_AuthDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	Login(silentLogger(), nil, nil, false, nil).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when auth disabled, got %d", w.Code)
	}
}
