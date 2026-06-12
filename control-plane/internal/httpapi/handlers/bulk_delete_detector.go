package handlers

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
)

// BulkDeletionDetector implements ransomware control R-03: it watches the
// rate of artifact deletions/deprecations per actor and raises an alert when
// an actor exceeds the threshold inside the sliding window — the signature of
// a compromised token destroying fleet update capability.
//
// On breach it emits an ERROR-level log line and a runtime event of type
// security.anomaly.bulk_artifact_deletion (visible on the events stream and
// persisted via the runtime events store). The actor is re-alerted at most
// once per window to avoid alert flooding while the deletion spree continues.
//
// All methods are safe on a nil receiver so handlers can be wired without a
// detector (tests, deployments that opt out).
type BulkDeletionDetector struct {
	mu        sync.Mutex
	window    time.Duration
	threshold int
	actions   map[string][]time.Time
	lastAlert map[string]time.Time
	logger    *log.Logger
	hub       *events.Hub
	store     runtimeEventCreator
	now       func() time.Time
}

type runtimeEventCreator interface {
	CreateRuntimeEvent(event store.RuntimeEvent) error
}

const (
	bulkDeletionThreshold = 10
	bulkDeletionWindow    = 5 * time.Minute
	bulkDeletionEventType = "security.anomaly.bulk_artifact_deletion"
)

// NewBulkDeletionDetector creates a detector with the policy threshold
// (>10 deletions/deprecations per actor in any 5-minute window).
func NewBulkDeletionDetector(logger *log.Logger, hub *events.Hub, st runtimeEventCreator) *BulkDeletionDetector {
	return &BulkDeletionDetector{
		window:    bulkDeletionWindow,
		threshold: bulkDeletionThreshold,
		actions:   map[string][]time.Time{},
		lastAlert: map[string]time.Time{},
		logger:    logger,
		hub:       hub,
		store:     st,
		now:       time.Now,
	}
}

// Record registers one deletion/deprecation by the actor and alerts when the
// actor crosses the threshold within the window.
func (d *BulkDeletionDetector) Record(actorID string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	now := d.now().UTC()
	cutoff := now.Add(-d.window)

	recent := d.actions[actorID][:0]
	for _, t := range d.actions[actorID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	d.actions[actorID] = recent

	count := len(recent)
	breached := count > d.threshold
	alreadyAlerted := d.lastAlert[actorID].After(cutoff)
	if breached && !alreadyAlerted {
		d.lastAlert[actorID] = now
	}
	d.mu.Unlock()

	if !breached || alreadyAlerted {
		return
	}

	if d.logger != nil {
		d.logger.Printf("ERROR %s actor=%s deletions=%d window=%s — possible ransomware activity; review and consider revoking the actor's sessions",
			bulkDeletionEventType, actorID, count, d.window)
	}

	payload, _ := json.Marshal(map[string]any{
		"actorId":   actorID,
		"count":     count,
		"windowSec": int(d.window.Seconds()),
		"threshold": d.threshold,
	})
	ev := events.Event{
		Type:    bulkDeletionEventType,
		At:      now,
		Payload: payload,
	}
	if d.hub != nil {
		d.hub.Publish(ev)
	}
	if d.store != nil {
		if err := d.store.CreateRuntimeEvent(store.RuntimeEvent{
			Type:        ev.Type,
			OccurredAt:  ev.At,
			PayloadJSON: ev.Payload,
		}); err != nil && d.logger != nil {
			d.logger.Printf("bulk deletion alert: runtime event write error: %v", err)
		}
	}
}
