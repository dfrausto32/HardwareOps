package artifactingest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPullAdapterRejectsInsecureHTTPWhenDisabled(t *testing.T) {
	adapter := NewHTTPPullAdapter([]string{"example.com"}, 5*time.Second, false)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: "http://example.com/demo.tar.gz",
	})
	if err == nil {
		t.Fatalf("expected insecure http rejection")
	}
}

func TestHTTPPullAdapterRejectsPrivateHostWithoutAllowlist(t *testing.T) {
	adapter := NewHTTPPullAdapter(nil, 5*time.Second, true)
	_, err := adapter.Pull(context.Background(), PullRequest{
		URI: "http://127.0.0.1/demo.tar.gz",
	})
	if !errors.Is(err, ErrSourceNotAllowed) {
		t.Fatalf("expected source not allowed error, got %v", err)
	}
}

func TestHTTPPullAdapterAllowsPrivateHostWhenAllowlisted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	adapter := NewHTTPPullAdapter([]string{"127.0.0.1"}, 5*time.Second, true)
	resp, err := adapter.Pull(context.Background(), PullRequest{URI: srv.URL + "/demo.tar.gz"})
	if err != nil {
		t.Fatalf("expected allowlisted pull success, got %v", err)
	}
	_ = resp.Body.Close()
}
