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
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestListDevices(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id1 := uuid.NewString()
	id2 := uuid.NewString()
	_ = mem.UpsertDevice(store.Device{DeviceID: id1, Status: "active", LastSeen: time.Now().UTC()})
	_ = mem.UpsertDevice(store.Device{DeviceID: id2, Status: "inactive", LastSeen: time.Now().UTC()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices?status=active&limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	ListDevices(logger, mem).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp DeviceListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].DeviceID != id1 {
		t.Fatalf("unexpected deviceId")
	}
}

func TestGetDevice(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	_ = mem.UpsertDevice(store.Device{DeviceID: id, Status: "active", LastSeen: time.Now().UTC()})
	_ = mem.UpsertDeviceState(store.DeviceState{DeviceID: id, CurrentVersion: "v1", CurrentConfigRev: "c1", UpdatedAt: time.Now().UTC()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+id, nil)
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	GetDevice(logger, mem).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp DeviceDetail
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.DeviceID != id {
		t.Fatalf("unexpected deviceId")
	}
	if resp.Current == nil || resp.Current.SoftwareVersion != "v1" {
		t.Fatalf("expected current state")
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+id, nil)
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	GetDevice(logger, mem).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPatchDeviceLabels(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	_ = mem.UpsertDevice(store.Device{DeviceID: id, Status: "active", LastSeen: time.Now().UTC()})

	body := []byte(`{"labels":{"region":"west"},"metadata":{"role":"edge"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PatchDevice(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPatchDevice_InvalidLabels(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	_ = mem.UpsertDevice(store.Device{DeviceID: id, Status: "active", LastSeen: time.Now().UTC()})

	body := []byte(`{"labels":"bad"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PatchDevice(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
