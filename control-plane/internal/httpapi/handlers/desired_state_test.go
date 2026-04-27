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
	"github.com/parcel/control-plane/internal/artifacttrust"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
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

func TestPutDesiredStateDevice_RejectsUnverifiedArtifactWhenStrictTrustPolicyEnabled(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID:          artifactID,
		Name:                "agent",
		Version:             "1.0.0",
		Type:                "agent_bundle",
		ObjectKey:           "artifacts/test.tar.gz",
		SHA256:              "abc",
		SizeBytes:           3,
		Status:              "active",
		VerificationStatus:  artifacttrust.VerificationStatusLegacy,
		CreatedAt:           time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := mem.SetArtifactTrustPolicy(store.ArtifactTrustPolicy{
		VerificationMode: artifacttrust.VerificationModeRequire,
	}); err != nil {
		t.Fatalf("set trust policy: %v", err)
	}

	id := uuid.NewString()
	body := []byte(`{"components":{"agent_bundle":{"artifactId":"` + artifactID + `","artifactType":"agent_bundle","desiredVersion":"1.0.0"}}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/desired-state/devices/"+id, bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	w := httptest.NewRecorder()

	PutDesiredStateDeviceWithPolicy(logger, mem, false, ArtifactSignaturePolicy{Store: mem}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("must be verified for strict policy")) {
		t.Fatalf("expected trust policy rejection, got %s", w.Body.String())
	}
}

func TestListDesiredState(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	_ = mem.UpsertDesiredStateGroup(store.DesiredStateGroup{GroupID: uuid.NewString(), DesiredVersion: "v1", UpdatedAt: time.Now().UTC()})
	_ = mem.UpsertDesiredStateDevice(store.DesiredStateDevice{DeviceID: uuid.NewString(), DesiredVersion: "v2", Source: "manual", UpdatedAt: time.Now().UTC()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/desired-state", nil)
	w := httptest.NewRecorder()

	ListDesiredState(logger, mem, false).ServeHTTP(w, req)

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

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

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

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

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

func TestDeviceCheckin_GroupDesiredResolvesFromEnrollmentProfileLabels(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	groupID := uuid.NewString()
	if err := mem.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "factory-kiosk",
		SelectorJSON: []byte(`{"site":"factory-a","role":"kiosk"}`),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed group failed: %v", err)
	}
	if err := mem.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:         groupID,
		DesiredVersion:  "v9",
		CheckinInterval: 12,
		UpdatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed group desired failed: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{
		"name":"line-a",
		"expiresInSec":3600,
		"defaultLabels":{"site":"factory-a","role":"kiosk"}
	}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var profileResp CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &profileResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
		AgentVersion: "0.1.0",
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusAccepted {
		t.Fatalf("request expected 202, got %d: %s", requestW.Code, requestW.Body.String())
	}
	var pendingResp PendingEnrollmentRequestResponse
	if err := json.Unmarshal(requestW.Body.Bytes(), &pendingResp); err != nil {
		t.Fatalf("decode request response: %v", err)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/approve", nil)
	approveReq = withURLParam(approveReq, "requestId", pendingResp.RequestID)
	approveW := httptest.NewRecorder()
	ApprovePendingEnrollment(logger, mem, false, nil).ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve expected 200, got %d: %s", approveW.Code, approveW.Body.String())
	}

	claimPayload := ClaimPendingEnrollmentPayload{
		RequestID:  pendingResp.RequestID,
		ClaimToken: pendingResp.ClaimToken,
	}
	claimBody, _ := json.Marshal(claimPayload)
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/claim", bytes.NewReader(claimBody))
	claimW := httptest.NewRecorder()
	ClaimPendingEnrollment(logger, mem, nil, &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}, DeviceIdentityPolicy{}, false, nil, nil).ServeHTTP(claimW, claimReq)
	if claimW.Code != http.StatusOK {
		t.Fatalf("claim expected 200, got %d: %s", claimW.Code, claimW.Body.String())
	}
	var claimResp ClaimPendingEnrollmentResponse
	if err := json.Unmarshal(claimW.Body.Bytes(), &claimResp); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}

	body := []byte(`{"deviceId":"` + claimResp.DeviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"}}`)
	checkinReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(body))
	checkinReq = attachExistingMTLSDevice(t, mem, checkinReq, claimResp.DeviceID)
	checkinW := httptest.NewRecorder()

	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(checkinW, checkinReq)
	if checkinW.Code != http.StatusOK {
		t.Fatalf("checkin expected 200, got %d: %s", checkinW.Code, checkinW.Body.String())
	}

	var resp DeviceCheckinResponse
	if err := json.Unmarshal(checkinW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode checkin response: %v", err)
	}
	if resp.Desired == nil || resp.Desired.SoftwareVersion != "v9" {
		t.Fatalf("expected group desired version from profile labels, got %+v", resp.Desired)
	}
	if resp.Desired.Source != "group" {
		t.Fatalf("expected desired source=group, got %q", resp.Desired.Source)
	}
	if resp.Desired.CheckinInterval != 12 {
		t.Fatalf("expected checkin interval from group, got %d", resp.Desired.CheckinInterval)
	}

	device, ok, err := mem.GetDevice(claimResp.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if !ok {
		t.Fatalf("expected issued device to exist")
	}
	if string(device.LabelsJSON) != `{"site":"factory-a","role":"kiosk"}` && string(device.LabelsJSON) != `{"role":"kiosk","site":"factory-a"}` {
		t.Fatalf("expected profile labels to persist through first checkin, got %s", string(device.LabelsJSON))
	}
}

func TestDeviceCheckin_AgentSetsDesired(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	deviceID := uuid.NewString()
	body := []byte(`{"deviceId":"` + deviceID + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},"currentComponents":{"agent_bundle":{"softwareVersion":"v1","configRev":"c1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", bytes.NewReader(body))
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
	if resp.Desired == nil || resp.Desired.Components == nil {
		t.Fatalf("expected desired to be set by agent")
	}
	comp, ok := resp.Desired.Components["agent_bundle"]
	if !ok || comp.SoftwareVersion != "v1" {
		t.Fatalf("expected desired component to be set by agent")
	}
}
