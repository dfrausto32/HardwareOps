package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store/memory"
)

// ── Mock OIDC IdP ─────────────────────────────────────────────────────────────

// regionalMockIdP is a minimal in-process OIDC provider for handler tests.
type regionalMockIdP struct {
	server  *httptest.Server
	privKey *rsa.PrivateKey
	// Subject and Email embedded in issued ID tokens.
	Subject string
	Email   string
	Groups  []string
}

func newRegionalMockIdP(t *testing.T) *regionalMockIdP {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	idp := &regionalMockIdP{privKey: privKey, Subject: "oidc-sub-1", Email: "oidc@example.com"}
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
		n := privKey.PublicKey.N.Bytes()
		e := new(big.Int).SetInt64(int64(privKey.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA", "use": "sig", "kid": "k1", "alg": "RS256",
				"n": base64.RawURLEncoding.EncodeToString(n),
				"e": base64.RawURLEncoding.EncodeToString(e),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		claims := jwt.MapClaims{
			"iss": srv.URL, "sub": idp.Subject,
			"aud": []string{"test-client"}, "exp": now.Add(10 * time.Minute).Unix(),
			"iat": now.Unix(), "email": idp.Email, "name": "OIDC User",
		}
		if len(idp.Groups) > 0 {
			claims["groups"] = idp.Groups
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "k1"
		signed, _ := tok.SignedString(idp.privKey)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "bearer",
			"id_token": signed, "expires_in": 600,
		})
	})
	return idp
}

func (idp *regionalMockIdP) Close() { idp.server.Close() }

func newRegionalOIDCProvider(t *testing.T, idp *regionalMockIdP, mem *memory.Store, roleMap string) *auth.OIDCProvider {
	t.Helper()
	mgr, err := auth.NewManager("local", "0123456789abcdef0123456789abcdef", 12*time.Hour, "parcel", mem)
	if err != nil {
		t.Fatalf("auth.NewManager: %v", err)
	}
	p, err := auth.NewOIDCProviderWithConfig(context.Background(), auth.OIDCConfig{
		Issuer:       idp.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
		GroupClaim:   "groups",
		RoleMap:      roleMap,
		DefaultRole:  "viewer",
	}, mem, mgr)
	if err != nil {
		t.Fatalf("NewOIDCProviderWithConfig: %v", err)
	}
	return p
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestOIDCLogin_Regional_RedirectsToIdP(t *testing.T) {
	idp := newRegionalMockIdP(t)
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	w := httptest.NewRecorder()
	OIDCLogin(provider).ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), idp.server.URL) {
		t.Errorf("redirect should point to IdP, got: %s", w.Header().Get("Location"))
	}
	if w.Header().Get("Set-Cookie") == "" {
		t.Error("expected state cookie to be set")
	}
}

func TestOIDCCallback_Regional_MissingStateCookie(t *testing.T) {
	idp := newRegionalMockIdP(t)
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, "")
	logger := silentLogger_()

	req := httptest.NewRequest(http.MethodGet, "/callback?code=x&state=y", nil)
	w := httptest.NewRecorder()
	OIDCCallback(logger, provider, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing state cookie, got %d", w.Code)
	}
}

func TestOIDCCallback_Regional_IdPError(t *testing.T) {
	idp := newRegionalMockIdP(t)
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, "")
	logger := silentLogger_()

	req := httptest.NewRequest(http.MethodGet, "/callback?error=access_denied&state=s", nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: "s"})
	w := httptest.NewRecorder()
	OIDCCallback(logger, provider, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for IdP error, got %d", w.Code)
	}
}

func TestOIDCCallback_Regional_Success_NewUser(t *testing.T) {
	idp := newRegionalMockIdP(t)
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, "")
	logger := silentLogger_()

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/callback?code=fake&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()
	OIDCCallback(logger, provider, mem, false).ServeHTTP(w, req)

	// Success: redirect to /#oidc_token=...
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "oidc_token=") {
		t.Errorf("expected oidc_token in redirect Location, got: %s", loc)
	}
}

func TestOIDCCallbackJSON_Regional_Success(t *testing.T) {
	idp := newRegionalMockIdP(t)
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, "")
	logger := silentLogger_()

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/callback?code=fake&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()
	OIDCCallbackJSON(logger, provider, mem, false).ServeHTTP(w, req)

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
}

func TestOIDCCallbackJSON_Regional_GroupRoleMapping(t *testing.T) {
	idp := newRegionalMockIdP(t)
	idp.Groups = []string{"sre-team"}
	defer idp.Close()
	mem := memory.New()
	provider := newRegionalOIDCProvider(t, idp, mem, `{"sre-team":"operator"}`)
	logger := silentLogger_()

	state, _ := auth.GenerateState()
	req := httptest.NewRequest(http.MethodGet, "/callback?code=x&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "hwops_oidc_state", Value: state})
	w := httptest.NewRecorder()
	OIDCCallbackJSON(logger, provider, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp LoginResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.User.UserID == "" {
		t.Error("expected user in response")
	}
	// User should have operator role from group mapping.
	var roles []string
	_ = json.Unmarshal(resp.User.Roles, &roles)
	hasOperator := false
	for _, r := range roles {
		if r == "operator" {
			hasOperator = true
		}
	}
	if !hasOperator {
		t.Errorf("expected operator role from group mapping, got: %v", roles)
	}
}

// silentLogger_ returns a no-op logger for tests (named with trailing _ to
// avoid conflict with any package-level silentLogger in other test files).
func silentLogger_() *log.Logger { return log.New(io.Discard, "", 0) }
