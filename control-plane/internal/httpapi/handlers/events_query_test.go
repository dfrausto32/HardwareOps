package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestListRuntimeEvents(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	_ = mem.CreateRuntimeEvent(store.RuntimeEvent{
		EventID:     "e1",
		OccurredAt:  time.Now().UTC().Add(-1 * time.Minute),
		Type:        events.TypeDeviceCheckin,
		DeviceID:    "d1",
		PayloadJSON: []byte(`{"ok":true}`),
	})
	_ = mem.CreateRuntimeEvent(store.RuntimeEvent{
		EventID:     "e2",
		OccurredAt:  time.Now().UTC(),
		Type:        events.TypeDeviceApplyResult,
		DeviceID:    "d1",
		PayloadJSON: []byte(`{"status":"success"}`),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/history?limit=10", nil)
	w := httptest.NewRecorder()

	ListRuntimeEvents(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp RuntimeEventListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].EventID != "e2" {
		t.Fatalf("expected newest event first")
	}
}

func TestRuntimeEventRetention(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/retention", nil)
	w := httptest.NewRecorder()
	GetRuntimeEventRetention(logger, mem).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp RuntimeEventRetentionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Days != 30 {
		t.Fatalf("expected default 30 days")
	}

	body := []byte(`{"days":14}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/events/retention", bytes.NewReader(body))
	w = httptest.NewRecorder()
	SetRuntimeEventRetention(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Days != 14 {
		t.Fatalf("expected updated days=14")
	}
}

func TestEmitRuntimeEventPublishesAndPersists(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	hub := events.NewHub(4)
	ch, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	ev := events.Event{
		Type:     events.TypeDeviceCheckin,
		DeviceID: "d1",
		At:       time.Now().UTC(),
		Payload:  []byte(`{"agentVersion":"0.1.0"}`),
	}
	emitRuntimeEvent(logger, mem, hub, ev)

	select {
	case got := <-ch:
		if got.Type != ev.Type {
			t.Fatalf("expected type %q, got %q", ev.Type, got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hub event")
	}

	rows, err := mem.ListRuntimeEvents(store.RuntimeEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list runtime events: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(rows))
	}
	if rows[0].Type != ev.Type {
		t.Fatalf("expected stored type %q, got %q", ev.Type, rows[0].Type)
	}
}
