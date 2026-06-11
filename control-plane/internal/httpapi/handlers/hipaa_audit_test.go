package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// seedAuditEvents adds audit events with and without phi_touched to the store.
func seedAuditEvents(t *testing.T, mem *memory.Store) {
	t.Helper()
	events := []store.AuditEvent{
		{
			OccurredAt: time.Now().UTC().Add(-1 * time.Hour),
			ActorID:    "user-1",
			Action:     "device.apply_result",
			TargetType: "device",
			TargetID:   "dev-1",
			Status:     "success",
			PhiTouched: true,
			MinimumNecessary: "device_apply_logs",
		},
		{
			OccurredAt: time.Now().UTC().Add(-2 * time.Hour),
			ActorID:    "user-1",
			Action:     "device.identity_violation",
			TargetType: "device",
			TargetID:   "dev-2",
			Status:     "success",
			PhiTouched: true,
			MinimumNecessary: "device_telemetry",
		},
		{
			OccurredAt: time.Now().UTC().Add(-3 * time.Hour),
			ActorID:    "admin-1",
			Action:     "artifact.published",
			TargetType: "artifact",
			TargetID:   "art-1",
			Status:     "success",
			PhiTouched: false,
		},
	}
	for _, ev := range events {
		if err := mem.CreateAuditEvent(ev); err != nil {
			t.Fatalf("seed audit event: %v", err)
		}
	}
}

func TestHIPAAExportAudit_DefaultPhiOnly(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedAuditEvents(t, mem)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	HIPAAExportAudit(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp HIPAAExportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	// Should return only the 2 PHI-touching events.
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 PHI events, got %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if !item.PhiTouched {
			t.Errorf("event %s: phiTouched=false should not be in default export", item.ActionType)
		}
	}
}

func TestHIPAAExportAudit_ScopeAll(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedAuditEvents(t, mem)

	req := httptest.NewRequest(http.MethodGet, "/?scope=all", nil)
	w := httptest.NewRecorder()

	HIPAAExportAudit(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp HIPAAExportResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	// Should return all 3 seeded events + 1 self-audit event.
	if len(resp.Items) < 3 {
		t.Errorf("expected at least 3 events with scope=all, got %d", len(resp.Items))
	}
}

func TestHIPAAExportAudit_RequiredFields(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedAuditEvents(t, mem)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	HIPAAExportAudit(logger, mem, false).ServeHTTP(w, req)
	var resp HIPAAExportResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	for _, item := range resp.Items {
		if item.DateTime.IsZero() {
			t.Error("dateTime must be set")
		}
		if item.ActionType == "" {
			t.Error("actionType must be set")
		}
		if item.Status == "" {
			t.Error("status must be set")
		}
	}
}

func TestSetAuditRetention_HIPAAFloorEnforced(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	body, _ := json.Marshal(map[string]int{"days": 30})
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	// Medical mode: floor = 2190 days
	SetAuditRetention(logger, mem, false, store.HIPAAMinRetentionDays).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 (HIPAA floor violation), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetAuditRetention_HIPAAFloorAllows(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	// 2190 days exactly — should be allowed.
	body, _ := json.Marshal(map[string]int{"days": store.HIPAAMinRetentionDays})
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	SetAuditRetention(logger, mem, false, store.HIPAAMinRetentionDays).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetAuditRetention_NoFloorStandardDeployment(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	// Standard deployment: minRetentionDays=0, so 30 days is fine.
	body, _ := json.Marshal(map[string]int{"days": 30})
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	SetAuditRetention(logger, mem, false, 0).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWithPHI(t *testing.T) {
	base := store.AuditEvent{Action: "device.checkin", Status: "success"}
	ev := withPHI(base, "device_telemetry")
	if !ev.PhiTouched {
		t.Error("PhiTouched should be true")
	}
	if ev.MinimumNecessary != "device_telemetry" {
		t.Errorf("MinimumNecessary = %q, want device_telemetry", ev.MinimumNecessary)
	}
	// Original should be unchanged (value semantics).
	if base.PhiTouched {
		t.Error("base event should not be mutated")
	}
}
