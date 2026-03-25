// Package sync manages background goroutines that poll regional control planes
// and populate the global aggregation cache.
package sync

import (
	"context"
	"log"
	"sync"

	"github.com/hardwareops/control-plane/internal/globalplane"
)

// Manager starts and stops per-plane poll workers.
type Manager struct {
	store   globalplane.Store
	encKey  []byte
	logger  *log.Logger
	mu      sync.Mutex
	workers map[string]*planeWorker // keyed by planeID
}

// NewManager creates a Manager. encKey is the AES-256 token decryption key.
func NewManager(store globalplane.Store, encKey []byte, logger *log.Logger) *Manager {
	return &Manager{
		store:   store,
		encKey:  encKey,
		logger:  logger,
		workers: make(map[string]*planeWorker),
	}
}

// Start loads all enabled regional planes and launches their workers.
// It blocks until ctx is cancelled, then stops all workers gracefully.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.Refresh(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	m.stopAll()
	return nil
}

// Refresh reconciles running workers against the current DB state.
// Call this after registering, updating, or removing a regional plane.
func (m *Manager) Refresh(ctx context.Context) error {
	planes, err := m.store.ListRegionalPlanes()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Build set of current plane IDs.
	active := make(map[string]bool, len(planes))
	for _, p := range planes {
		active[p.PlaneID] = true
	}

	// Stop workers for planes no longer in the DB or now disabled.
	for id, w := range m.workers {
		if !active[id] {
			w.cancel()
			<-w.done
			delete(m.workers, id)
			m.logger.Printf("[global-plane] stopped worker for plane %s", id)
		}
	}

	// Start workers for new or re-enabled planes.
	for _, p := range planes {
		if !p.Enabled {
			// Stop if currently running.
			if w, ok := m.workers[p.PlaneID]; ok {
				w.cancel()
				<-w.done
				delete(m.workers, p.PlaneID)
			}
			continue
		}
		if _, ok := m.workers[p.PlaneID]; !ok {
			w := newPlaneWorker(p, m.store, m.encKey, m.logger)
			w.start(ctx)
			m.workers[p.PlaneID] = w
			m.logger.Printf("[global-plane] started worker for plane %s (%s)", p.Name, p.PlaneID)
		}
	}
	return nil
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.workers {
		w.cancel()
		<-w.done
		delete(m.workers, id)
		m.logger.Printf("[global-plane] stopped worker for plane %s", id)
	}
}
