package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/config"
	"github.com/parcel/control-plane/internal/store"
	"golang.org/x/oauth2"
)

const oidcStateCookieName = "hwops_oidc_state"

// OIDCConfig holds the OIDC provider configuration.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string // space- or comma-separated; defaults to "openid email profile"
	GroupClaim   string // JWT claim that carries group membership; defaults to "groups"
	RoleMap      string // JSON map of IdP group → hwops role
	DefaultRole  string // Role assigned when no group matches; defaults to "viewer"
}

// OIDCStore is the minimal store interface required by OIDCProvider.
// Both the full store.Store and the global-plane's PostgresStore satisfy it.
type OIDCStore interface {
	GetUserByEmail(email string) (store.User, bool, error)
	GetUserByExternalID(provider, externalID string) (store.User, bool, error)
	CreateUser(user store.User) error
	UpdateUser(update store.UserUpdate) error
	SetUserLastLogin(userID string, at time.Time) error
}

// OIDCProvider handles OIDC authorization code flow.
type OIDCProvider struct {
	provider    *gooidc.Provider
	verifier    *gooidc.IDTokenVerifier
	oauth2Cfg   oauth2.Config
	groupClaim  string
	roleMap     map[string]string // IdP group -> hwops role
	defaultRole string
	store       OIDCStore
	manager     *Manager
}

// NewOIDCProvider discovers the IdP configuration and returns an OIDCProvider.
// The regional control-plane calls this with *config.Config and store.Store.
func NewOIDCProvider(ctx context.Context, cfg *config.Config, st store.Store, mgr *Manager) (*OIDCProvider, error) {
	return NewOIDCProviderWithConfig(ctx, OIDCConfig{
		Issuer:       cfg.AuthOIDCIssuer,
		ClientID:     cfg.AuthOIDCClientID,
		ClientSecret: cfg.AuthOIDCClientSecret,
		RedirectURL:  cfg.AuthOIDCRedirectURL,
		Scopes:       cfg.AuthOIDCScopes,
		GroupClaim:   cfg.AuthOIDCGroupClaim,
		RoleMap:      cfg.AuthOIDCRoleMap,
		DefaultRole:  cfg.AuthOIDCDefaultRole,
	}, st, mgr)
}

// NewOIDCProviderWithConfig is the primary constructor for the global-plane (and
// any consumer that does not use the regional config.Config type).
func NewOIDCProviderWithConfig(ctx context.Context, cfg OIDCConfig, st OIDCStore, mgr *Manager) (*OIDCProvider, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("AUTH_OIDC_ISSUER is required")
	}
	if cfg.ClientID == "" {
		return nil, errors.New("AUTH_OIDC_CLIENT_ID is required")
	}
	if cfg.ClientSecret == "" {
		return nil, errors.New("AUTH_OIDC_CLIENT_SECRET is required")
	}
	if cfg.RedirectURL == "" {
		return nil, errors.New("AUTH_OIDC_REDIRECT_URL is required")
	}

	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	scopes := parseScopesCSV(cfg.Scopes)
	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "email", "profile"}
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	roleMap := map[string]string{}
	if cfg.RoleMap != "" {
		if err := json.Unmarshal([]byte(cfg.RoleMap), &roleMap); err != nil {
			return nil, fmt.Errorf("AUTH_OIDC_ROLE_MAP invalid JSON: %w", err)
		}
	}

	groupClaim := cfg.GroupClaim
	if groupClaim == "" {
		groupClaim = "groups"
	}

	defaultRole := cfg.DefaultRole
	if defaultRole == "" {
		defaultRole = "viewer"
	}

	return &OIDCProvider{
		provider:    provider,
		verifier:    verifier,
		oauth2Cfg:   oauth2Cfg,
		groupClaim:  groupClaim,
		roleMap:     roleMap,
		defaultRole: defaultRole,
		store:       st,
		manager:     mgr,
	}, nil
}

