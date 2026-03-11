package objectstore

import (
	"testing"
)

func TestMinioCredentialsUsesStaticWhenKeysPresent(t *testing.T) {
	if mode := minioCredentialMode(MinIOConfig{AccessKey: "access", SecretKey: "secret"}); mode != "static" {
		t.Fatalf("credential mode = %q, want %q", mode, "static")
	}

	creds := minioCredentials(MinIOConfig{
		AccessKey: "access",
		SecretKey: "secret",
	})

	value, err := creds.Get()
	if err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	if value.AccessKeyID != "access" {
		t.Fatalf("access key = %q, want %q", value.AccessKeyID, "access")
	}
	if value.SecretAccessKey != "secret" {
		t.Fatalf("secret key = %q, want %q", value.SecretAccessKey, "secret")
	}
}

func TestMinioCredentialsUsesIAMWhenKeysMissing(t *testing.T) {
	creds := minioCredentials(MinIOConfig{})

	if creds == nil {
		t.Fatal("credentials should not be nil")
	}
	if mode := minioCredentialMode(MinIOConfig{}); mode != "iam" {
		t.Fatalf("credential mode = %q, want %q", mode, "iam")
	}
}
