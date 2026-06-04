package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// mockOIDCStore satisfies both auth.AuthStore and auth.OIDCStore.
type mockOIDCStore struct {
	users      map[string]store.User
	byEmail    map[string]store.User
	byExternal map[string]store.User // key = provider+":"+externalID
	audit      []store.AuditEvent
}

func newMockOIDCStore() *mockOIDCStore {
	return &mockOIDCStore{
		users:      make(map[string]store.User),
		byEmail:    make(map[string]store.User),
		byExternal: make(map[string]store.User),
	}
}

// auth.AuthStore
func (m *mockOIDCStore) GetUser(id string) (store.User, bool, error) {
	u, ok := m.users[id]
	return u, ok, nil
}
func (m *mockOIDCStore) GetUserByEmail(email string) (store.User, bool, error) {
	u, ok := m.byEmail[email]
	return u, ok, nil
}
func (m *mockOIDCStore) SetUserLastLogin(id string, at time.Time) error {
	if u, ok := m.users[id]; ok {
		u.LastLoginAt = at
		m.users[id] = u
	}
	return nil
}
func (m *mockOIDCStore) GetServiceTokenByTokenHash(_ string) (store.ServiceToken, bool, error) {
	return store.ServiceToken{}, false, nil
}
func (m *mockOIDCStore) SetServiceTokenLastUsed(_ string, _ time.Time) error { return nil }

// auth.OIDCStore extras
func (m *mockOIDCStore) GetUserByExternalID(provider, extID string) (store.User, bool, error) {
	u, ok := m.byExternal[provider+":"+extID]
	return u, ok, nil
}
func (m *mockOIDCStore) CreateUser(u store.User) error {
	m.users[u.UserID] = u
	m.byEmail[u.Email] = u
	if u.ExternalID != "" {
		m.byExternal[u.AuthProvider+":"+u.ExternalID] = u
	}
	return nil
}
func (m *mockOIDCStore) UpdateUser(upd store.UserUpdate) error {
	u, ok := m.users[upd.UserID]
	if !ok {
		return nil
	}
	if upd.DisplayName != nil {
		u.DisplayName = *upd.DisplayName
	}
	if len(upd.RolesJSON) > 0 {
		u.RolesJSON = upd.RolesJSON
	}
	m.users[u.UserID] = u
	if u.Email != "" {
		m.byEmail[u.Email] = u
	}
	return nil
}

// oidcStore (audit)
func (m *mockOIDCStore) CreateAuditEvent(ev store.AuditEvent) error {
	m.audit = append(m.audit, ev)
	return nil
}

// ── Mock IdP ──────────────────────────────────────────────────────────────────

type mockIdP struct {
	server  *httptest.Server
	privKey *rsa.PrivateKey
	Subject string
	Email   string
	Groups  []string
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	idp := &mockIdP{privKey: privKey, Subject: "sub-123", Email: "sso@example.com"}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	idp.server = srv

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":               srv.URL + "/auth",
			"token_endpoint":                       srv.URL + "/token",
			"jwks_uri":                             srv.URL + "/jwks",
			"response_types_supported":             []string{"code"},
			"subject_types_supported":              []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{rsaPublicKeyJWK(&privKey.PublicKey)},
		})
	})

	// Token endpoint — ignores the code and always issues a valid ID token.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		claims := jwt.MapClaims{
			"iss":   srv.URL,
			"sub":   idp.Subject,
			"aud":   []string{"test-client-id"},
			"exp":   now.Add(10 * time.Minute).Unix(),
			"iat":   now.Unix(),
			"email": idp.Email,
			"name":  "SSO User",
		}
		if len(idp.Groups) > 0 {
			claims["groups"] = idp.Groups
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "test-key"
		signed, err := tok.SignedString(idp.privKey)
		if err != nil {
			http.Error(w, "sign: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
			"token_type":   "bearer",
			"id_token":     signed,
			"expires_in":   600,
		})
	})

	return idp
}

func (idp *mockIdP) Close() { idp.server.Close() }

