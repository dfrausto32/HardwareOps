package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestPostApplyResult(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"success","appliedVersion":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ApplyResultResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Status != "success" {
		t.Fatalf("unexpected status")
	}
}

func TestPostApplyResult_InvalidStatus(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"nope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPostApplyResult_InvalidPreApplyStatus(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	id := uuid.NewString()
	body := []byte(`{"status":"success","appliedVersion":"1.0.0","preApplyStatus":"nope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+id+"/apply-result", bytes.NewReader(body))
	req = withURLParam(req, "deviceId", id)
	req, id = attachMTLSDevice(t, mem, req, id)
	w := httptest.NewRecorder()

	PostApplyResult(logger, mem, nil, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
