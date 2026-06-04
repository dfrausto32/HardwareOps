package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
)

func TestListGlobalEnrollmentProfiles_Empty(t *testing.T) {
	st := newFakeEnrollmentStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enrollment-profiles", nil)
	w := httptest.NewRecorder()

	ListGlobalEnrollmentProfiles(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, "[") {
		t.Fatalf("expected JSON array, got: %s", body)
	}
}

func TestCreateGlobalEnrollmentProfile_MissingName(t *testing.T) {
	st := newFakeEnrollmentStore()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", strings.NewReader(`{"maxUses":5}`))
	w := httptest.NewRecorder()

	CreateGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateGlobalEnrollmentProfile_Valid(t *testing.T) {
	st := newFakeEnrollmentStore()
	body := `{"name":"factory-floor","requireApproval":true,"maxUses":100,"certValidityDays":730}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p globalplane.GlobalEnrollmentProfile
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if p.ProfileID == "" || p.Name != "factory-floor" {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if p.MaxUses != 100 || p.CertValidityDays != 730 {
		t.Fatalf("unexpected profile fields: maxUses=%d certValidityDays=%d", p.MaxUses, p.CertValidityDays)
	}
}

func TestCreateGlobalEnrollmentProfile_DefaultsApprovalAndCertValidity(t *testing.T) {
	st := newFakeEnrollmentStore()
	// requireApproval and certValidityDays omitted — should get defaults.
	body := `{"name":"minimal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollment-profiles", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p globalplane.GlobalEnrollmentProfile
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if !p.RequireApproval {
		t.Fatal("requireApproval should default to true")
	}
	if p.CertValidityDays != 365 {
		t.Fatalf("certValidityDays should default to 365, got %d", p.CertValidityDays)
	}
}

func TestUpdateGlobalEnrollmentProfile_NotFound(t *testing.T) {
	st := newFakeEnrollmentStore()
	pid := uuid.NewString()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/enrollment-profiles/"+pid, strings.NewReader(`{"name":"new"}`))
	req = withURLParam(req, "profileId", pid)
	w := httptest.NewRecorder()

	UpdateGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateGlobalEnrollmentProfile_Valid(t *testing.T) {
	st := newFakeEnrollmentStore()
	p, _ := st.CreateGlobalEnrollmentProfile(globalplane.GlobalEnrollmentProfile{
		Name: "original", RequireApproval: true, CertValidityDays: 365,
	})

	body := `{"name":"updated","requireApproval":false,"certValidityDays":180,"maxUses":50}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/enrollment-profiles/"+p.ProfileID, strings.NewReader(body))
	req = withURLParam(req, "profileId", p.ProfileID)
	w := httptest.NewRecorder()

	UpdateGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated globalplane.GlobalEnrollmentProfile
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updated.Name != "updated" || updated.RequireApproval || updated.CertValidityDays != 180 {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}
}

func TestDeleteGlobalEnrollmentProfile_NotFound(t *testing.T) {
	st := newFakeEnrollmentStore()
	pid := uuid.NewString()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/enrollment-profiles/"+pid, nil)
	req = withURLParam(req, "profileId", pid)
	w := httptest.NewRecorder()

	DeleteGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteGlobalEnrollmentProfile_Valid(t *testing.T) {
	st := newFakeEnrollmentStore()
	p, _ := st.CreateGlobalEnrollmentProfile(globalplane.GlobalEnrollmentProfile{Name: "to-delete"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/enrollment-profiles/"+p.ProfileID, nil)
	req = withURLParam(req, "profileId", p.ProfileID)
	w := httptest.NewRecorder()

	DeleteGlobalEnrollmentProfile(st, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if _, ok, _ := st.GetGlobalEnrollmentProfile(p.ProfileID); ok {
		t.Fatal("profile should have been removed from store")
	}
}

func TestListGlobalPendingEnrollments_FiltersStatus(t *testing.T) {
	st := newFakeEnrollmentStore()
	planeID := uuid.NewString()
	st.pendingEnrollments = []globalplane.GlobalPendingEnrollment{
		{PlaneID: planeID, RequestID: "req-1", Status: "pending"},
		{PlaneID: planeID, RequestID: "req-2", Status: "approved"},
		{PlaneID: planeID, RequestID: "req-3", Status: "pending"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pending-enrollments?status=pending", nil)
	w := httptest.NewRecorder()

	ListGlobalPendingEnrollments(st).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var items []globalplane.GlobalPendingEnrollment
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 pending items, got %d", len(items))
	}
	for _, item := range items {
		if item.Status != "pending" {
			t.Fatalf("unexpected status %q in filtered results", item.Status)
		}
	}
}

func TestListGlobalPendingEnrollments_NoFilter(t *testing.T) {
	st := newFakeEnrollmentStore()
	planeID := uuid.NewString()
	st.pendingEnrollments = []globalplane.GlobalPendingEnrollment{
		{PlaneID: planeID, RequestID: "req-1", Status: "pending"},
		{PlaneID: planeID, RequestID: "req-2", Status: "denied"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pending-enrollments", nil)
	w := httptest.NewRecorder()

	ListGlobalPendingEnrollments(st).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var items []globalplane.GlobalPendingEnrollment
	_ = json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 2 {
		t.Fatalf("expected 2 items with no filter, got %d", len(items))
	}
}

func TestApproveGlobalPendingEnrollment_RequestNotInCache(t *testing.T) {
	st := newFakeEnrollmentStore()
	// Cache is empty — request ID unknown.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/unknown-req/approve", nil)
	req = withURLParam(req, "requestId", "unknown-req")
	w := httptest.NewRecorder()

	ApproveGlobalPendingEnrollment(st, testEncKey, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestApproveGlobalPendingEnrollment_ProxiesCorrectly(t *testing.T) {
	approved := make(chan string, 1)
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/approve") {
			approved <- r.URL.Path
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer fakeRegional.Close()

	st := newFakeEnrollmentStore()
	planeID := uuid.NewString()
	enc, _ := globalplane.EncryptToken("regional-token", testEncKey)
	st.planes[planeID] = globalplane.RegionalPlane{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}
	requestID := "req-approve-test"
	st.pendingEnrollments = []globalplane.GlobalPendingEnrollment{
		{PlaneID: planeID, RequestID: requestID, Status: "pending"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+requestID+"/approve", nil)
	req = withURLParam(req, "requestId", requestID)
	w := httptest.NewRecorder()

	ApproveGlobalPendingEnrollment(st, testEncKey, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case path := <-approved:
		if !strings.Contains(path, requestID) {
			t.Fatalf("approve request sent to wrong path: %s", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("approve request did not reach regional plane")
	}
}

func TestDenyGlobalPendingEnrollment_ProxiesWithReason(t *testing.T) {
	type denyPayload struct {
		Reason string `json:"reason"`
	}
	var receivedReason string
	denied := make(chan struct{}, 1)
	fakeRegional := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/deny") {
			var p denyPayload
			_ = json.NewDecoder(r.Body).Decode(&p)
			receivedReason = p.Reason
			denied <- struct{}{}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer fakeRegional.Close()

	st := newFakeEnrollmentStore()
	planeID := uuid.NewString()
	enc, _ := globalplane.EncryptToken("regional-token", testEncKey)
	st.planes[planeID] = globalplane.RegionalPlane{
		PlaneID:        planeID,
		BaseURL:        fakeRegional.URL,
		EncryptedToken: enc,
		Enabled:        true,
	}
	requestID := "req-deny-test"
	st.pendingEnrollments = []globalplane.GlobalPendingEnrollment{
		{PlaneID: planeID, RequestID: requestID, Status: "pending"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pending-enrollments/"+requestID+"/deny",
		strings.NewReader(`{"reason":"unauthorized device"}`))
	req = withURLParam(req, "requestId", requestID)
	w := httptest.NewRecorder()

	DenyGlobalPendingEnrollment(st, testEncKey, silentLogger(), false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case <-denied:
		if receivedReason != "unauthorized device" {
			t.Fatalf("expected reason 'unauthorized device', got %q", receivedReason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deny request did not reach regional plane")
	}
}
