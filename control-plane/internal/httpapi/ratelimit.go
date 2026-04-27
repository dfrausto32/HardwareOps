package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/parcel/control-plane/internal/metrics"
)

type RateLimiter struct {
	mu           sync.Mutex
	limit        int
	window       time.Duration
	trustProxy   bool
	entries      map[string]*rateEntry
	name         string
	metrics      *metrics.Metrics
	pruneCounter int
}

type rateEntry struct {
	count int
	reset time.Time
}

func NewRateLimiter(limit int, window time.Duration, trustProxy bool, name string, metricsCollector *metrics.Metrics) *RateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	if name == "" {
		name = "unknown"
	}
	return &RateLimiter{
		limit:      limit,
		window:     window,
		trustProxy: trustProxy,
		entries:    make(map[string]*rateEntry),
		name:       name,
		metrics:    metricsCollector,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	if rl == nil || rl.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r, rl.trustProxy)
		if key == "" {
			key = "unknown"
		}
		allowed, retryAfter := rl.allow(key)
		if !allowed {
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			if rl.metrics != nil {
				rl.metrics.IncRateLimit(rl.name)
			}
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MiddlewareWithKey is like Middleware but uses the provided keyFn to extract
// the rate-limit key. Falls back to clientIP if keyFn returns "".
func (rl *RateLimiter) MiddlewareWithKey(keyFn func(*http.Request) string, next http.Handler) http.Handler {
	if rl == nil || rl.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := keyFn(r)
		if key == "" {
			key = clientIP(r, rl.trustProxy)
		}
		if key == "" {
			key = "unknown"
		}
		allowed, retryAfter := rl.allow(key)
		if !allowed {
			secs := int(retryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			if rl.metrics != nil {
				rl.metrics.IncRateLimit(rl.name)
			}
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(key string) (bool, time.Duration) {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.pruneCounter++
	if rl.pruneCounter%100 == 0 {
		rl.prune(now)
	}

	entry := rl.entries[key]
	if entry == nil || now.After(entry.reset) {
		rl.entries[key] = &rateEntry{count: 1, reset: now.Add(rl.window)}
		return true, rl.window
	}

	if entry.count >= rl.limit {
		return false, entry.reset.Sub(now)
	}
	entry.count++
	return true, entry.reset.Sub(now)
}

func (rl *RateLimiter) prune(now time.Time) {
	for key, entry := range rl.entries {
		if now.After(entry.reset) {
			delete(rl.entries, key)
		}
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy && proxyHeadersAllowed(r) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); ip != "" {
					return ip
				}
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
