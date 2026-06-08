package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMedicalOnly_StandardProfile_Returns404(t *testing.T) {
	mw := MedicalOnly("standard")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/status", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for standard profile, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected error field in 404 response")
	}
}

func TestMedicalOnly_MedicalProfile_PassesThrough(t *testing.T) {
	mw := MedicalOnly("medical")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/status", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for medical profile, got %d", w.Code)
	}
}

func TestMedicalOnly_EmptyProfile_Returns404(t *testing.T) {
	mw := MedicalOnly("")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/status", nil)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for empty profile, got %d", w.Code)
	}
}

func TestMedicalStatus_MedicalProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/status", nil)
	w := httptest.NewRecorder()
	MedicalStatus("medical").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body MedicalProfileStatus
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Profile != "medical" {
		t.Errorf("expected profile=medical, got %q", body.Profile)
	}
	if !body.Medical {
		t.Error("expected medical=true")
	}
}

func TestMedicalStatus_StandardProfile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical/status", nil)
	w := httptest.NewRecorder()
	MedicalStatus("standard").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body MedicalProfileStatus
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Profile != "standard" {
		t.Errorf("expected profile=standard, got %q", body.Profile)
	}
	if body.Medical {
		t.Error("expected medical=false for standard profile")
	}
}
