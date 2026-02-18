package auth

import (
	"encoding/json"
	"strings"
	"time"
)

type ServiceToken struct {
	TokenID    string
	Name       string
	Scopes     []string
	AuthMethod string
	ExpiresAt  time.Time
}

func parseScopes(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal(data, &scopes); err != nil {
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

func HasScope(scopes []string, required string) bool {
	required = strings.TrimSpace(strings.ToLower(required))
	if required == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "*" || scope == required {
			return true
		}
	}
	return false
}
