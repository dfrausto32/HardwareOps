package auth

import (
	"encoding/json"
	"strings"
	"time"
)

// Defined service token scopes. The wildcard "*" implies all scopes.
const (
	ScopeArtifactRead      = "artifact.read"
	ScopeArtifactPublish   = "artifact.publish"
	ScopeDeviceRead        = "device.read"
	ScopeDeploymentTrigger = "deployment.trigger"
	ScopeWebhookManage     = "webhook.manage"
	// Federation scopes — used by the global aggregation plane.
	// ScopeFederationPush is granted to service tokens created on regional control
	// planes so the global-plane sync client can read device/health/artifact data.
	// ScopeFederationManage gates administrative operations on the global-plane itself
	// (registering and removing regional planes).
	ScopeFederationPush   = "federation.push"
	ScopeFederationManage = "federation.manage"
)

// ValidScopes is the canonical set of allowed scope values (excluding "*").
var ValidScopes = []string{
	ScopeArtifactRead,
	ScopeArtifactPublish,
	ScopeDeviceRead,
	ScopeDeploymentTrigger,
	ScopeWebhookManage,
	ScopeFederationPush,
	ScopeFederationManage,
}

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

// HasAnyScope returns true if the token has at least one of the listed scopes.
func HasAnyScope(scopes []string, required ...string) bool {
	for _, r := range required {
		if HasScope(scopes, r) {
			return true
		}
	}
	return false
}

// IsValidScope returns true for recognised scope strings (including "*").
func IsValidScope(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "*" {
		return true
	}
	for _, v := range ValidScopes {
		if s == v {
			return true
		}
	}
	return false
}
