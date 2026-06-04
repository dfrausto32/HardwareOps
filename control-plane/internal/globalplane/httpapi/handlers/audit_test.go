package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

// fakeAuditQueryStore satisfies the auditQueryStore interface.
type fakeAuditQueryStore struct {
	events []store.AuditEvent
}

func (f *fakeAuditQueryStore) CreateAuditEvent(ev store.AuditEvent) error {
	f.events = append(f.events, ev)
	return nil
}
func (f *fakeAuditQueryStore) ListAuditEvents(_ store.AuditEventFilter) ([]store.AuditEvent, error) {
	return f.events, nil
}
func (f *fakeAuditQueryStore) DeleteAuditEventsBefore(_ time.Time) (int, error) { return 0, nil }

func TestListGlobalAuditEvents_Empty(t *testing.T) {
	st := &fakeAuditQueryStore{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()

	ListGlobalAuditEvents(silentLogger(), st, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Items == nil {
		t.Error("expected items array (even if empty), got nil")
	}
}

func TestListGlobalAuditEvents_WithEvents(t *testing.T) {
	st := &fakeAuditQueryStore{
		events: []store.AuditEvent{
			{EventID: "e1", Action: "auth.login", Status: "success", OccurredAt: time.Now().UTC()},
			{EventID: "e2", Action: "plane.register", Status: "success", OccurredAt: time.Now().UTC()},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()

	ListGlobalAuditEvents(silentLogger(), st, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct {
			EventID string `json:"eventId"`
			Action  string `json:"action"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestExportGlobalAuditCSV(t *testing.T) {
	st := &fakeAuditQueryStore{
		events: []store.AuditEvent{
			{EventID: "e1", Action: "auth.login", Status: "success", OccurredAt: time.Now().UTC()},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/export", nil)
	w := httptest.NewRecorder()

	ExportGlobalAuditCSV(silentLogger(), st, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected text/csv, got %s", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty CSV body")
	}
}

func TestListGlobalAuditEvents_InvalidTimeParam(t *testing.T) {
	st := &fakeAuditQueryStore{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?since=not-a-time", nil)
	w := httptest.NewRecorder()

	ListGlobalAuditEvents(silentLogger(), st, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid time, got %d", w.Code)
	}
}
