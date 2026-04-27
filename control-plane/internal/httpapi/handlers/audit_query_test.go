package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestListAuditEvents(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:    "e1",
		OccurredAt: time.Now().UTC().Add(-1 * time.Minute),
		Action:     "device.checkin",
		Status:     "success",
	})
	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:    "e2",
		OccurredAt: time.Now().UTC(),
		Action:     "artifact.upload",
		Status:     "success",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=10", nil)
	w := httptest.NewRecorder()

	ListAuditEvents(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp AuditListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].EventID != "e2" {
		t.Fatalf("expected newest event first")
	}
}

func TestExportAuditCSV(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	_ = mem.CreateAuditEvent(store.AuditEvent{
		EventID:    "e1",
		OccurredAt: time.Now().UTC(),
		Action:     "device.enroll",
		Status:     "success",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit.csv", nil)
	w := httptest.NewRecorder()

	ExportAuditCSV(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("invalid csv: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + row")
	}
	if records[1][0] != "e1" {
		t.Fatalf("expected event id in csv")
	}
}

func TestAuditRetention(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/retention", nil)
	w := httptest.NewRecorder()
	GetAuditRetention(logger, mem).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp AuditRetentionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Days != 90 {
		t.Fatalf("expected default 90 days")
	}

	body := []byte(`{"days":30}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/audit/retention", bytes.NewReader(body))
	w = httptest.NewRecorder()
	SetAuditRetention(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Days != 30 {
		t.Fatalf("expected updated days=30")
	}
}
