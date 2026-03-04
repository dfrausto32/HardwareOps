package handlers

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
)

type PendingEnrollmentGuardConfig struct {
	RequestRPMPerSource  int
	RequestRPMPerProfile int
	MaxActive            int
	MaxActivePerProfile  int
	MaxActivePerSource   int
}

type PendingEnrollmentGuard struct {
	sourceLimiter  *pendingRateLimiter
	profileLimiter *pendingRateLimiter
	maxActive      int
	maxPerProfile  int
	maxPerSource   int
	metrics        *metrics.Metrics
}

func NewPendingEnrollmentGuard(cfg PendingEnrollmentGuardConfig, metricsCollector *metrics.Metrics) *PendingEnrollmentGuard {
	return &PendingEnrollmentGuard{
		sourceLimiter:  newPendingRateLimiter(cfg.RequestRPMPerSource, time.Minute, "pending_enroll_source", metricsCollector),
		profileLimiter: newPendingRateLimiter(cfg.RequestRPMPerProfile, time.Minute, "pending_enroll_profile", metricsCollector),
		maxActive:      cfg.MaxActive,
		maxPerProfile:  cfg.MaxActivePerProfile,
		maxPerSource:   cfg.MaxActivePerSource,
		metrics:        metricsCollector,
	}
}

func (g *PendingEnrollmentGuard) AllowRequest(sourceIP, profileKey string) (bool, time.Duration, string) {
	if g == nil {
		return true, 0, ""
	}
	if g.sourceLimiter != nil {
		key := sourceIP
		if key == "" {
			key = "unknown"
		}
		if ok, retryAfter := g.sourceLimiter.allow(key); !ok {
			return false, retryAfter, "source_rate_limited"
		}
	}
	if g.profileLimiter != nil {
		key := profileKey
		if key == "" {
			key = "unknown"
		}
		if ok, retryAfter := g.profileLimiter.allow(key); !ok {
			return false, retryAfter, "profile_rate_limited"
		}
	}
	return true, 0, ""
}

func (g *PendingEnrollmentGuard) CheckQueueCaps(st store.Store, profileID, sourceIP string, now time.Time) (bool, string, error) {
	if g == nil || st == nil {
		return true, "", nil
	}
	total, err := st.CountActivePendingEnrollments("", "", now)
	if err != nil {
		return false, "", err
	}
	if g.metrics != nil {
		g.metrics.SetPendingEnrollActive(total)
	}
	if g.maxActive > 0 {
		if total >= g.maxActive {
			if g.metrics != nil {
				g.metrics.IncPendingEnrollThrottle("queue_full_global")
			}
			return false, "queue_full_global", nil
		}
	}
	if g.maxPerProfile > 0 && profileID != "" {
		count, err := st.CountActivePendingEnrollments(profileID, "", now)
		if err != nil {
			return false, "", err
		}
		if count >= g.maxPerProfile {
			if g.metrics != nil {
				g.metrics.IncPendingEnrollThrottle("queue_full_profile")
			}
			return false, "queue_full_profile", nil
		}
	}
	if g.maxPerSource > 0 && sourceIP != "" {
		count, err := st.CountActivePendingEnrollments("", sourceIP, now)
		if err != nil {
			return false, "", err
		}
		if count >= g.maxPerSource {
			if g.metrics != nil {
				g.metrics.IncPendingEnrollThrottle("queue_full_source")
			}
			return false, "queue_full_source", nil
		}
	}
	return true, "", nil
}

func writeRetryAfterHeader(w headerSetter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		return
	}
	secs := int(retryAfter.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
}

type headerSetter interface {
	Header() http.Header
}

type pendingRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*pendingRateEntry
	name    string
	metrics *metrics.Metrics
}

type pendingRateEntry struct {
	count int
	reset time.Time
}

func newPendingRateLimiter(limit int, window time.Duration, name string, metricsCollector *metrics.Metrics) *pendingRateLimiter {
	if limit <= 0 {
		return nil
	}
	if window <= 0 {
		window = time.Minute
	}
	return &pendingRateLimiter{
		limit:   limit,
		window:  window,
		entries: map[string]*pendingRateEntry{},
		name:    name,
		metrics: metricsCollector,
	}
}

func (rl *pendingRateLimiter) allow(key string) (bool, time.Duration) {
	now := time.Now().UTC()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry := rl.entries[key]
	if entry == nil || now.After(entry.reset) {
		rl.entries[key] = &pendingRateEntry{count: 1, reset: now.Add(rl.window)}
		rl.prune(now)
		return true, rl.window
	}
	if entry.count >= rl.limit {
		if rl.metrics != nil {
			rl.metrics.IncRateLimit(rl.name)
			rl.metrics.IncPendingEnrollThrottle(rl.name)
		}
		return false, entry.reset.Sub(now)
	}
	entry.count++
	return true, entry.reset.Sub(now)
}

func (rl *pendingRateLimiter) prune(now time.Time) {
	if len(rl.entries) < 1024 {
		return
	}
	for key, entry := range rl.entries {
		if now.After(entry.reset) {
			delete(rl.entries, key)
		}
	}
}
