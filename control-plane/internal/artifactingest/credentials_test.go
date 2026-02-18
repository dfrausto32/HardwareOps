package artifactingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

func TestStaticCredentialResolverResolve(t *testing.T) {
	r := NewStaticCredentialResolver(map[string]map[string]string{
		"repo-a": {
			"Authorization": "Bearer token",
		},
	})
	creds, err := r.Resolve(context.Background(), "repo-a")
	if err != nil {
		t.Fatalf("resolve creds: %v", err)
	}
	if creds.Ref != "repo-a" {
		t.Fatalf("unexpected ref: %q", creds.Ref)
	}
	if got := creds.Get("authorization"); got != "Bearer token" {
		t.Fatalf("unexpected authorization value: %q", got)
	}
}

func TestStaticCredentialResolverResolveNotFound(t *testing.T) {
	r := NewStaticCredentialResolver(nil)
	_, err := r.Resolve(context.Background(), "missing")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestLoadStaticCredentialsFromJSON(t *testing.T) {
	values, err := LoadStaticCredentials("", `{"repo":{"authorization":"Bearer abc"}}`)
	if err != nil {
		t.Fatalf("load static credentials: %v", err)
	}
	if values["repo"]["authorization"] != "Bearer abc" {
		t.Fatalf("unexpected loaded value")
	}
}

func TestLoadStaticCredentialsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	if err := os.WriteFile(path, []byte(`{"repo":{"username":"u","password":"p"}}`), 0o600); err != nil {
		t.Fatalf("write creds file: %v", err)
	}
	values, err := LoadStaticCredentials(path, "")
	if err != nil {
		t.Fatalf("load static credentials from file: %v", err)
	}
	if values["repo"]["username"] != "u" || values["repo"]["password"] != "p" {
		t.Fatalf("unexpected loaded values")
	}
}

func TestMergeStaticCredentialSets(t *testing.T) {
	merged := MergeStaticCredentialSets(
		map[string]map[string]string{
			"repo-a": {
				"username": "first",
				"password": "first-pass",
			},
		},
		map[string]map[string]string{
			"repo-b": {
				"authorization": "Bearer token",
			},
			"repo-a": {
				"username": "second",
			},
		},
	)
	if got := merged["repo-a"]["username"]; got != "second" {
		t.Fatalf("expected repo-a username to be overridden, got %q", got)
	}
	if got := merged["repo-a"]["password"]; got != "" {
		t.Fatalf("expected repo-a password to be replaced by second set, got %q", got)
	}
	if got := merged["repo-b"]["authorization"]; got != "Bearer token" {
		t.Fatalf("expected repo-b authorization, got %q", got)
	}
}

func TestLoadStaticCredentialsFromAWSSecretManager(t *testing.T) {
	origLoad := loadAWSConfig
	origClient := newAWSSecretsManagerClient
	t.Cleanup(func() {
		loadAWSConfig = origLoad
		newAWSSecretsManagerClient = origClient
	})

	loadAWSConfig = func(_ context.Context, _ ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newAWSSecretsManagerClient = func(_ aws.Config) awsSecretsManagerClient {
		return fakeSecretsManagerClient{
			secretString: `{"repo":{"username":"aws-user","password":"aws-pass"}}`,
		}
	}

	creds, err := LoadStaticCredentialsFromAWSSecretManager(context.Background(), "demo/secret", "")
	if err != nil {
		t.Fatalf("load creds from aws secrets manager: %v", err)
	}
	if got := creds["repo"]["username"]; got != "aws-user" {
		t.Fatalf("unexpected username: %q", got)
	}
	if got := creds["repo"]["password"]; got != "aws-pass" {
		t.Fatalf("unexpected password: %q", got)
	}
}

type fakeSecretsManagerClient struct {
	secretString string
}

func (f fakeSecretsManagerClient) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return &secretsmanager.GetSecretValueOutput{
		SecretString: &f.secretString,
	}, nil
}
