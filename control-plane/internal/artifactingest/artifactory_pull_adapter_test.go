package artifactingest

import (
	"context"
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

	adapter := NewArtifactoryPullAdapter(nil, 5*time.Second)
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

	adapter := NewArtifactoryPullAdapter(nil, 5*time.Second)
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

	adapter := NewArtifactoryPullAdapter([]string{"example.com"}, 5*time.Second)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: srv.URL + "/artifactory/generic-local/demo.tar.gz",
	})
	if err == nil {
		t.Fatalf("expected host allowlist error")
	}
}
