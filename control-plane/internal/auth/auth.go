package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/parcel/control-plane/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// AuthStore is the minimal store interface required by auth.Manager.
// The full store.Store satisfies it, as does any lighter implementation
// (e.g. the global-plane store).
type AuthStore interface {
	GetUser(userID string) (store.User, bool, error)
	GetUserByEmail(email string) (store.User, bool, error)
	SetUserLastLogin(userID string, at time.Time) error
	GetServiceTokenByTokenHash(tokenHash string) (store.ServiceToken, bool, error)
	SetServiceTokenLastUsed(tokenID string, at time.Time) error
}

type User struct {
	UserID     string
	Email      string
	Roles      []string
	AuthMethod string
}

type Manager struct {
	store     AuthStore
	mode      string
	jwtSecret []byte
	tokenTTL  time.Duration
	issuer    string
}

type Claims struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

const (
	userKey      ctxKey = "auth.user"
	serviceKey   ctxKey = "auth.service_token"
	ModeDisabled        = "disabled"
	ModeLocal           = "local"
)

var roleRank = map[string]int{
	"viewer":   1,
	"operator": 2,
	"admin":    3,
}

func NewManager(mode string, jwtSecret string, tokenTTL time.Duration, issuer string, st AuthStore) (*Manager, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = ModeDisabled
	}
	if mode == ModeDisabled {
		return &Manager{mode: ModeDisabled}, nil
	}
	if mode != ModeLocal {
		return nil, errors.New("unsupported auth mode")
	}
	if strings.TrimSpace(jwtSecret) == "" {
		return nil, errors.New("AUTH_JWT_SECRET required for local auth")
	}
	if tokenTTL <= 0 {
		tokenTTL = 12 * time.Hour
	}
	if issuer == "" {
		issuer = "parcel"
	}
	return &Manager{
		store:     st,
		mode:      mode,
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
		issuer:    issuer,
	}, nil
}

func (m *Manager) Enabled() bool {
	return m != nil && m.mode != ModeDisabled
}

func (m *Manager) Mode() string {
	if m == nil {
		return ModeDisabled
	}
	return m.mode
}

func (m *Manager) Issuer() string {
	if m == nil {
		return "parcel"
	}
	return m.issuer
}

// CheckPassword verifies the plaintext password against the user's stored hash.
func (m *Manager) CheckPassword(user store.User, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
}

