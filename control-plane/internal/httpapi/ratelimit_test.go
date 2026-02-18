package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Middleware(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute, false, "test", nil)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "1.2.3.4:1234"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
}

func TestRateLimiter_ProxyHeadersRequireTrustedRemote(t *testing.T) {
	ConfigureTrustedProxyCIDRs([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ConfigureTrustedProxyCIDRs(nil) })

	limiter := NewRateLimiter(1, time.Minute, true, "test", nil)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "8.8.8.8:5555"
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429 for untrusted proxy header, got %d", w.Code)
	}
}

func TestRateLimiter_TrustedProxyUsesForwardedClientIP(t *testing.T) {
	ConfigureTrustedProxyCIDRs([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ConfigureTrustedProxyCIDRs(nil) })

	limiter := NewRateLimiter(1, time.Minute, true, "test", nil)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "10.2.3.4:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "10.2.3.4:5555"
	req.Header.Set("X-Forwarded-For", "2.2.2.2")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected second request 200 for different forwarded client IP, got %d", w.Code)
	}
}
