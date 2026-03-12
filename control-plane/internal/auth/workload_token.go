package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const workloadIdentityTokenKind = "workload_identity"

type WorkloadIdentitySession struct {
	Provider  string
	Subject   string
	Name      string
	Scopes    []string
	Metadata  map[string]string
	ExpiresAt time.Time
}

type workloadIdentityClaims struct {
	Kind     string            `json:"kind"`
	Provider string            `json:"provider"`
	Name     string            `json:"name"`
	Scopes   []string          `json:"scopes"`
	Metadata map[string]string `json:"metadata,omitempty"`
	jwt.RegisteredClaims
}

func (m *Manager) IssueWorkloadIdentityToken(session WorkloadIdentitySession) (string, time.Time, error) {
	if m == nil || !m.Enabled() {
		return "", time.Time{}, errors.New("auth disabled")
	}
	if strings.TrimSpace(session.Provider) == "" {
		return "", time.Time{}, errors.New("provider required")
	}
	if strings.TrimSpace(session.Subject) == "" {
		return "", time.Time{}, errors.New("subject required")
	}
	scopes := normalizeWorkloadScopes(session.Scopes)
	if len(scopes) == 0 {
		return "", time.Time{}, errors.New("scope required")
	}
	now := time.Now().UTC()
	exp := session.ExpiresAt.UTC()
	if exp.IsZero() || !exp.After(now) {
		exp = now.Add(15 * time.Minute)
	}
	name := strings.TrimSpace(session.Name)
	if name == "" {
		name = session.Subject
	}
	claims := workloadIdentityClaims{
		Kind:     workloadIdentityTokenKind,
		Provider: strings.TrimSpace(session.Provider),
		Name:     name,
		Scopes:   scopes,
		Metadata: session.Metadata,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   session.Subject,
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

func (m *Manager) workloadTokenFromToken(tokenStr string) (ServiceToken, bool, error) {
	if m == nil || !m.Enabled() {
		return ServiceToken{}, false, nil
	}
	claims := workloadIdentityClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return m.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return ServiceToken{}, false, nil
	}
	if claims.Kind != workloadIdentityTokenKind {
		return ServiceToken{}, false, nil
	}
	if claims.Issuer != "" && claims.Issuer != m.issuer {
		return ServiceToken{}, false, errors.New("invalid token issuer")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return ServiceToken{}, false, errors.New("invalid token subject")
	}
	return ServiceToken{
		TokenID:    claims.Subject,
		Name:       claims.Name,
		Scopes:     normalizeWorkloadScopes(claims.Scopes),
		AuthMethod: workloadIdentityTokenKind,
		ExpiresAt:  claims.ExpiresAt.Time,
	}, true, nil
}

func normalizeWorkloadScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}
