//go:build medical

package medical_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/parcel/control-plane/internal/httpapi/handlers"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// IEC 62304 §5.7 — change records must carry a valid safety class.
func TestIEC62304_UpsertChangeRecord_InvalidSafetyClassRejected(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})

	body, _ := json.Marshal(handlers.ChangeRecordRequest{
		SafetyClass:   "ClassX", // not a valid class
		ImpactSummary: "desc",
		RiskControls:  "controls",
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.UpsertChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid safety class, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "safetyClass") {
		t.Fatalf("expected error to mention safetyClass, got: %s", w.Body.String())
	}
}

// IEC 62304 §5.7 — all three valid class values must be accepted.
func TestIEC62304_UpsertChangeRecord_ValidSafetyClasses(t *testing.T) {
	for _, cls := range []string{"ClassA", "ClassB", "ClassC"} {
		t.Run(cls, func(t *testing.T) {
			mem := memory.New()
			_ = mem.CreateArtifact(store.Artifact{
				ArtifactID: "art-" + cls, Name: "fw", Version: "1.0", Type: "firmware",
			})
			body, _ := json.Marshal(handlers.ChangeRecordRequest{
				SafetyClass:   cls,
				ImpactSummary: "desc",
				RiskControls:  "controls",
			})
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req = withURLParam(req, "artifactId", "art-"+cls)
			w := httptest.NewRecorder()
			handlers.UpsertChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)
			if w.Code != http.StatusCreated && w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200/201, got %d: %s", cls, w.Code, w.Body.String())
			}
		})
	}
}

// IEC 62304 §5.7 — submit requires both ImpactSummary AND RiskControls.
func TestIEC62304_SubmitChangeRecord_RequiresBothMandatoryFields(t *testing.T) {
	cases := []struct {
		name          string
		impactSummary string
		riskControls  string
	}{
		{"missing_impact_summary", "", "controls"},
		{"missing_risk_controls", "desc", ""},
		{"missing_both", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := memory.New()
			_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})
			_, _ = mem.CreateChangeRecord(store.ChangeRecord{
				RecordID:      "cr1",
				ArtifactID:    "art1",
				SafetyClass:   "ClassB",
				ImpactSummary: tc.impactSummary,
				RiskControls:  tc.riskControls,
				Status:        "draft",
			})

			req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
			req = withURLParam(req, "artifactId", "art1")
			w := httptest.NewRecorder()
			handlers.SubmitChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s: expected 422, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		})
	}
}

// IEC 62304 §5.7 — state transition: draft → pending_approval.
func TestIEC62304_SubmitChangeRecord_TransitionsToPendingApproval(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:      "cr1",
		ArtifactID:    "art1",
		SafetyClass:   "ClassB",
		ImpactSummary: "security patch",
		RiskControls:  "QA review",
		Status:        "draft",
	})

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.SubmitChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp handlers.ChangeRecordResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "pending_approval" {
		t.Fatalf("expected status=pending_approval, got %q", resp.Status)
	}
}

// IEC 62304 §5.7 — submit must be rejected when record is already pending_approval.
func TestIEC62304_SubmitChangeRecord_CannotResubmitPendingApproval(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:      "cr1",
		ArtifactID:    "art1",
		SafetyClass:   "ClassB",
		ImpactSummary: "desc",
		RiskControls:  "controls",
		Status:        "pending_approval",
	})

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.SubmitChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for already-submitted record, got %d", w.Code)
	}
}

// IEC 62304 §5.7 — approve: must be in pending_approval state.
func TestIEC62304_ApproveChangeRecord_RequiresPendingApprovalState(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:    "cr1",
		ArtifactID:  "art1",
		SafetyClass: "ClassB",
		Status:      "draft",
	})

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.ApproveChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for draft-state approve, got %d", w.Code)
	}
}

// IEC 62304 §5.7 — reject requires a non-empty reason.
func TestIEC62304_RejectChangeRecord_RequiresReason(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:      "cr1",
		ArtifactID:    "art1",
		SafetyClass:   "ClassB",
		ImpactSummary: "desc",
		RiskControls:  "controls",
		Status:        "pending_approval",
	})

	body := []byte(`{"reason":""}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.RejectChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty reject reason, got %d: %s", w.Code, w.Body.String())
	}
}

// IEC 62304 §5.7 — full happy path: create → submit → approve with audit trail.
func TestIEC62304_FullApprovalWorkflow_ProducesAuditTrail(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art1", Name: "fw", Version: "1.0", Type: "firmware"})

	// Create
	body, _ := json.Marshal(handlers.ChangeRecordRequest{
		SafetyClass:   "ClassB",
		ImpactSummary: "security patch for CVE-2025-1234",
		RiskControls:  "QA review, regression tests, clinical validation",
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withURLParam(req, "artifactId", "art1")
	w := httptest.NewRecorder()
	handlers.UpsertChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Submit
	req = httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art1")
	w = httptest.NewRecorder()
	handlers.SubmitChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Approve
	req = httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art1")
	w = httptest.NewRecorder()
	handlers.ApproveChangeRecord(silentLogger(), mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp handlers.ChangeRecordResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "approved" {
		t.Fatalf("expected approved, got %q", resp.Status)
	}

	// Audit trail must contain all three transition events.
	events, err := mem.ListAuditEvents(store.AuditEventFilter{TargetType: "change_record", Limit: 50})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	actions := make(map[string]bool)
	for _, ev := range events {
		actions[ev.Action] = true
	}
	for _, expected := range []string{"change_record.created", "change_record.submitted", "change_record.approved"} {
		if !actions[expected] {
			t.Errorf("missing audit action %q; found: %v", expected, actions)
		}
	}
}
