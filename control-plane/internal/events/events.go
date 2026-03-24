package events

import (
	"encoding/json"
	"sync"
	"time"
)

const (
	// Device lifecycle events.
	TypeDeviceCheckin          = "device.checkin"
	TypeDeviceEnroll           = "device.enroll"
	TypeDeviceEnrollPending    = "device.enroll_pending"
	TypeDeviceApplyResult      = "device.apply_result"
	TypeDeviceCloneSuspected   = "device.clone_suspected"
	TypeDeviceIdentityConflict = "device.identity_conflict"
	TypeDeviceOffline          = "device.offline"
	// Artifact events.
	TypeArtifactRegistered     = "artifact.registered"
	TypeArtifactDeprecated     = "artifact.deprecated"
	TypeArtifactScanCompleted  = "artifact.scan_completed"
	// Deployment events.
	TypeDeploymentTriggered    = "deployment.triggered"
	// Infrastructure events.
	TypeCertRotated            = "cert.rotated"
	TypeLicenseCapWarning      = "license.cap_warning"
)

type Event struct {
	Type     string          `json:"type"`
	DeviceID string          `json:"deviceId,omitempty"`
	At       time.Time       `json:"at"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// WebhookDispatcher is a narrow interface so the Hub does not import the
// webhooks package directly (avoids a circular dependency).
type WebhookDispatcher interface {
	// Fanout is called synchronously inside Publish but must be non-blocking.
	Fanout(event Event)
}

type Hub struct {
	mu         sync.RWMutex
	subs       map[chan Event]struct{}
	buffer     int
	dispatcher WebhookDispatcher
}

func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = 64
	}
	return &Hub{
		subs:   map[chan Event]struct{}{},
		buffer: buffer,
	}
}

// SetDispatcher wires an outbound webhook dispatcher into the hub.
// Must be called before any events are published.
func (h *Hub) SetDispatcher(d WebhookDispatcher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dispatcher = d
}

func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	snapshot := make([]chan Event, 0, len(h.subs))
	for ch := range h.subs {
		snapshot = append(snapshot, ch)
	}
	d := h.dispatcher
	h.mu.RUnlock()

	for _, ch := range snapshot {
		select {
		case ch <- event:
		default:
			// Subscriber buffer full; drop rather than block.
		}
	}

	// Non-blocking: dispatcher fanout runs in its own goroutines.
	if d != nil {
		d.Fanout(event)
	}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, h.buffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}
