package artifactingest

import (
	"context"
	"errors"
	"testing"
)

func TestPullCredentialManagerReload(t *testing.T) {
	mgr := &PullCredentialManager{
		filePath:   "/tmp/creds.json",
		inlineJSON: "",
		loadStatic: func(_ string, _ string) (map[string]map[string]string, error) {
			return map[string]map[string]string{
				"repo-a": {"authorization": "Bearer static"},
			}, nil
		},
		loadAWS: func(_ context.Context, _ string, _ string) (map[string]map[string]string, error) {
			return nil, nil
		},
		resolver: NoopCredentialResolver{},
	}

	status, err := mgr.Reload(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !status.Configured || !status.StaticConfigured {
		t.Fatalf("expected configured/staticConfigured true")
	}
	if status.CredentialRefCount != 1 || !status.ResolverAvailable {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(status.CredentialRefs) != 1 || status.CredentialRefs[0] != "repo-a" {
		t.Fatalf("unexpected refs: %+v", status.CredentialRefs)
	}

	creds, err := mgr.Resolve(context.Background(), "repo-a")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := creds.Get("authorization"); got != "Bearer static" {
		t.Fatalf("unexpected credential value: %q", got)
	}
}

func TestPullCredentialManagerReloadAWSOverride(t *testing.T) {
	mgr := &PullCredentialManager{
		filePath:    "/tmp/creds.json",
		awsSecretID: "secret-id",
		loadStatic: func(_ string, _ string) (map[string]map[string]string, error) {
			return map[string]map[string]string{
				"repo-a": {"authorization": "Bearer static"},
			}, nil
		},
		loadAWS: func(_ context.Context, _ string, _ string) (map[string]map[string]string, error) {
			return map[string]map[string]string{
				"repo-a": {"authorization": "Bearer aws"},
			}, nil
		},
		resolver: NoopCredentialResolver{},
	}

	status, err := mgr.Reload(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if status.StaticCredentialRefs != 1 || status.AWSCredentialRefs != 1 {
		t.Fatalf("unexpected source counts: %+v", status)
	}
	creds, err := mgr.Resolve(context.Background(), "repo-a")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := creds.Get("authorization"); got != "Bearer aws" {
		t.Fatalf("expected aws override, got %q", got)
	}
}

func TestPullCredentialManagerReloadErrorKeepsPreviousResolver(t *testing.T) {
	mgr := &PullCredentialManager{
		filePath: "/tmp/creds.json",
		loadStatic: func(_ string, _ string) (map[string]map[string]string, error) {
			return map[string]map[string]string{
				"repo-a": {"authorization": "Bearer static"},
			}, nil
		},
		loadAWS: func(_ context.Context, _ string, _ string) (map[string]map[string]string, error) {
			return nil, nil
		},
		resolver: NoopCredentialResolver{},
	}

	if _, err := mgr.Reload(context.Background()); err != nil {
		t.Fatalf("initial reload: %v", err)
	}
	initialStatus := mgr.Status()

	mgr.loadStatic = func(_ string, _ string) (map[string]map[string]string, error) {
		return nil, errors.New("boom")
	}
	if _, err := mgr.Reload(context.Background()); err == nil {
		t.Fatal("expected reload error")
	}

	statusAfter := mgr.Status()
	if statusAfter.LastLoadedAt != initialStatus.LastLoadedAt {
		t.Fatalf("status should remain unchanged on error")
	}
	creds, err := mgr.Resolve(context.Background(), "repo-a")
	if err != nil {
		t.Fatalf("resolve after failed reload: %v", err)
	}
	if got := creds.Get("authorization"); got != "Bearer static" {
		t.Fatalf("unexpected credential after failed reload: %q", got)
	}
}

func TestPullCredentialManagerNoConfig(t *testing.T) {
	mgr := &PullCredentialManager{
		loadStatic: func(_ string, _ string) (map[string]map[string]string, error) {
			return nil, nil
		},
		loadAWS: func(_ context.Context, _ string, _ string) (map[string]map[string]string, error) {
			return nil, nil
		},
		resolver: NoopCredentialResolver{},
	}

	status, err := mgr.Reload(context.Background())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false, got %+v", status)
	}
	if _, err := mgr.Resolve(context.Background(), "repo-a"); !errors.Is(err, ErrCredentialResolverUnavailable) {
		t.Fatalf("expected resolver unavailable, got %v", err)
	}
}
