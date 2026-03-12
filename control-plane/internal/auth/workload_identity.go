package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/hardwareops/control-plane/internal/config"
)

type WorkloadIdentityExchanger interface {
	Exchange(ctx context.Context, provider, rawToken string, requestedScopes []string) (WorkloadIdentitySession, error)
}

type WorkloadIdentityManager struct {
	providers map[string]*workloadIdentityProvider
}

func (m *WorkloadIdentityManager) ProviderNames() []string {
	if m == nil || len(m.providers) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.providers))
	for name := range m.providers {
		out = append(out, name)
	}
	return out
}

type workloadIdentityProvider struct {
	name          string
	issuer        string
	verifier      *gooidc.IDTokenVerifier
	claimMatches  map[string][]string
	allowedScopes map[string]struct{}
	defaultScopes []string
	ttl           time.Duration
}

type workloadIdentityProviderConfig struct {
	Name          string              `json:"name"`
	Issuer        string              `json:"issuer"`
	Audience      string              `json:"audience"`
	AllowedScopes []string            `json:"allowedScopes"`
	DefaultScopes []string            `json:"defaultScopes"`
	TTL           string              `json:"ttl"`
	ClaimMatches  map[string][]string `json:"claimMatches"`
}

func NewWorkloadIdentityManager(ctx context.Context, cfg *config.Config) (*WorkloadIdentityManager, error) {
	configs, err := loadWorkloadIdentityConfigs(cfg)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	providers := make(map[string]*workloadIdentityProvider, len(configs))
	for _, providerCfg := range configs {
		provider, err := buildWorkloadIdentityProvider(ctx, providerCfg)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", providerCfg.Name, err)
		}
		providers[provider.name] = provider
	}
	return &WorkloadIdentityManager{providers: providers}, nil
}

func (m *WorkloadIdentityManager) Exchange(ctx context.Context, provider, rawToken string, requestedScopes []string) (WorkloadIdentitySession, error) {
	if m == nil || len(m.providers) == 0 {
		return WorkloadIdentitySession{}, errors.New("workload identity disabled")
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		if len(m.providers) == 1 {
			for name := range m.providers {
				provider = name
			}
		} else {
			return WorkloadIdentitySession{}, errors.New("provider required")
		}
	}
	p, ok := m.providers[provider]
	if !ok {
		return WorkloadIdentitySession{}, errors.New("unknown provider")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return WorkloadIdentitySession{}, errors.New("id token required")
	}
	idToken, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return WorkloadIdentitySession{}, fmt.Errorf("verify id token: %w", err)
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return WorkloadIdentitySession{}, fmt.Errorf("decode claims: %w", err)
	}
	if err := validateClaimMatches(claims, p.claimMatches); err != nil {
		return WorkloadIdentitySession{}, err
	}
	subject := firstClaimValue(claims, "sub")
	if subject == "" {
		return WorkloadIdentitySession{}, errors.New("subject claim missing")
	}
	scopes, err := p.resolveScopes(requestedScopes)
	if err != nil {
		return WorkloadIdentitySession{}, err
	}
	now := time.Now().UTC()
	exp := idToken.Expiry.UTC()
	if p.ttl > 0 {
		ttlExp := now.Add(p.ttl)
		if exp.IsZero() || ttlExp.Before(exp) {
			exp = ttlExp
		}
	}
	return WorkloadIdentitySession{
		Provider:  provider,
		Subject:   subject,
		Name:      displayNameForClaims(claims, provider),
		Scopes:    scopes,
		Metadata:  metadataForClaims(claims, provider),
		ExpiresAt: exp,
	}, nil
}

func buildWorkloadIdentityProvider(ctx context.Context, cfg workloadIdentityProviderConfig) (*workloadIdentityProvider, error) {
	name := strings.TrimSpace(strings.ToLower(cfg.Name))
	if name == "" {
		return nil, errors.New("name required")
	}
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("issuer required")
	}
	audience := strings.TrimSpace(cfg.Audience)
	if audience == "" {
		return nil, errors.New("audience required")
	}
	if len(cfg.ClaimMatches) == 0 {
		return nil, errors.New("at least one claim match is required")
	}
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: audience})
	allowedScopes, err := parseProviderScopes(cfg.AllowedScopes)
	if err != nil {
		return nil, fmt.Errorf("allowed scopes: %w", err)
	}
	if len(allowedScopes) == 0 {
		allowedScopes = []string{"artifact.publish"}
	}
	defaultScopes, err := parseProviderScopes(cfg.DefaultScopes)
	if err != nil {
		return nil, fmt.Errorf("default scopes: %w", err)
	}
	if len(defaultScopes) == 0 {
		defaultScopes = append(defaultScopes, allowedScopes...)
	}
	for _, scope := range defaultScopes {
		if !scopeAllowed(scope, allowedScopes) {
			return nil, fmt.Errorf("default scope %q is not allowed", scope)
		}
	}
	ttl := 15 * time.Minute
	if strings.TrimSpace(cfg.TTL) != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			return nil, fmt.Errorf("invalid ttl: %w", err)
		}
		if parsed <= 0 {
			return nil, errors.New("ttl must be > 0")
		}
		ttl = parsed
	}
	claimMatches := make(map[string][]string, len(cfg.ClaimMatches))
	for claim, patterns := range cfg.ClaimMatches {
		claim = strings.TrimSpace(claim)
		if claim == "" {
			continue
		}
		out := make([]string, 0, len(patterns))
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			out = append(out, pattern)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("claim %q requires at least one pattern", claim)
		}
		claimMatches[claim] = out
	}
	return &workloadIdentityProvider{
		name:          name,
		issuer:        strings.TrimSpace(cfg.Issuer),
		verifier:      verifier,
		claimMatches:  claimMatches,
		allowedScopes: scopeSet(allowedScopes),
		defaultScopes: defaultScopes,
		ttl:           ttl,
	}, nil
}

