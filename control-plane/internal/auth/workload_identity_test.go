package auth

import "testing"

func TestValidateClaimMatches(t *testing.T) {
	claims := map[string]any{
		"repository":   "example/app",
		"ref":          "refs/heads/main",
		"workflow_ref": "example/app/.github/workflows/release.yml@refs/heads/main",
	}
	if err := validateClaimMatches(claims, map[string][]string{
		"repository":   {"example/*"},
		"ref":          {"refs/heads/main"},
		"workflow_ref": {"example/app/.github/workflows/*.yml@refs/heads/main"},
	}); err != nil {
		t.Fatalf("expected claim match success, got %v", err)
	}
	if err := validateClaimMatches(claims, map[string][]string{
		"repository": {"other/*"},
	}); err == nil {
		t.Fatal("expected claim match failure")
	}
}

func TestParseWorkloadIdentityConfigBlob(t *testing.T) {
	configs, err := parseWorkloadIdentityConfigBlob([]byte(`{
		"name":"github-actions",
		"issuer":"https://token.actions.githubusercontent.com",
		"audience":"hardwareops-ci",
		"claimMatches":{"repository":["example/app"]}
	}`))
	if err != nil {
		t.Fatalf("parse config blob: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "github-actions" {
		t.Fatalf("expected github-actions name, got %q", configs[0].Name)
	}
}
