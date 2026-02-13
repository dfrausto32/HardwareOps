package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestDeviceCheckin_Valid(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},"currentComponents":{"agent_bundle":{"softwareVersion":"v1","configRev":"c1"}}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, deviceID = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.ServerTime.IsZero() {
		t.Fatalf("expected serverTime set")
	}
	if resp.Desired == nil || resp.Desired.Components == nil {
		t.Fatalf("expected desired set by agent")
	}
	comp, ok := resp.Desired.Components["agent_bundle"]
	if !ok || comp.SoftwareVersion != "v1" {
		t.Fatalf("expected desired component set by agent")
	}
	if _, ok, _ := mem.GetDevice(deviceID); !ok {
		t.Fatalf("device not persisted")
	}
	if st, ok, _ := mem.GetDeviceState(deviceID); !ok || st.CurrentVersion != "v1" {
		t.Fatalf("device state not persisted")
	}
}

func TestDeviceCheckin_MissingDeviceID(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	reqBody := []byte(`{"agentVersion":"0.1.0"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDeviceCheckin_InvalidDeviceID(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	reqBody := []byte(`{"deviceId":"not-a-uuid","agentVersion":"0.1.0"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, nil).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
