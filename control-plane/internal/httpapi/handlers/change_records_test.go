package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func makeArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	a := store.Artifact{
		ArtifactID: "art-1",
		Name:       "test-app",
		Version:    "1.0.0",
		Type:       "app_bundle",
		Status:     "published",
	}
	if err := mem.CreateArtifact(a); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	return a
}

func crBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func TestUpsertChangeRecord_Create(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	body := crBody(t, map[string]string{
		"safetyClass":   "ClassB",
		"impactSummary": "update core firmware",
		"riskControls":  "verified in staging",
	})
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req = withURLParam(req, "artifactId", "art-1")
	w := httptest.NewRecorder()

	UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.SafetyClass != "ClassB" {
		t.Errorf("safetyClass = %q, want ClassB", resp.SafetyClass)
	}
	if resp.Status != "draft" {
		t.Errorf("status = %q, want draft", resp.Status)
	}
	if resp.RecordID == "" {
		t.Error("recordId should be non-empty")
	}
}

func TestUpsertChangeRecord_ArtifactNotFound(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	body := crBody(t, map[string]string{"safetyClass": "ClassA"})
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req = withURLParam(req, "artifactId", "nonexistent")
	w := httptest.NewRecorder()

	UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpsertChangeRecord_InvalidSafetyClass(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	body := crBody(t, map[string]string{"safetyClass": "ClassX"})
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req = withURLParam(req, "artifactId", "art-1")
	w := httptest.NewRecorder()

	UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpsertChangeRecord_UpdateDraft(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Create
	upsert := func(sc string) *httptest.ResponseRecorder {
		b := crBody(t, map[string]string{"safetyClass": sc, "impactSummary": "x", "riskControls": "y"})
		req := withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1")
		w := httptest.NewRecorder()
		UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)
		return w
	}

	w := upsert("ClassA")
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}

	// Update — should change safetyClass
	w2 := upsert("ClassC")
	if w2.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w2.Code, w2.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w2.Body).Decode(&resp)
	if resp.SafetyClass != "ClassC" {
		t.Errorf("safetyClass = %q, want ClassC", resp.SafetyClass)
	}
}

func TestUpsertChangeRecord_CannotEditPendingApproval(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Create draft
	b := crBody(t, map[string]string{"safetyClass": "ClassB", "impactSummary": "x", "riskControls": "y"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1")
	w := httptest.NewRecorder()
	UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}

	// Submit to pending_approval
	req2 := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1")
	w2 := httptest.NewRecorder()
	SubmitChangeRecord(logger, mem, false).ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w2.Code, w2.Body.String())
	}

	// Try to edit — should get 409
	b2 := crBody(t, map[string]string{"safetyClass": "ClassC"})
	req3 := withURLParam(httptest.NewRequest(http.MethodPost, "/", b2), "artifactId", "art-1")
	w3 := httptest.NewRecorder()
	UpsertChangeRecord(logger, mem, false).ServeHTTP(w3, req3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestGetChangeRecord_NotFound(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	GetChangeRecord(logger, mem).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetChangeRecord_Found(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Create
	b := crBody(t, map[string]string{"safetyClass": "ClassA", "impactSummary": "minor", "riskControls": "none"})
	createReq := withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1")
	UpsertChangeRecord(logger, mem, false).ServeHTTP(httptest.NewRecorder(), createReq)

	// GET
	getReq := withURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	GetChangeRecord(logger, mem).ServeHTTP(w, getReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.SafetyClass != "ClassA" {
		t.Errorf("safetyClass = %q", resp.SafetyClass)
	}
}

func TestSubmitChangeRecord_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Create draft with required fields
	b := crBody(t, map[string]string{"safetyClass": "ClassB", "impactSummary": "summary", "riskControls": "controls"})
	UpsertChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1"),
	)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	SubmitChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "pending_approval" {
		t.Errorf("status = %q, want pending_approval", resp.Status)
	}
}

func TestSubmitChangeRecord_MissingFields(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Create draft with empty impactSummary/riskControls
	b := crBody(t, map[string]string{"safetyClass": "ClassB"})
	UpsertChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1"),
	)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	SubmitChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApproveChangeRecord_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)
	createAndSubmit(t, mem, logger)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	ApproveChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "approved" {
		t.Errorf("status = %q, want approved", resp.Status)
	}
	if resp.ApprovedAt == nil {
		t.Error("approvedAt should be set")
	}
}

func TestApproveChangeRecord_WrongState(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)

	// Still in draft — approve should fail
	b := crBody(t, map[string]string{"safetyClass": "ClassB", "impactSummary": "x", "riskControls": "y"})
	UpsertChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1"),
	)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1")
	w := httptest.NewRecorder()
	ApproveChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRejectChangeRecord_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)
	createAndSubmit(t, mem, logger)

	body := crBody(t, map[string]string{"reason": "insufficient risk analysis"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", body), "artifactId", "art-1")
	w := httptest.NewRecorder()
	RejectChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "rejected" {
		t.Errorf("status = %q, want rejected", resp.Status)
	}
	if resp.RejectedReason != "insufficient risk analysis" {
		t.Errorf("rejectedReason = %q", resp.RejectedReason)
	}
}

func TestRejectChangeRecord_MissingReason(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)
	createAndSubmit(t, mem, logger)

	body := crBody(t, map[string]string{"reason": ""})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", body), "artifactId", "art-1")
	w := httptest.NewRecorder()
	RejectChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRejectThenReopenChangeRecord(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifact(t, mem)
	createAndSubmit(t, mem, logger)

	// Reject
	rejectBody := crBody(t, map[string]string{"reason": "needs more info"})
	RejectChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", rejectBody), "artifactId", "art-1"),
	)

	// Edit (reopen to draft)
	editBody := crBody(t, map[string]string{"safetyClass": "ClassB", "impactSummary": "updated", "riskControls": "updated controls"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", editBody), "artifactId", "art-1")
	w := httptest.NewRecorder()
	UpsertChangeRecord(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reopen: %d %s", w.Code, w.Body.String())
	}
	var resp ChangeRecordResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Status != "draft" {
		t.Errorf("status = %q, want draft after reopen", resp.Status)
	}
}

// createAndSubmit is a test helper that creates a draft and submits it.
func createAndSubmit(t *testing.T, mem *memory.Store, logger *log.Logger) {
	t.Helper()
	b := crBody(t, map[string]string{"safetyClass": "ClassB", "impactSummary": "summary", "riskControls": "controls"})
	UpsertChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", b), "artifactId", "art-1"),
	)
	SubmitChangeRecord(logger, mem, false).ServeHTTP(
		httptest.NewRecorder(),
		withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-1"),
	)
}