func loadWorkloadIdentityConfigs(cfg *config.Config) ([]workloadIdentityProviderConfig, error) {
	if cfg == nil || !cfg.WorkloadIdentityEnabled() {
		return nil, nil
	}
	var configs []workloadIdentityProviderConfig
	if raw := strings.TrimSpace(cfg.CIWorkloadIdentityProvidersFile); raw != "" {
		data, err := os.ReadFile(raw)
		if err != nil {
			return nil, fmt.Errorf("read workload identity config file: %w", err)
		}
		parsed, err := parseWorkloadIdentityConfigBlob(data)
		if err != nil {
			return nil, fmt.Errorf("parse workload identity config file: %w", err)
		}
		configs = append(configs, parsed...)
	}
	if raw := strings.TrimSpace(cfg.CIWorkloadIdentityProvidersJSON); raw != "" {
		parsed, err := parseWorkloadIdentityConfigBlob([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("parse workload identity config json: %w", err)
		}
		configs = append(configs, parsed...)
	}
	seen := map[string]struct{}{}
	out := make([]workloadIdentityProviderConfig, 0, len(configs))
	for _, providerCfg := range configs {
		name := strings.TrimSpace(strings.ToLower(providerCfg.Name))
		if name == "" {
			return nil, errors.New("workload identity provider name required")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate workload identity provider %q", name)
		}
		seen[name] = struct{}{}
		providerCfg.Name = name
		out = append(out, providerCfg)
	}
	return out, nil
}

func parseWorkloadIdentityConfigBlob(data []byte) ([]workloadIdentityProviderConfig, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	var list []workloadIdentityProviderConfig
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var single workloadIdentityProviderConfig
	if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
		return nil, err
	}
	return []workloadIdentityProviderConfig{single}, nil
}

func validateClaimMatches(claims map[string]any, matches map[string][]string) error {
	for claim, patterns := range matches {
		values := claimValues(claims, claim)
		if len(values) == 0 {
			return fmt.Errorf("required claim %q missing", claim)
		}
		if !matchAnyPattern(values, patterns) {
			return fmt.Errorf("claim %q did not match allowed patterns", claim)
		}
	}
	return nil
}

func claimValues(claims map[string]any, key string) []string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func firstClaimValue(claims map[string]any, key string) string {
	values := claimValues(claims, key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func matchAnyPattern(values, patterns []string) bool {
	for _, value := range values {
		for _, pattern := range patterns {
			if ok, err := path.Match(pattern, value); err == nil && ok {
				return true
			}
			if value == pattern {
				return true
			}
		}
	}
	return false
}

func parseProviderScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	valid := map[string]struct{}{
		"artifact.publish": {},
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if _, ok := valid[scope]; !ok {
			return nil, fmt.Errorf("invalid scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out, nil
}

func scopeAllowed(scope string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == scope {
			return true
		}
	}
	return false
}

func scopeSet(scopes []string) map[string]struct{} {
	out := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		out[scope] = struct{}{}
	}
	return out
}

func (p *workloadIdentityProvider) resolveScopes(requested []string) ([]string, error) {
	scopes := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	valid := map[string]struct{}{
		"artifact.publish": {},
	}
	for _, scope := range requested {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if _, ok := valid[scope]; !ok {
			return nil, fmt.Errorf("invalid scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		scopes = append(scopes, p.defaultScopes...)
	}
	if len(scopes) == 0 {
		return nil, errors.New("scope required")
	}
	for _, scope := range scopes {
		if _, ok := p.allowedScopes[scope]; !ok {
			return nil, fmt.Errorf("scope %q not allowed", scope)
		}
	}
	return scopes, nil
}

func metadataForClaims(claims map[string]any, provider string) map[string]string {
	out := map[string]string{
		"provider": provider,
	}
	for _, key := range []string{"sub", "repository", "repository_owner", "ref", "workflow_ref", "job_workflow_ref", "project_path", "namespace_path"} {
		if value := firstClaimValue(claims, key); value != "" {
			out[key] = value
		}
	}
	return out
}

func displayNameForClaims(claims map[string]any, provider string) string {
	repository := firstClaimValue(claims, "repository")
	ref := firstClaimValue(claims, "ref")
	if repository != "" && ref != "" {
		return repository + "@" + ref
	}
	if repository != "" {
		return repository
	}
	if project := firstClaimValue(claims, "project_path"); project != "" {
		return project
	}
	if subject := firstClaimValue(claims, "sub"); subject != "" {
		return provider + ":" + subject
	}
	return provider
}