// AuthCodeURL returns the IdP redirect URL for the given state.
func (p *OIDCProvider) AuthCodeURL(state string) string {
	return p.oauth2Cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// Exchange performs the authorization code exchange and upserts the user.
// Returns the signed JWT, its expiry, and the store.User.
func (p *OIDCProvider) Exchange(ctx context.Context, code, stateParam, expectedState string) (token string, expiresAt time.Time, user *store.User, err error) {
	if stateParam != expectedState {
		return "", time.Time{}, nil, errors.New("oidc state mismatch")
	}

	oauth2Token, err := p.oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("oidc code exchange: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return "", time.Time{}, nil, errors.New("oidc: no id_token in response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("oidc id_token verify: %w", err)
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return "", time.Time{}, nil, fmt.Errorf("oidc claims: %w", err)
	}

	email, _ := claims["email"].(string)
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", time.Time{}, nil, errors.New("oidc: email claim missing")
	}

	sub := idToken.Subject
	if sub == "" {
		return "", time.Time{}, nil, errors.New("oidc: subject claim missing")
	}

	displayName, _ := claims["name"].(string)

	// Map groups to role.
	role := p.mapRole(claims)
	rolesJSON, _ := json.Marshal([]string{role})

	now := time.Now().UTC()

	// Upsert user: try by external ID first, then by email.
	u, found, err := p.store.GetUserByExternalID("oidc", sub)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("oidc user lookup: %w", err)
	}
	if !found {
		// Try by email (account linking).
		u, found, err = p.store.GetUserByEmail(email)
		if err != nil {
			return "", time.Time{}, nil, fmt.Errorf("oidc email lookup: %w", err)
		}
	}

	if !found {
		// Create new user.
		u = store.User{
			UserID:       uuid.NewString(),
			Email:        email,
			DisplayName:  displayName,
			RolesJSON:    rolesJSON,
			AuthProvider: "oidc",
			ExternalID:   sub,
			CreatedAt:    now,
			UpdatedAt:    now,
			LastLoginAt:  now,
		}
		if err := p.store.CreateUser(u); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("oidc create user: %w", err)
		}
	} else {
		// Update existing user's roles (and display name if provided).
		dispName := u.DisplayName
		if displayName != "" {
			dispName = displayName
		}
		update := store.UserUpdate{
			UserID:      u.UserID,
			DisplayName: &dispName,
			RolesJSON:   rolesJSON,
		}
		if err := p.store.UpdateUser(update); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("oidc update user: %w", err)
		}
		// Update auth_provider and external_id directly if they've changed.
		u.AuthProvider = "oidc"
		u.ExternalID = sub
		u.RolesJSON = rolesJSON
		u.DisplayName = dispName
		_ = p.store.SetUserLastLogin(u.UserID, now)
		u.LastLoginAt = now
	}

	signed, exp, err := p.manager.IssueToken(u)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("oidc issue token: %w", err)
	}

	return signed, exp, &u, nil
}

// mapRole maps IdP groups to the highest-precedence hwops role.
func (p *OIDCProvider) mapRole(claims map[string]interface{}) string {
	groupsRaw, ok := claims[p.groupClaim]
	if !ok {
		return p.defaultRole
	}

	var groups []string
	switch v := groupsRaw.(type) {
	case []interface{}:
		for _, g := range v {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	case []string:
		groups = v
	case string:
		groups = []string{v}
	}

	best := ""
	bestRank := 0
	for _, g := range groups {
		role, ok := p.roleMap[g]
		if !ok {
			continue
		}
		rank := roleRank[strings.ToLower(role)]
		if rank > bestRank {
			bestRank = rank
			best = role
		}
	}
	if best == "" {
		return p.defaultRole
	}
	return best
}

// GenerateState produces a 16-byte random hex string for OIDC state.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetStateCookie writes the OIDC state to a short-lived HttpOnly cookie.
func SetStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ReadStateCookie reads the OIDC state cookie value.
func ReadStateCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		return "", errors.New("oidc state cookie missing")
	}
	return c.Value, nil
}

// ClearStateCookie removes the OIDC state cookie.
func ClearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// parseScopesCSV splits a space-or-comma separated scopes string.
func parseScopesCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Accept both space-separated and comma-separated.
	raw = strings.ReplaceAll(raw, ",", " ")
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
