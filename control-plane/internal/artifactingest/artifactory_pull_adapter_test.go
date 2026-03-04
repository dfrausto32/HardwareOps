package artifactingest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestArtifactoryPullAdapterTokenAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	adapter := NewArtifactoryPullAdapter([]string{"127.0.0.1"}, 5*time.Second, true)
	resp, err := adapter.Pull(context.Background(), PullRequest{
		URI: srv.URL + "/artifactory/generic-local/demo.tar.gz",
		Credentials: Credentials{
			Values: map[string]string{
				"artifactory_token": "token-123",
			},
		},
	})
	if err != nil {
		t.Fatalf("pull with token alias: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "ok" {
		t.Fatalf("unexpected response body: %q", string(data))
	}
}

func TestArtifactoryPullAdapterAPIKeyAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-JFrog-Art-Api"); got != "api-key-abc" {
			http.Error(w, "missing api key", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	adapter := NewArtifactoryPullAdapter([]string{"127.0.0.1"}, 5*time.Second, true)
	resp, err := adapter.Pull(context.Background(), PullRequest{
		URI: srv.URL + "/artifactory/generic-local/demo.tar.gz",
		Credentials: Credentials{
			Values: map[string]string{
				"artifactory_api_key": "api-key-abc",
			},
		},
	})
	if err != nil {
		t.Fatalf("pull with api key alias: %v", err)
	}
	defer resp.Body.Close()
}

func TestArtifactoryPullAdapterHostAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	adapter := NewArtifactoryPullAdapter([]string{"example.com"}, 5*time.Second, true)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: srv.URL + "/artifactory/generic-local/demo.tar.gz",
	})
	if err == nil {
		t.Fatalf("expected host allowlist error")
	}
}

func TestArtifactoryPullAdapterRejectsInsecureHTTPWhenDisabled(t *testing.T) {
	adapter := NewArtifactoryPullAdapter([]string{"example.com"}, 5*time.Second, false)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: "http://example.com/artifactory/generic-local/demo.tar.gz",
	})
	if err == nil {
		t.Fatalf("expected insecure http rejection")
	}
}

func TestArtifactoryPullAdapterRejectsPrivateHostWithoutAllowlist(t *testing.T) {
	adapter := NewArtifactoryPullAdapter(nil, 5*time.Second, true)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: "http://127.0.0.1/artifactory/generic-local/demo.tar.gz",
	})
	if !errors.Is(err, ErrSourceNotAllowed) {
		t.Fatalf("expected source not allowed error, got %v", err)
	}
}