func (m *Manager) IssueToken(user store.User) (string, time.Time, error) {
	if m == nil || !m.Enabled() {
		return "", time.Time{}, errors.New("auth disabled")
	}
	roles := rolesFromJSON(user.RolesJSON)
	now := time.Now().UTC()
	exp := now.Add(m.tokenTTL)
	claims := Claims{
		Email: user.Email,
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// AuthenticateResult indicates how the login flow should continue.
type AuthenticateResult int

const (
	AuthResultOK          AuthenticateResult = iota // full JWT issued
	AuthResultTOTPPending                           // TOTP step required; token is a totp_pending JWT
)

// Authenticate verifies email+password and returns a JWT.
// When the user has TOTP enabled, a short-lived totp_pending token is returned
// instead of a full session JWT, and result is AuthResultTOTPPending.
// Callers must check result and direct the client to POST /auth/totp/verify.
func (m *Manager) Authenticate(email, password string) (store.User, string, time.Time, AuthenticateResult, error) {
	if m == nil || !m.Enabled() {
		return store.User{}, "", time.Time{}, AuthResultOK, errors.New("auth disabled")
	}
	user, ok, err := m.store.GetUserByEmail(email)
	if err != nil {
		return store.User{}, "", time.Time{}, AuthResultOK, err
	}
	if !ok {
		return store.User{}, "", time.Time{}, AuthResultOK, errors.New("invalid credentials")
	}
	if user.Disabled {
		return store.User{}, "", time.Time{}, AuthResultOK, errors.New("user disabled")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return store.User{}, "", time.Time{}, AuthResultOK, errors.New("invalid credentials")
	}
	_ = m.store.SetUserLastLogin(user.UserID, time.Now().UTC())
	if user.TOTPEnabled {
		pendingToken, exp, err := m.IssueTOTPPendingToken(user.UserID)
		if err != nil {
			return store.User{}, "", time.Time{}, AuthResultOK, err
		}
		return user, pendingToken, exp, AuthResultTOTPPending, nil
	}
	token, exp, err := m.IssueToken(user)
	return user, token, exp, AuthResultOK, err
}

func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil || !m.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if tokenStr == "" {
			tokenStr = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if tokenStr == "" {
			next.ServeHTTP(w, r)
			return
		}
		if user, err := m.userFromToken(tokenStr); err == nil {
			ctx := context.WithValue(r.Context(), userKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if workload, ok, err := m.workloadTokenFromToken(tokenStr); err == nil && ok {
			ctx := context.WithValue(r.Context(), serviceKey, workload)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if svc, ok, _ := m.serviceTokenFromToken(tokenStr); ok {
			ctx := context.WithValue(r.Context(), serviceKey, svc)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (m *Manager) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m == nil || !m.Enabled() {
				next.ServeHTTP(w, r)
				return
			}
			user, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !hasRole(user.Roles, role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserFromContext(ctx context.Context) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	user, ok := ctx.Value(userKey).(User)
	return user, ok
}

func ServiceTokenFromContext(ctx context.Context) (ServiceToken, bool) {
	if ctx == nil {
		return ServiceToken{}, false
	}
	svc, ok := ctx.Value(serviceKey).(ServiceToken)
	return svc, ok
}

func rolesFromJSON(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var roles []string
	if err := jsonUnmarshal(data, &roles); err != nil {
		return nil
	}
	return roles
}

func hasRole(userRoles []string, required string) bool {
	required = strings.ToLower(required)
	reqRank := roleRank[required]
	best := 0
	for _, role := range userRoles {
		role = strings.ToLower(role)
		if rank := roleRank[role]; rank > best {
			best = rank
		}
	}
	return best >= reqRank
}

func HasRole(userRoles []string, required string) bool {
	return hasRole(userRoles, required)
}

func (m *Manager) userFromToken(tokenStr string) (User, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return m.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return User{}, errors.New("invalid token")
	}
	if claims.Subject == "" {
		return User{}, errors.New("invalid token subject")
	}
	if claims.Issuer != "" && claims.Issuer != m.issuer {
		return User{}, errors.New("invalid token issuer")
	}
	user, ok, err := m.store.GetUser(claims.Subject)
	if err != nil {
		return User{}, err
	}
	if !ok {
		return User{}, errors.New("user not found")
	}
	if user.Disabled {
		return User{}, errors.New("user disabled")
	}
	return User{
		UserID:     user.UserID,
		Email:      user.Email,
		Roles:      rolesFromJSON(user.RolesJSON),
		AuthMethod: m.mode,
	}, nil
}

func (m *Manager) serviceTokenFromToken(tokenStr string) (ServiceToken, bool, error) {
	if m == nil || !m.Enabled() {
		return ServiceToken{}, false, nil
	}
	tokenHash := HashToken(tokenStr)
	token, ok, err := m.store.GetServiceTokenByTokenHash(tokenHash)
	if err != nil {
		return ServiceToken{}, false, err
	}
	if !ok {
		return ServiceToken{}, false, nil
	}
	now := time.Now().UTC()
	if !token.RevokedAt.IsZero() {
		return ServiceToken{}, false, nil
	}
	if !token.ExpiresAt.IsZero() && now.After(token.ExpiresAt) {
		return ServiceToken{}, false, nil
	}
	_ = m.store.SetServiceTokenLastUsed(token.TokenID, now)
	return ServiceToken{
		TokenID:    token.TokenID,
		Name:       token.Name,
		Scopes:     parseScopes(token.ScopesJSON),
		AuthMethod: "service_token",
		ExpiresAt:  token.ExpiresAt,
	}, true, nil
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password required")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// jsonUnmarshal is isolated for easier testing/mocking.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
