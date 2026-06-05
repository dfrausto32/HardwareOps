package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// stubLDAPProvider satisfies ldapAuthenticator for unit tests.
type stubLDAPProvider struct {
	token     string
	expiresAt time.Time
	user      *store.User
	err       error
}

func (s *stubLDAPProvider) Authenticate(_ context.Context, _, _ string) (string, time.Time, *store.User, error) {
	return s.token, s.expiresAt, s.user, s.err
}

// successLDAPProvider returns a valid JWT and user.
func successLDAPProvider(t *testing.T, mem *memory.Store) *stubLDAPProvider {
	t.Helper()
	mgr, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "parcel", mem)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	user := store.User{
		UserID:    "ldap-user-1",
		Email:     "ldap@example.com",
		RolesJSON: []byte(`["operator"]`),
	}
	tok, exp, err := mgr.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return &stubLDAPProvider{token: tok, expiresAt: exp, user: &user}
}

func ldapLoginBody(username, password string) string {
	b, _ := json.Marshal(LDAPLoginRequest{Username: username, Password: password})
	return string(b)
}

func TestLDAPLogin_NilProvider(t *testing.T) {
	handler := LDAPLogin(silentLogger_(), nil, memory.New(), false)
	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login", strings.NewReader(ldapLoginBody("user", "pw")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nil provider, got %d", w.Code)
	}
}

func TestLDAPLogin_MissingFields(t *testing.T) {
	provider := &stubLDAPProvider{}
	handler := LDAPLogin(silentLogger_(), provider, memory.New(), false)

	cases := []string{
		`{}`,
		`{"username":"user"}`,
		`{"password":"pw"}`,
		`{"username":"","password":"pw"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, w.Code)
		}
	}
}

func TestLDAPLogin_InvalidCredentials(t *testing.T) {
	provider := &stubLDAPProvider{err: auth.ErrLDAPInvalidCredentials}
	handler := LDAPLogin(silentLogger_(), provider, memory.New(), false)

	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login",
		strings.NewReader(ldapLoginBody("user", "wrong")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid credentials, got %d", w.Code)
	}
}

func TestLDAPLogin_AccountDisabled(t *testing.T) {
	provider := &stubLDAPProvider{err: auth.ErrLDAPAccountDisabled}
	handler := LDAPLogin(silentLogger_(), provider, memory.New(), false)

	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login",
		strings.NewReader(ldapLoginBody("user", "pw")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled account, got %d", w.Code)
	}
}

func TestLDAPLogin_InternalError(t *testing.T) {
	provider := &stubLDAPProvider{err: errors.New("ldap connection refused")}
	handler := LDAPLogin(silentLogger_(), provider, memory.New(), false)

	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login",
		strings.NewReader(ldapLoginBody("user", "pw")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for internal error, got %d", w.Code)
	}
}

func TestLDAPLogin_Success(t *testing.T) {
	mem := memory.New()
	provider := successLDAPProvider(t, mem)
	handler := LDAPLogin(silentLogger_(), provider, mem, false)

	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login",
		strings.NewReader(ldapLoginBody("ldap@example.com", "correct")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.User.Email != "ldap@example.com" {
		t.Errorf("expected user email ldap@example.com, got %q", resp.User.Email)
	}
}

func TestLDAPLogin_InvalidJSON(t *testing.T) {
	provider := &stubLDAPProvider{}
	handler := LDAPLogin(silentLogger_(), provider, memory.New(), false)

	req := httptest.NewRequest(http.MethodPost, "/auth/ldap/login", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", w.Code)
	}
}
