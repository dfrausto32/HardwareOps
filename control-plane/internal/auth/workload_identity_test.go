package auth

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

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

type fakeAWSSecretsManagerClient struct {
	secret string
	err    error
}

func (f fakeAWSSecretsManagerClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(f.secret),
	}, nil
}

func TestLoadWorkloadIdentityConfigsFromAWSSecretManager(t *testing.T) {
	origLoad := loadAWSConfig
	origNew := newAWSSecretsManagerClient
	t.Cleanup(func() {
		loadAWSConfig = origLoad
		newAWSSecretsManagerClient = origNew
	})
	loadAWSConfig = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newAWSSecretsManagerClient = func(cfg aws.Config) awsSecretsManagerClient {
		return fakeAWSSecretsManagerClient{secret: `[
			{
				"name":"github-actions",
				"issuer":"https://token.actions.githubusercontent.com",
				"audience":"hardwareops-ci",
				"claimMatches":{"repository":["example/app"]}
			}
		]`}
	}
	configs, err := LoadWorkloadIdentityConfigsFromAWSSecretManager(context.Background(), "secret-id", "us-east-1")
	if err != nil {
		t.Fatalf("load workload identity configs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "github-actions" {
		t.Fatalf("expected github-actions, got %q", configs[0].Name)
	}
}
