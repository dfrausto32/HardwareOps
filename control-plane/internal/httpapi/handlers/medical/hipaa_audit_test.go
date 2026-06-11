//go:build medical

package medical_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/httpapi/handlers"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// TestHIPAA_ExportDefaultsToPhiTouchedOnly verifies the default HIPAA export
// returns only events where phi_touched=true, scoping the report to PHI access.
func TestHIPAA_ExportDefaultsToPhiTouchedOnly(t *testing.T) {
	mem := memory.New()

	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:          "phi-1",
		OccurredAt:       time.Now().UTC(),
		Action:           "device.checkin",
		Status:           "success",
		PhiTouched:       true,
		MinimumNecessary: "device_telemetry",
	})
	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:    "nonphi-1",
		OccurredAt: time.Now().UTC(),
		Action:     "artifact.upload",
		Status:     "success",
		PhiTouched: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/audit/hipaa-export", http.NoBody)
	w := httptest.NewRecorder()
	handlers.HIPAAExportAudit(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp handlers.HIPAAExportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 PHI event, got %d", len(resp.Items))
	}
	if !resp.Items[0].PhiTouched {
		t.Fatal("returned event must have phiTouched=true")
	}
}

// TestHIPAA_ExportScopeAllIncludesNonPhi verifies ?scope=all bypasses the
// phi_touched filter so auditors can see the full audit trail.
func TestHIPAA_ExportScopeAllIncludesNonPhi(t *testing.T) {
	mem := memory.New()

	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID: "phi-1", OccurredAt: time.Now().UTC(), Action: "device.checkin",
		Status: "success", PhiTouched: true,
	})
	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID: "nonphi-1", OccurredAt: time.Now().UTC(), Action: "artifact.upload",
		Status: "success", PhiTouched: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/audit/hipaa-export?scope=all", http.NoBody)
	w := httptest.NewRecorder()
	handlers.HIPAAExportAudit(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp handlers.HIPAAExportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) < 2 {
		t.Fatalf("scope=all should include non-PHI events; got %d items", len(resp.Items))
	}
}

// TestHIPAA_RequiredFieldsPresent verifies every event in the HIPAA export
// carries the HIPAA Security Rule required fields (45 CFR §164.312(b)).
func TestHIPAA_RequiredFieldsPresent(t *testing.T) {
	mem := memory.New()

	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:          "phi-2",
		OccurredAt:       time.Now().UTC(),
		Action:           "device.apply_result",
		Status:           "success",
		ActorID:          "user-123",
		ActorEmail:       "clinician@example.com",
		PhiTouched:       true,
		MinimumNecessary: "device_apply_logs",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/audit/hipaa-export", http.NoBody)
	w := httptest.NewRecorder()
	handlers.HIPAAExportAudit(silentLogger(), mem, false).ServeHTTP(w, req)

	var resp handlers.HIPAAExportResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) == 0 {
		t.Fatal("expected at least one item")
	}
	ev := resp.Items[0]
	if ev.DateTime.IsZero() {
		t.Error("dateTime field must be set (45 CFR §164.312(b))")
	}
	if ev.ActionType == "" {
		t.Error("actionType field must be set")
	}
	if ev.Status == "" {
		t.Error("status field must be set")
	}
	if ev.DataAccessed == "" {
		t.Error("dataAccessed (minimumNecessary) must be set for PHI events")
	}
}

// TestHIPAA_RetentionFloorEnforced verifies that HIPAA's 6-year retention
// requirement (45 CFR §164.530(j)) cannot be circumvented in medical mode.
func TestHIPAA_RetentionFloorEnforced(t *testing.T) {
	mem := memory.New()

	body := []byte(`{"days":30}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/audit/retention", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handlers.SetAuditRetention(silentLogger(), mem, false, store.HIPAAMinRetentionDays).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("retention below HIPAA floor should return 422, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHIPAA_RetentionFloorExactBoundary verifies retention exactly at 2190 days
// (6 years) is accepted — the minimum permissible value.
func TestHIPAA_RetentionFloorExactBoundary(t *testing.T) {
	mem := memory.New()

	body, _ := json.Marshal(map[string]int{"days": store.HIPAAMinRetentionDays})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/audit/retention", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handlers.SetAuditRetention(silentLogger(), mem, false, store.HIPAAMinRetentionDays).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("retention=2190 days should be accepted, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHIPAA_RetentionAboveFloorAccepted verifies retention above the floor
// (e.g. 3000 days / ~8 years) is also accepted.
func TestHIPAA_RetentionAboveFloorAccepted(t *testing.T) {
	mem := memory.New()

	body, _ := json.Marshal(map[string]int{"days": 3000})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/audit/retention", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handlers.SetAuditRetention(silentLogger(), mem, false, store.HIPAAMinRetentionDays).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("retention=3000 days should be accepted, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHIPAA_ExportEmitsAuditEvent verifies that the HIPAA export itself is
// audited — viewing PHI access logs is a PHI-touching operation.
func TestHIPAA_ExportEmitsAuditEvent(t *testing.T) {
	mem := memory.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/audit/hipaa-export", http.NoBody)
	w := httptest.NewRecorder()
	handlers.HIPAAExportAudit(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "audit.hipaa_export", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("HIPAA export must self-audit with action audit.hipaa_export")
	}
}
