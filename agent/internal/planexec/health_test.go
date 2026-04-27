package planexec

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/agent/internal/plan"
)

func TestHealthSuccessWithinTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	h := plan.Health{
		Type:         "probe.http",
		URL:          server.URL,
		ExpectStatus: intPtr(http.StatusOK),
		TimeoutSec:   intPtr(2),
		IntervalSec:  intPtr(1),
	}

	if err := ExecuteHealth(h, Context{}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestHealthFailureReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	h := plan.Health{
		Type:         "probe.http",
		URL:          server.URL,
		ExpectStatus: intPtr(http.StatusOK),
		TimeoutSec:   intPtr(1),
		IntervalSec:  intPtr(1),
	}

	start := time.Now()
	err := ExecuteHealth(h, Context{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if time.Since(start) < time.Second {
		t.Fatalf("expected to retry until timeout")
	}
}

func intPtr(v int) *int {
	return &v
}