func newTestProvider(t *testing.T, idp *mockIdP, st *mockOIDCStore, roleMap string) *auth.OIDCProvider {
	t.Helper()
	mgr, err := auth.NewManager("local", "test-jwt-secret-global-plane!", 1*time.Hour, "parcel-global", st)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	p, err := auth.NewOIDCProviderWithConfig(context.Background(), auth.OIDCConfig{
		Issuer:       idp.server.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost/callback",
		GroupClaim:   "groups",
		RoleMap:      roleMap,
		DefaultRole:  "viewer",
	}, st, mgr)
	if err != nil {
		t.Fatalf("NewOIDCProviderWithConfig: %v", err)
	}
	return p
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestOIDCLogin_RedirectsToIdP(t *testing.T) {
	idp := newMockIdP(t)
	defer idp.Close()
	p := newTestProvider(t, idp, newMockOIDCStore(), "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	w := httptest.NewRecorder()
	OIDCLogin(p).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), idp.server.URL) {
		t.Errorf("redirect location does not point to mock IdP: %s", w.Header().Get("Location"))
	}
	if w.Header().Get("Set-Cookie") == "" {
		t.Error("expected state cookie")
	}
}

func TestOIDCCallback_MissingStateCookie(t *testing.T) {
	idp := newMockIdP(t)
	defer idp.Close()
	st := newMockOIDCStore()
	p := newTestProvider(t, idp, st, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=x&state=s", nil)
	w := httptest.NewRecorder()
	OIDCCallback(silentLogger(), p, st, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !auditHasAction(st.audit, "auth.oidc.login.failed") {
		t.Error("expected auth.oidc.login.failed audit event")
	}
}

func TestOIDCCallback_IdPError(t *testing.T) {
	idp := newMockIdP(t)
	defer idp.Close()
	st := newMockOIDCStore()
	p := newTestProvider(t, idp, st, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?error=access_denied&state=s", nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: "s"})
	w := httptest.NewRecorder()
	OIDCCallback(silentLogger(), p, st, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestOIDCCallback_Success_JSONResponse(t *testing.T) {
	idp := newMockIdP(t)
	defer idp.Close()
	st := newMockOIDCStore()
	p := newTestProvider(t, idp, st, "")

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=fake-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()

	OIDCCallback(silentLogger(), p, st, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["token"] == nil || resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
	if len(st.users) == 0 {
		t.Error("expected user to be upserted in store")
	}
	if !auditHasAction(st.audit, "auth.oidc.login") {
		t.Error("expected auth.oidc.login audit event")
	}
}

func TestOIDCCallback_Success_Redirect(t *testing.T) {
	idp := newMockIdP(t)
	defer idp.Close()
	st := newMockOIDCStore()
	p := newTestProvider(t, idp, st, "")

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=x&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()

	OIDCCallback(silentLogger(), p, st, false, "https://app.example.com").ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://app.example.com?global_oidc_token=") {
		t.Errorf("unexpected redirect: %s", loc)
	}
}

func TestOIDCCallback_GroupRoleMapping(t *testing.T) {
	idp := newMockIdP(t)
	idp.Groups = []string{"platform-admins"}
	defer idp.Close()
	st := newMockOIDCStore()
	p := newTestProvider(t, idp, st, `{"platform-admins":"admin"}`)

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=x&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()
	OIDCCallback(silentLogger(), p, st, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	for _, u := range st.users {
		roles := rolesFromJSON(u.RolesJSON)
		for _, r := range roles {
			if r == "admin" {
				return // pass
			}
		}
		t.Errorf("expected admin role, got: %v", roles)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func auditHasAction(events []store.AuditEvent, action string) bool {
	for _, ev := range events {
		if ev.Action == action {
			return true
		}
	}
	return false
}

// rsaPublicKeyJWK returns a minimal RS256 JWK for the given public key.
func rsaPublicKeyJWK(pub *rsa.PublicKey) map[string]any {
	nBytes := pub.N.Bytes()
	e := big.NewInt(int64(pub.E))
	eBytes := e.Bytes()
	return map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": "test-key",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(nBytes),
		"e":   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}
