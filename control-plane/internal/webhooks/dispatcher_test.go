package webhooks

import (
	"encoding/json"
	"io"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
)

type stubStore struct {
	hooks      []store.Webhook
	deliveries []store.WebhookDelivery
}

func (s *stubStore) ListWebhooks() ([]store.Webhook, error) { return s.hooks, nil }
func (s *stubStore) CreateWebhookDelivery(delivery store.WebhookDelivery) error {
	s.deliveries = append(s.deliveries, delivery)
	return nil
}
func (s *stubStore) UpdateWebhookDelivery(store.WebhookDelivery) error { return nil }
func (s *stubStore) UpdateWebhookLastFired(string, time.Time, int) error { return nil }

func TestDispatcherFanoutIncludesDeviceID(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	encrypted, err := EncryptSecret(key, "shared-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	st := &stubStore{
		hooks: []store.Webhook{
			{
				ID:              uuid.NewString(),
				URL:             "http://example.invalid/hook",
				EncryptedSecret: encrypted,
				EventTypes:      []string{events.TypeDeploymentTriggered},
				Enabled:         true,
				CreatedAt:       time.Now().UTC(),
			},
		},
	}

	dispatcher := NewDispatcher(Config{Store: st, EncryptionKey: key, Workers: 1}, log.New(io.Discard, "", 0))
	dispatcher.fanout(events.Event{
		Type:     events.TypeDeploymentTriggered,
		DeviceID: "device-123",
		At:       time.Now().UTC(),
		Payload:  json.RawMessage(`{"deviceId":"device-123","status":"success"}`),
	})

	if len(st.deliveries) != 1 {
		t.Fatalf("expected one queued delivery, got %d", len(st.deliveries))
	}
	var payload Payload
	if err := json.Unmarshal(st.deliveries[0].PayloadJSON, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.DeviceID != "device-123" {
		t.Fatalf("expected top-level deviceId, got %#v", payload.DeviceID)
	}
}
