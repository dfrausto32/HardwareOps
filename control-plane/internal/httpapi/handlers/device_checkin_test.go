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
	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestDeviceCheckin_Valid(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},"currentComponents":{"agent_bundle":{"softwareVersion":"v1","configRev":"c1"}}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, deviceID = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

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

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

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

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDeviceCheckin_CloneSignalOnRapidSourceIPSwitch(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, deviceID = attachMTLSDevice(t, mem, req, deviceID)
	req.RemoteAddr = "10.0.0.2:12345"

	device, ok, err := mem.GetDevice(deviceID)
	if err != nil || !ok {
		t.Fatalf("seed device read failed: %v", err)
	}
	device.LastSeen = time.Now().UTC()
	device.MetadataJSON = []byte(`{"hwops":{"identity":{"lastSourceIP":"10.0.0.1","suspectedCloneCount":1}}}`)
	if err := mem.UpsertDevice(device); err != nil {
		t.Fatalf("seed device update failed: %v", err)
	}

	w := httptest.NewRecorder()
	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	eventsRows, err := mem.ListRuntimeEvents(store.RuntimeEventFilter{
		Type:     events.TypeDeviceCloneSuspected,
		DeviceID: deviceID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(eventsRows) != 1 {
		t.Fatalf("expected one clone signal event, got %d", len(eventsRows))
	}

	updated, ok, err := mem.GetDevice(deviceID)
	if err != nil || !ok {
		t.Fatalf("read updated device failed: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(updated.MetadataJSON, &meta); err != nil {
		t.Fatalf("metadata decode failed: %v", err)
	}
	hwops, _ := meta["hwops"].(map[string]any)
	identity, _ := hwops["identity"].(map[string]any)
	if identity["lastSourceIP"] != "10.0.0.2" {
		t.Fatalf("expected lastSourceIP updated, got %#v", identity["lastSourceIP"])
	}
	if int(identity["suspectedCloneCount"].(float64)) != 2 {
		t.Fatalf("expected suspectedCloneCount=2, got %#v", identity["suspectedCloneCount"])
	}
}

func TestDeviceCheckin_RejectsHardwareIdentityReuseInEnforceMode(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	otherDeviceID := uuid.NewString()
	hardwareID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := mem.CreateDevice(store.Device{
		DeviceID:     otherDeviceID,
		Status:       "active",
		LastSeen:     time.Now().UTC(),
		MetadataJSON: []byte(`{"hwops":{"identity":{"hardwareId":"` + hardwareID + `"}}}`),
	}); err != nil {
		t.Fatalf("seed device failed: %v", err)
	}

	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},"capabilities":{"hw":{"identity":{"id":"` + hardwareID + `","source":"machine-id"}}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{Mode: "enforce"}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	eventsRows, err := mem.ListRuntimeEvents(store.RuntimeEventFilter{
		Type:     events.TypeDeviceIdentityConflict,
		DeviceID: deviceID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(eventsRows) != 1 {
		t.Fatalf("expected one identity conflict event, got %d", len(eventsRows))
	}
}

func TestDeviceCheckin_AppliesSignaturePolicyDefaults(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	desiredArtifactID := uuid.NewString()
	now := time.Now().UTC()
	if err := mem.UpsertDesiredStateDevice(store.DesiredStateDevice{
		DeviceID:       deviceID,
		ArtifactID:     desiredArtifactID,
		DesiredVersion: "1.2.3",
		ComponentsJSON: nil,
		Source:         "manual",
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed desired state failed: %v", err)
	}

	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{
		Require: true,
		KeyID:   "sha256:test-signing-key",
	}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Desired == nil {
		t.Fatalf("expected desired")
	}
	comp, ok := resp.Desired.Components["app_bundle"]
	if !ok {
		t.Fatalf("expected app_bundle desired component")
	}
	var policy map[string]any
	if err := json.Unmarshal(comp.ApplyPolicy, &policy); err != nil {
		t.Fatalf("applyPolicy json decode failed: %v", err)
	}
	if policy["requireSignature"] != true {
		t.Fatalf("expected requireSignature=true, got %#v", policy["requireSignature"])
	}
	if policy["signingKeyId"] != "sha256:test-signing-key" {
		t.Fatalf("expected signingKeyId default, got %#v", policy["signingKeyId"])
	}
}

func TestDeviceCheckin_ImmediateRecheckinConsumesPendingTrigger(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)

	if err := mem.UpsertDeployTrigger(store.DeployTrigger{
		DeviceID:    deviceID,
		TriggeredBy: "test",
		TriggeredAt: time.Now().UTC(),
		Reason:      "ci",
	}); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if !resp.ImmediateRecheckin {
		t.Fatal("expected immediateRecheckin=true")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req2 = attachExistingMTLSDevice(t, mem, req2, deviceID)
	w2 := httptest.NewRecorder()
	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on second check-in, got %d", w2.Code)
	}
	var resp2 DeviceCheckinResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp2.ImmediateRecheckin {
		t.Fatal("expected immediateRecheckin to be consumed after one check-in")
	}
}

func TestDeviceCheckin_NoPendingTriggerOmitsImmediateRecheckin(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	deviceID := uuid.NewString()
	reqBody := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(reqBody))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceCheckinResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.ImmediateRecheckin {
		t.Fatal("expected immediateRecheckin=false without pending trigger")
	}
	if bytes.Contains(w.Body.Bytes(), []byte(`"immediateRecheckin"`)) {
		t.Fatalf("expected immediateRecheckin to be omitted from JSON: %s", w.Body.String())
	}
}
