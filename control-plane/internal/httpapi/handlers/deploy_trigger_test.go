package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestTriggerDeviceApply(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	hub := events.NewHub(4)
	sub, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	deviceID := uuid.NewString()
	if err := mem.UpsertDevice(store.Device{DeviceID: deviceID, Status: "active", LastSeen: time.Now().UTC()}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+deviceID+"/trigger-apply", bytes.NewBufferString(`{"reason":"ci"}`))
	req = withURLParam(req, "deviceId", deviceID)
	w := httptest.NewRecorder()

	TriggerDeviceApply(logger, mem, hub, false, 10).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	trigger, ok, err := mem.ConsumeDeployTrigger(deviceID)
	if err != nil || !ok {
		t.Fatalf("consume trigger: ok=%v err=%v", ok, err)
	}
	if trigger.DeviceID != deviceID || trigger.Reason != "ci" {
		t.Fatalf("unexpected trigger: %#v", trigger)
	}

	select {
	case ev := <-sub:
		if ev.Type != events.TypeDeploymentTriggered {
			t.Fatalf("unexpected event type: %s", ev.Type)
		}
		if ev.DeviceID != deviceID {
			t.Fatalf("expected device-scoped event, got %q", ev.DeviceID)
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["deviceId"] != deviceID {
			t.Fatalf("expected payload deviceId=%s, got %#v", deviceID, payload["deviceId"])
		}
	default:
		t.Fatal("expected runtime event")
	}

	auditEvents, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "deployment.trigger", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(auditEvents) == 0 {
		t.Fatal("expected audit event")
	}
	var metadata map[string]any
	if err := json.Unmarshal(auditEvents[0].MetadataJSON, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if metadata["deviceId"] != deviceID {
		t.Fatalf("expected audit metadata deviceId=%s, got %#v", deviceID, metadata["deviceId"])
	}
}

func TestTriggerGroupApply_TriggersMatchingActiveDevicesOnly(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	hub := events.NewHub(4)
	groupID := uuid.NewString()

	if err := mem.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "edge",
		SelectorJSON: []byte(`{"role":"edge"}`),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	activeOne := uuid.NewString()
	activeTwo := uuid.NewString()
	decommissioned := uuid.NewString()
	other := uuid.NewString()
	devices := []store.Device{
		{DeviceID: activeOne, Status: "active", LastSeen: time.Now().UTC(), LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: activeTwo, Status: "active", LastSeen: time.Now().UTC(), LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: decommissioned, Status: "decommissioned", LastSeen: time.Now().UTC(), LabelsJSON: []byte(`{"role":"edge"}`)},
		{DeviceID: other, Status: "active", LastSeen: time.Now().UTC(), LabelsJSON: []byte(`{"role":"core"}`)},
	}
	for _, d := range devices {
		if err := mem.UpsertDevice(d); err != nil {
			t.Fatalf("seed device %s: %v", d.DeviceID, err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID+"/trigger-apply", bytes.NewBufferString(`{"reason":"rollout"}`))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()

	TriggerGroupApply(logger, mem, hub, false, 10).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp TriggerApplyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Triggered != 2 {
		t.Fatalf("expected 2 triggered devices, got %d", resp.Triggered)
	}

	for _, id := range []string{activeOne, activeTwo} {
		if _, ok, err := mem.ConsumeDeployTrigger(id); err != nil || !ok {
			t.Fatalf("expected trigger for %s: ok=%v err=%v", id, ok, err)
		}
	}
	if _, ok, err := mem.ConsumeDeployTrigger(decommissioned); err != nil {
		t.Fatalf("consume decommissioned trigger: %v", err)
	} else if ok {
		t.Fatalf("did not expect trigger for decommissioned device")
	}
	if _, ok, err := mem.ConsumeDeployTrigger(other); err != nil {
		t.Fatalf("consume other trigger: %v", err)
	} else if ok {
		t.Fatalf("did not expect trigger for unmatched device")
	}
}
