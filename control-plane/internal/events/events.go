package events

import (
	"encoding/json"
	"sync"
	"time"
)

const (
	TypeDeviceCheckin          = "device.checkin"
	TypeDeviceEnroll           = "device.enroll"
	TypeDeviceEnrollPending    = "device.enroll_pending"
	TypeDeviceApplyResult      = "device.apply_result"
	TypeDeviceCloneSuspected   = "device.clone_suspected"
	TypeDeviceIdentityConflict = "device.identity_conflict"
	TypeArtifactRegistered     = "artifact.registered"
)

type Event struct {
	Type     string          `json:"type"`
	DeviceID string          `json:"deviceId,omitempty"`
	At       time.Time       `json:"at"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type Hub struct {
	mu     sync.RWMutex
	subs   map[chan Event]struct{}
	buffer int
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

func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	// Snapshot subscriber channels so we can release the lock before sending.
	snapshot := make([]chan Event, 0, len(h.subs))
	for ch := range h.subs {
		snapshot = append(snapshot, ch)
	}
	h.mu.RUnlock()

	for _, ch := range snapshot {
		select {
		case ch <- event:
		default:
			// Subscriber buffer full; drop event for this subscriber rather than blocking.
		}
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
