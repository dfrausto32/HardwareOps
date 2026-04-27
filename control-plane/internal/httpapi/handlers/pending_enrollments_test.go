package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/metrics"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestCreateEnrollmentProfile(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	reqBody := []byte(`{"name":"factory-floor-a","expiresInSec":3600,"maxUses":10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateEnrollmentProfileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if resp.ProfileID == "" || resp.BootstrapToken == "" || resp.ExpiresAt.IsZero() {
		t.Fatalf("missing expected fields: %+v", resp)
	}
}

func TestCreateEnrollmentProfile_RejectsImpossibleApprovalDelay(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	reqBody := []byte(`{"name":"factory-floor-a","expiresInSec":3600,"approvalDelaySec":3600}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAndDisableEnrollmentProfile(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"factory-floor-a","expiresInSec":3600,"maxUses":10}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var created CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/enrollment-profiles", nil)
	listW := httptest.NewRecorder()
	ListEnrollmentProfiles(logger, mem, false).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list profiles expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var listResp EnrollmentProfileListResponse
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ProfileID != created.ProfileID {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles/"+created.ProfileID+"/disable", nil)
	disableReq = withURLParam(disableReq, "profileId", created.ProfileID)
	disableW := httptest.NewRecorder()
	SetEnrollmentProfileDisabled(logger, mem, false, true).ServeHTTP(disableW, disableReq)
	if disableW.Code != http.StatusOK {
		t.Fatalf("disable profile expected 200, got %d: %s", disableW.Code, disableW.Body.String())
	}
	var disabled EnrollmentProfileListItem
	if err := json.Unmarshal(disableW.Body.Bytes(), &disabled); err != nil {
		t.Fatalf("decode disable response: %v", err)
	}
	if !disabled.Disabled {
		t.Fatalf("expected profile to be disabled: %+v", disabled)
	}
}

func TestRotateEnrollmentProfileToken(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"factory-floor-a","expiresInSec":3600,"maxUses":10}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var created CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rotateReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles/"+created.ProfileID+"/rotate", nil)
	rotateReq = withURLParam(rotateReq, "profileId", created.ProfileID)
	rotateW := httptest.NewRecorder()
	RotateEnrollmentProfileToken(logger, mem, false).ServeHTTP(rotateW, rotateReq)
	if rotateW.Code != http.StatusOK {
		t.Fatalf("rotate profile token expected 200, got %d: %s", rotateW.Code, rotateW.Body.String())
	}
	var rotated EnrollmentProfileTokenResponse
	if err := json.Unmarshal(rotateW.Body.Bytes(), &rotated); err != nil {
		t.Fatalf("decode rotate response: %v", err)
	}
	if rotated.BootstrapToken == "" || rotated.BootstrapToken == created.BootstrapToken {
		t.Fatalf("expected a new bootstrap token, got %+v", rotated)
	}

	oldProfile, err := mem.GetEnrollmentProfileByTokenHash(hashToken(created.BootstrapToken))
	if err == nil || oldProfile.ProfileID != "" {
		t.Fatalf("expected old bootstrap token to be invalid, got %+v, err=%v", oldProfile, err)
	}
	newProfile, err := mem.GetEnrollmentProfileByTokenHash(hashToken(rotated.BootstrapToken))
	if err != nil {
		t.Fatalf("expected rotated bootstrap token to resolve, got %v", err)
	}
	if newProfile.ProfileID != created.ProfileID {
		t.Fatalf("expected rotated token to map to %s, got %s", created.ProfileID, newProfile.ProfileID)
	}
}

func TestRotateEnrollmentProfileToken_WithGraceWindow(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"factory-floor-a","expiresInSec":3600,"maxUses":10}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var created CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rotatePayload := []byte(`{"reason":"routine_rotation","gracePeriodSec":300}`)
	rotateReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles/"+created.ProfileID+"/rotate", bytes.NewReader(rotatePayload))
	rotateReq = withURLParam(rotateReq, "profileId", created.ProfileID)
	rotateW := httptest.NewRecorder()
	RotateEnrollmentProfileToken(logger, mem, false).ServeHTTP(rotateW, rotateReq)
	if rotateW.Code != http.StatusOK {
		t.Fatalf("rotate profile token expected 200, got %d: %s", rotateW.Code, rotateW.Body.String())
	}
	var rotated EnrollmentProfileTokenResponse
	if err := json.Unmarshal(rotateW.Body.Bytes(), &rotated); err != nil {
		t.Fatalf("decode rotate response: %v", err)
	}
	if rotated.PreviousTokenGraceUntil.IsZero() {
		t.Fatalf("expected previousTokenGraceUntil to be set")
	}
	if rotated.PreviousTokenGraceUntil.Before(time.Now().UTC()) {
		t.Fatalf("expected grace to be in the future: %s", rotated.PreviousTokenGraceUntil)
	}

	if _, err := mem.GetEnrollmentProfileByTokenHash(hashToken(created.BootstrapToken)); err != nil {
		t.Fatalf("expected old token to remain valid within grace window, got %v", err)
	}
	if _, err := mem.GetEnrollmentProfileByTokenHash(hashToken(rotated.BootstrapToken)); err != nil {
		t.Fatalf("expected new token to resolve, got %v", err)
	}
}

func TestRotateEnrollmentProfileToken_RejectsExcessiveGraceWindow(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"factory-floor-a","expiresInSec":3600,"maxUses":10}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var created CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	rotatePayload := []byte(`{"gracePeriodSec":86401}`)
	rotateReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles/"+created.ProfileID+"/rotate", bytes.NewReader(rotatePayload))
	rotateReq = withURLParam(rotateReq, "profileId", created.ProfileID)
	rotateW := httptest.NewRecorder()
	RotateEnrollmentProfileToken(logger, mem, false).ServeHTTP(rotateW, rotateReq)
	if rotateW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for excessive grace, got %d: %s", rotateW.Code, rotateW.Body.String())
	}
}

func TestUpdateEnrollmentProfile(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{
		"name":"factory-floor-a",
		"expiresInSec":3600,
		"maxUses":10,
		"challengeSecret":"old-secret",
		"challengeHint":"old hint",
		"approvalDelaySec":5,
		"defaultLabels":{"site":"a"}
	}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var created CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/enrollment-profiles/"+created.ProfileID, bytes.NewReader([]byte(`{
		"name":"factory-floor-b",
		"maxUses":25,
		"allowUnsignedHardwareIdentity":true,
		"approvalDelaySec":11,
		"defaultLabels":{"site":"b","role":"kiosk"},
		"clearChallenge":true
	}`)))
	updateReq = withURLParam(updateReq, "profileId", created.ProfileID)
	updateW := httptest.NewRecorder()
	UpdateEnrollmentProfile(logger, mem, false).ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update profile expected 200, got %d: %s", updateW.Code, updateW.Body.String())
	}
	var updated EnrollmentProfileListItem
	if err := json.Unmarshal(updateW.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Name != "factory-floor-b" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if updated.MaxUses != 25 {
		t.Fatalf("expected maxUses=25, got %d", updated.MaxUses)
	}
	if !updated.AllowUntrustedHW {
		t.Fatalf("expected allowUnsignedHardwareIdentity=true")
	}
	if updated.ApprovalDelaySec != 11 {
		t.Fatalf("expected approvalDelaySec=11, got %d", updated.ApprovalDelaySec)
	}
	if updated.ChallengeEnabled {
		t.Fatalf("expected challenge to be cleared")
	}

	stored, err := mem.GetEnrollmentProfile(created.ProfileID)
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if string(stored.DefaultLabelsJSON) != `{"site":"b","role":"kiosk"}` && string(stored.DefaultLabelsJSON) != `{"role":"kiosk","site":"b"}` {
		t.Fatalf("unexpected default labels: %s", string(stored.DefaultLabelsJSON))
	}
}

func TestRequestPendingEnrollmentAndList(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var profileResp CreateEnrollmentProfileResponse
	if err := json.Unmarshal(createW.Body.Bytes(), &profileResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	csr := string(mustCSR(t))
	payload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          csr,
		AgentVersion: "0.1.0",
		Capabilities: json.RawMessage(`{"hw":{"identity":{"id":"device-abc12345","source":"machine-id"}}}`),
		Metadata:     json.RawMessage(`{"hostname":"kiosk-17"}`),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(body))
	w := httptest.NewRecorder()

	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp PendingEnrollmentRequestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode pending response: %v", err)
	}
	if resp.RequestID == "" || resp.ClaimToken == "" || resp.Status != "pending" {
		t.Fatalf("unexpected pending response: %+v", resp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pending-enrollments?status=pending", nil)
	listW := httptest.NewRecorder()
	ListPendingEnrollments(logger, mem, false, nil).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var listResp PendingEnrollmentListResponse
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 pending item, got %d", len(listResp.Items))
	}
	if listResp.Items[0].RequestID != resp.RequestID {
		t.Fatalf("expected request id %s, got %s", resp.RequestID, listResp.Items[0].RequestID)
	}
}

func TestRequestPendingEnrollment_InvalidProfileToken(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	payload := PendingEnrollmentRequestPayload{
		ProfileToken: "bad-token",
		CSR:          string(mustCSR(t)),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(body))
	w := httptest.NewRecorder()

	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveAndClaimPendingEnrollment(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
		AgentVersion: "0.1.0",
		Capabilities: json.RawMessage(`{"hw":{"identity":{"id":"device-claim123","source":"machine-id"}}}`),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusAccepted {
		t.Fatalf("request expected 202, got %d: %s", requestW.Code, requestW.Body.String())
	}
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

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
	if claimResp.Status != "issued" || claimResp.DeviceID == "" || claimResp.CertPEM == "" {
		t.Fatalf("unexpected claim response: %+v", claimResp)
	}
	if _, ok, _ := mem.GetDevice(claimResp.DeviceID); !ok {
		t.Fatalf("expected enrolled device to exist")
	}
}

func TestClaimPendingEnrollment_AppliesProfileDefaultLabels(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

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
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

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
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

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

	device, ok, err := mem.GetDevice(claimResp.DeviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if !ok {
		t.Fatalf("expected issued device to exist")
	}
	if string(device.LabelsJSON) != `{"site":"factory-a","role":"kiosk"}` && string(device.LabelsJSON) != `{"role":"kiosk","site":"factory-a"}` {
		t.Fatalf("expected default labels on issued device, got %s", string(device.LabelsJSON))
	}
}

func TestClaimPendingEnrollmentWhilePending(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

	claimPayload := ClaimPendingEnrollmentPayload{
		RequestID:  pendingResp.RequestID,
		ClaimToken: pendingResp.ClaimToken,
	}
	claimBody, _ := json.Marshal(claimPayload)
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/claim", bytes.NewReader(claimBody))
	claimW := httptest.NewRecorder()
	ClaimPendingEnrollment(logger, mem, nil, &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}, DeviceIdentityPolicy{}, false, nil, nil).ServeHTTP(claimW, claimReq)
	if claimW.Code != http.StatusAccepted {
		t.Fatalf("claim expected 202, got %d: %s", claimW.Code, claimW.Body.String())
	}
}

func TestDenyPendingEnrollmentThenClaim(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

	denyReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/deny", bytes.NewReader([]byte(`{"reason":"unknown hardware"}`)))
	denyReq = withURLParam(denyReq, "requestId", pendingResp.RequestID)
	denyW := httptest.NewRecorder()
	DenyPendingEnrollment(logger, mem, false, nil).ServeHTTP(denyW, denyReq)
	if denyW.Code != http.StatusOK {
		t.Fatalf("deny expected 200, got %d: %s", denyW.Code, denyW.Body.String())
	}

	claimPayload := ClaimPendingEnrollmentPayload{
		RequestID:  pendingResp.RequestID,
		ClaimToken: pendingResp.ClaimToken,
	}
	claimBody, _ := json.Marshal(claimPayload)
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/claim", bytes.NewReader(claimBody))
	claimW := httptest.NewRecorder()
	ClaimPendingEnrollment(logger, mem, nil, &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}, DeviceIdentityPolicy{}, false, nil, nil).ServeHTTP(claimW, claimReq)
	if claimW.Code != http.StatusGone {
		t.Fatalf("claim expected 410, got %d: %s", claimW.Code, claimW.Body.String())
	}
}

func TestResetPendingEnrollment(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

	denyReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/deny", bytes.NewReader([]byte(`{"reason":"unknown hardware"}`)))
	denyReq = withURLParam(denyReq, "requestId", pendingResp.RequestID)
	denyW := httptest.NewRecorder()
	DenyPendingEnrollment(logger, mem, false, nil).ServeHTTP(denyW, denyReq)
	if denyW.Code != http.StatusOK {
		t.Fatalf("deny expected 200, got %d: %s", denyW.Code, denyW.Body.String())
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/reset", nil)
	resetReq = withURLParam(resetReq, "requestId", pendingResp.RequestID)
	resetW := httptest.NewRecorder()
	ResetPendingEnrollment(logger, mem, false, nil).ServeHTTP(resetW, resetReq)
	if resetW.Code != http.StatusOK {
		t.Fatalf("reset expected 200, got %d: %s", resetW.Code, resetW.Body.String())
	}
	var resetResp ResetPendingEnrollmentResponse
	if err := json.Unmarshal(resetW.Body.Bytes(), &resetResp); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if resetResp.Status != "pending" || resetResp.ExpiresAt.IsZero() {
		t.Fatalf("unexpected reset response: %+v", resetResp)
	}
}

func TestClaimConflictPendingEnrollment(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
		Capabilities: json.RawMessage(`{"hw":{"identity":{"id":"device-dup","source":"machine-id"}}}`),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusAccepted {
		t.Fatalf("request expected 202, got %d: %s", requestW.Code, requestW.Body.String())
	}
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/approve", nil)
	approveReq = withURLParam(approveReq, "requestId", pendingResp.RequestID)
	approveW := httptest.NewRecorder()
	ApprovePendingEnrollment(logger, mem, false, nil).ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve expected 200, got %d: %s", approveW.Code, approveW.Body.String())
	}

	if err := mem.CreateDevice(store.Device{
		DeviceID:     "device-existing",
		Status:       "active",
		MetadataJSON: []byte(`{"hwops":{"identity":{"hardwareId":"device-dup","source":"machine-id"}}}`),
	}); err != nil {
		t.Fatalf("create existing device: %v", err)
	}

	claimPayload := ClaimPendingEnrollmentPayload{
		RequestID:  pendingResp.RequestID,
		ClaimToken: pendingResp.ClaimToken,
	}
	claimBody, _ := json.Marshal(claimPayload)
	claimReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/claim", bytes.NewReader(claimBody))
	claimW := httptest.NewRecorder()
	ClaimPendingEnrollment(logger, mem, nil, &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}, DeviceIdentityPolicy{}, false, nil, nil).ServeHTTP(claimW, claimReq)
	if claimW.Code != http.StatusConflict {
		t.Fatalf("claim expected 409, got %d: %s", claimW.Code, claimW.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/pending-enrollments?status=conflict", nil)
	listW := httptest.NewRecorder()
	ListPendingEnrollments(logger, mem, false, nil).ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list conflict expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var listResp PendingEnrollmentListResponse
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode conflict list: %v", err)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].Status != "conflict" {
		t.Fatalf("unexpected conflict list: %+v", listResp)
	}
}

func TestRequestPendingEnrollment_ChallengeRequired(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{
		"name":"line-a",
		"expiresInSec":3600,
		"challengeSecret":"shared-secret",
		"challengeHint":"shared"
	}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create profile expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusUnauthorized {
		t.Fatalf("request without challenge expected 401, got %d: %s", requestW.Code, requestW.Body.String())
	}

	requestPayload.ChallengeResponse = "shared-secret"
	requestBody, _ = json.Marshal(requestPayload)
	requestReq = httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW = httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, nil, nil, nil).ServeHTTP(requestW, requestReq)
	if requestW.Code != http.StatusAccepted {
		t.Fatalf("request with challenge expected 202, got %d: %s", requestW.Code, requestW.Body.String())
	}
}

func TestRequestPendingEnrollment_RateLimitedPerSource(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	metricsCollector := metrics.New()
	guard := NewPendingEnrollmentGuard(PendingEnrollmentGuardConfig{
		RequestRPMPerSource: 1,
	}, metricsCollector)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	req1.RemoteAddr = "198.51.100.10:1234"
	w1 := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, metricsCollector, nil, guard).ServeHTTP(w1, req1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first request expected 202, got %d: %s", w1.Code, w1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	req2.RemoteAddr = "198.51.100.10:9999"
	w2 := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, metricsCollector, nil, guard).ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request expected 429, got %d: %s", w2.Code, w2.Body.String())
	}
	body := scrapeMetrics(t, metricsCollector)
	if got := metricValue(t, body, `hwops_pending_enroll_throttle_total{reason="pending_enroll_source"}`); got != 1 {
		t.Fatalf("pending_enroll_source throttle expected 1, got %v", got)
	}
}

func TestRequestPendingEnrollment_QueueFullPerSource(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	metricsCollector := metrics.New()
	guard := NewPendingEnrollmentGuard(PendingEnrollmentGuardConfig{
		MaxActivePerSource: 1,
	}, metricsCollector)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{"name":"line-a","expiresInSec":3600}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	req1.RemoteAddr = "198.51.100.11:1234"
	w1 := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, metricsCollector, nil, guard).ServeHTTP(w1, req1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first request expected 202, got %d: %s", w1.Code, w1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	req2.RemoteAddr = "198.51.100.11:9999"
	w2 := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, metricsCollector, nil, guard).ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request expected 429, got %d: %s", w2.Code, w2.Body.String())
	}
	body := scrapeMetrics(t, metricsCollector)
	if got := metricValue(t, body, `hwops_pending_enroll_throttle_total{reason="queue_full_source"}`); got != 1 {
		t.Fatalf("queue_full_source throttle expected 1, got %v", got)
	}
}

func TestApprovePendingEnrollment_ApprovalDelay(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	metricsCollector := metrics.New()

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", bytes.NewReader([]byte(`{
		"name":"line-a",
		"expiresInSec":3600,
		"approvalDelaySec":600
	}`)))
	createW := httptest.NewRecorder()
	CreateEnrollmentProfile(logger, mem, false).ServeHTTP(createW, createReq)
	var profileResp CreateEnrollmentProfileResponse
	_ = json.Unmarshal(createW.Body.Bytes(), &profileResp)

	requestPayload := PendingEnrollmentRequestPayload{
		ProfileToken: profileResp.BootstrapToken,
		CSR:          string(mustCSR(t)),
	}
	requestBody, _ := json.Marshal(requestPayload)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/request", bytes.NewReader(requestBody))
	requestW := httptest.NewRecorder()
	RequestPendingEnrollment(logger, mem, DeviceIdentityPolicy{}, false, metricsCollector, nil, nil).ServeHTTP(requestW, requestReq)
	var pendingResp PendingEnrollmentRequestResponse
	_ = json.Unmarshal(requestW.Body.Bytes(), &pendingResp)

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+pendingResp.RequestID+"/approve", nil)
	approveReq = withURLParam(approveReq, "requestId", pendingResp.RequestID)
	approveW := httptest.NewRecorder()
	ApprovePendingEnrollment(logger, mem, false, metricsCollector).ServeHTTP(approveW, approveReq)
	if approveW.Code != http.StatusTooManyRequests {
		t.Fatalf("approve expected 429, got %d: %s", approveW.Code, approveW.Body.String())
	}
	body := scrapeMetrics(t, metricsCollector)
	if got := metricValue(t, body, `hwops_pending_enroll_throttle_total{reason="approval_delay"}`); got != 1 {
		t.Fatalf("approval_delay throttle expected 1, got %v", got)
	}
}

func TestRefreshPendingEnrollmentMetrics_QueueAgeBuckets(t *testing.T) {
	mem := memory.New()
	metricsCollector := metrics.New()
	now := time.Now().UTC().Truncate(time.Second)

	profileToken := "profile-token"
	profile := store.EnrollmentProfile{
		ProfileID:        uuid.NewString(),
		Name:             "line-a",
		TokenHash:        hashToken(profileToken),
		RequireApproval:  true,
		ExpiresAt:        now.Add(time.Hour),
		CreatedAt:        now,
		ApprovalDelaySec: 0,
	}
	if err := mem.CreateEnrollmentProfile(profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	createPending := func(createdAt, expiresAt time.Time) {
		t.Helper()
		_, err := mem.CreatePendingEnrollmentForProfileToken(hashToken(profileToken), store.PendingEnrollment{
			RequestID:      uuid.NewString(),
			Status:         "pending",
			CSR:            "csr",
			ClaimTokenHash: hashToken(uuid.NewString()),
			CreatedAt:      createdAt,
			ExpiresAt:      expiresAt,
		})
		if err != nil {
			t.Fatalf("create pending enrollment: %v", err)
		}
	}

	createPending(now.Add(-30*time.Second), now.Add(10*time.Minute))
	createPending(now.Add(-2*time.Minute), now.Add(10*time.Minute))
	createPending(now.Add(-7*time.Minute), now.Add(10*time.Minute))
	createPending(now.Add(-30*time.Minute), now.Add(10*time.Minute))
	createPending(now.Add(-2*time.Hour), now.Add(10*time.Minute))
	createPending(now.Add(-45*time.Second), now.Add(-1*time.Minute))

	refreshPendingEnrollmentMetrics(mem, metricsCollector, now)
	body := scrapeMetrics(t, metricsCollector)

	if got := metricValue(t, body, "hwops_pending_enroll_active_total"); got != 5 {
		t.Fatalf("active pending expected 5, got %v", got)
	}
	if got := metricValue(t, body, `hwops_pending_enroll_queue_age_total{bucket="lt_1m"}`); got != 1 {
		t.Fatalf("lt_1m expected 1, got %v", got)
	}
	if got := metricValue(t, body, `hwops_pending_enroll_queue_age_total{bucket="1m_5m"}`); got != 1 {
		t.Fatalf("1m_5m expected 1, got %v", got)
	}
	if got := metricValue(t, body, `hwops_pending_enroll_queue_age_total{bucket="5m_15m"}`); got != 1 {
		t.Fatalf("5m_15m expected 1, got %v", got)
	}
	if got := metricValue(t, body, `hwops_pending_enroll_queue_age_total{bucket="15m_1h"}`); got != 1 {
		t.Fatalf("15m_1h expected 1, got %v", got)
	}
	if got := metricValue(t, body, `hwops_pending_enroll_queue_age_total{bucket="gte_1h"}`); got != 1 {
		t.Fatalf("gte_1h expected 1, got %v", got)
	}
	oldest := metricValue(t, body, "hwops_pending_enroll_oldest_age_seconds")
	if oldest < 7199 || oldest > 7201 {
		t.Fatalf("oldest age expected ~7200s, got %v", oldest)
	}
}

func scrapeMetrics(t *testing.T, metricsCollector *metrics.Metrics) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	metricsCollector.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("scrape metrics expected 200, got %d", w.Code)
	}
	return w.Body.String()
}

func metricValue(t *testing.T, body, linePrefix string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, linePrefix+" ") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, linePrefix))
			val, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				t.Fatalf("parse metric %q: %v", linePrefix, err)
			}
			return val
		}
	}
	t.Fatalf("metric line not found: %s", linePrefix)
	return 0
}
