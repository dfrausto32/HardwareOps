package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type MaintenanceState struct {
	mu        sync.RWMutex
	enabled   bool
	message   string
	updatedAt time.Time
}

func NewMaintenanceState(enabled bool, message string) *MaintenanceState {
	return &MaintenanceState{
		enabled:   enabled,
		message:   message,
		updatedAt: time.Now().UTC(),
	}
}

func (m *MaintenanceState) Get() (bool, string, time.Time) {
	if m == nil {
		return false, "", time.Time{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled, m.message, m.updatedAt
}

func (m *MaintenanceState) Set(enabled bool, message string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
	m.message = message
	m.updatedAt = time.Now().UTC()
}

func MaintenanceMiddleware(state *MaintenanceState) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if state == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enabled, _, _ := state.Get()
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			if r.URL.Path == "/healthz" ||
				strings.HasPrefix(r.URL.Path, "/api/v1/maintenance") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/auth") {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-Maintenance-Mode", "1")
			http.Error(w, "maintenance mode enabled", http.StatusServiceUnavailable)
		})
	}
}
