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

func TestPutDesiredStateDevice(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"desiredVersion":"v2","desiredConfigRev":"c2"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PutDesiredStateDevice(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp DesiredStateDeviceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Source != "manual" {
		t.Fatalf("expected source=manual")
	}
}

func TestPutDesiredStateDevice_RequiresArtifactWhenVersionEmpty(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"desiredConfigRev":"c2"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PutDesiredStateDevice(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPutDesiredStateDevice_AllowsCheckinIntervalOnly(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"checkinIntervalSec":15}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PutDesiredStateDevice(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPutDesiredStateGroup(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"desiredVersion":"v1"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/groups/"+id, bytes.NewReader(body))
	req = withURLParam(req, "groupId", id)
	w := httptest.NewRecorder()

	PutDesiredStateGroup(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPutDesiredStateGroup_RequiresArtifactWhenVersionEmpty(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"desiredConfigRev":"c2"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/groups/"+id, bytes.NewReader(body))
	req = withURLParam(req, "groupId", id)
	w := httptest.NewRecorder()

	PutDesiredStateGroup(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPutDesiredStateGroup_AllowsCheckinIntervalOnly(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"checkinIntervalSec":30}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/groups/"+id, bytes.NewReader(body))
	req = withURLParam(req, "groupId", id)
	w := httptest.NewRecorder()

	PutDesiredStateGroup(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPutDesiredStateGroup_InvalidCheckinInterval(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"checkinIntervalSec":-1}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/groups/"+id, bytes.NewReader(body))
	req = withURLParam(req, "groupId", id)
	w := httptest.NewRecorder()

	PutDesiredStateGroup(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListDesiredState(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	_ = mem.UpsertDesiredStateGroup(store.DesiredStateGroup{GroupID: uuid.NewString(), DesiredVersion: "v1", UpdatedAt: time.Now().UTC()})
	_ = mem.UpsertDesiredStateDevice(store.DesiredStateDevice{DeviceID: uuid.NewString(), DesiredVersion: "v2", Source: "manual", UpdatedAt: time.Now().UTC()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/desired-state", nil)
	w := httptest.NewRecorder()

	ListDesiredState(logger, mem).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp DesiredStateListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp.Groups) == 0 || len(resp.Devices) == 0 {
		t.Fatalf("expected groups and devices")
	}
}

func TestDeviceCheckin_AgentDesiredOverrides(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	deviceID := uuid.NewString()
	groupID := uuid.NewString()
	_ = mem.UpsertGroup(store.Group{GroupID: groupID, SelectorJSON: []byte(`{"region":"west"}`)})
	_ = mem.UpsertDesiredStateGroup(store.DesiredStateGroup{GroupID: groupID, DesiredVersion: "v3", UpdatedAt: time.Now().UTC()})
	_ = mem.UpsertDesiredStateDevice(store.DesiredStateDevice{DeviceID: deviceID, DesiredVersion: "v2", Source: "manual", UpdatedAt: time.Now().UTC()})

	body := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","labels":{"region":"west"},"current":{"softwareVersion":"v1","configRev":"c1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(body))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Desired == nil || resp.Desired.SoftwareVersion != "v2" {
		t.Fatalf("expected manual desired to remain")
	}
}

func TestDeviceCheckin_GroupDesiredOverridesAgent(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	deviceID := uuid.NewString()
	groupID := uuid.NewString()
	_ = mem.UpsertGroup(store.Group{GroupID: groupID, SelectorJSON: []byte(`{"region":"west"}`)})
	_ = mem.UpsertDesiredStateGroup(store.DesiredStateGroup{GroupID: groupID, DesiredVersion: "v9", CheckinInterval: 12, UpdatedAt: time.Now().UTC()})

	body := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","labels":{"region":"west"},"current":{"softwareVersion":"v1","configRev":"c1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(body))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Desired == nil || resp.Desired.SoftwareVersion != "v9" {
		t.Fatalf("expected group desired to override agent")
	}
	if resp.Desired.CheckinInterval != 12 {
		t.Fatalf("expected checkin interval from group")
	}
}
func TestDeviceCheckin_AgentSetsDesired(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	deviceID := uuid.NewString()
	body := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(body))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Desired == nil || resp.Desired.SoftwareVersion != "v1" {
		t.Fatalf("expected desired to be set by agent")
	}
}
