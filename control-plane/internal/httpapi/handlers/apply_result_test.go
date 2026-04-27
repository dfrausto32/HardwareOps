package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestPostApplyResult(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"success","appliedVersion":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ApplyResultResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Status != "success" {
		t.Fatalf("unexpected status")
	}
}

func TestPostApplyResult_InvalidStatus(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"nope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPostApplyResult_InvalidPreApplyStatus(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"success","appliedVersion":"1.0.0","preApplyStatus":"nope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPostApplyResult_EmitsDeviceIDInRuntimeEventPayload(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	hub := events.NewHub(4)
	sub, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	id := uuid.NewString()
	artifactID := uuid.NewString()
	body := []byte(`{"status":"error","artifactId":"` + artifactID + `","component":"app_bundle","appliedVersion":"1.0.0","error":"boom"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, hub, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case ev := <-sub:
		if ev.Type != events.TypeDeviceApplyResult {
			t.Fatalf("unexpected event type: %s", ev.Type)
		}
		if ev.DeviceID != id {
			t.Fatalf("expected device-scoped event, got %q", ev.DeviceID)
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatalf("decode event payload: %v", err)
		}
		want := map[string]any{
			"deviceId":       id,
			"status":         "error",
			"artifactId":     artifactID,
			"component":      "app_bundle",
			"appliedVersion": "1.0.0",
			"error":          "boom",
		}
		for key, value := range want {
			if payload[key] != value {
				t.Fatalf("expected payload[%s]=%#v, got %#v", key, value, payload[key])
			}
		}
	default:
		t.Fatal("expected runtime event")
	}

	eventsRows, err := mem.ListRuntimeEvents(store.RuntimeEventFilter{Type: events.TypeDeviceApplyResult, DeviceID: id, Limit: 10})
	if err != nil {
		t.Fatalf("list runtime events: %v", err)
	}
	if len(eventsRows) != 1 {
		t.Fatalf("expected one stored runtime event, got %d", len(eventsRows))
	}
}
