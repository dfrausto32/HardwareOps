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

func TestListPlanes_Empty(t *testing.T) {
	st := newFakePlanesStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes", nil)
	w := httptest.NewRecorder()

	ListPlanes(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Should be an empty JSON array, not null.
	body := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(body, "[") {
		t.Fatalf("expected JSON array, got: %s", body)
	}
}

func TestListPlanes_RedactsToken(t *testing.T) {
	st := newFakePlanesStore()
	_ = st.CreateRegionalPlane(globalplane.RegionalPlane{
		PlaneID:        uuid.NewString(),
		Name:           "us-east-1",
		BaseURL:        "https://us-east.example.com",
		EncryptedToken: []byte("secret-should-not-appear"),
		Enabled:        true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes", nil)
	w := httptest.NewRecorder()
	ListPlanes(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-should-not-appear") {
		t.Fatal("response must not contain encrypted token bytes")
	}
}

func TestListPlanes_IncludesPolicySyncSummary(t *testing.T) {
	st := newFakePlanesStore()
	planeID := uuid.NewString()
	_ = st.CreateRegionalPlane(globalplane.RegionalPlane{PlaneID: planeID, Name: "us-west", Enabled: true})
	pushedAt := time.Now().Add(-5 * time.Minute)
	st.statuses = []globalplane.PolicySyncStatus{
		{GroupID: uuid.NewString(), PlaneID: planeID, PushedAt: &pushedAt, PushError: ""},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes", nil)
	w := httptest.NewRecorder()
	ListPlanes(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var planes []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &planes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(planes) != 1 {
		t.Fatalf("expected 1 plane, got %d", len(planes))
	}
	if planes[0]["lastPolicySyncAt"] == nil {
		t.Fatal("expected lastPolicySyncAt to be populated")
	}
}

func TestListPlanes_SurfacesPolicySyncError(t *testing.T) {
	st := newFakePlanesStore()
	planeID := uuid.NewString()
	_ = st.CreateRegionalPlane(globalplane.RegionalPlane{PlaneID: planeID, Name: "eu-central", Enabled: true})
	st.statuses = []globalplane.PolicySyncStatus{
		{GroupID: uuid.NewString(), PlaneID: planeID, PushError: "connection refused"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes", nil)
	w := httptest.NewRecorder()
	ListPlanes(st).ServeHTTP(w, req)

	var planes []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &planes)
	if len(planes) == 0 {
		t.Fatal("expected at least one plane")
	}
	if planes[0]["lastPolicySyncError"] != "connection refused" {
		t.Fatalf("expected lastPolicySyncError, got: %v", planes[0]["lastPolicySyncError"])
	}
}

func TestRegisterPlane_MissingRequiredFields(t *testing.T) {
	st := newFakePlanesStore()
	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{"baseUrl":"https://host","serviceToken":"tok"}`},
		{"missing baseUrl", `{"name":"p1","serviceToken":"tok"}`},
		{"missing serviceToken", `{"name":"p1","baseUrl":"https://host"}`},
		{"empty body", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/planes", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			RegisterPlane(st, testEncKey, noopSync{}).ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegisterPlane_Valid(t *testing.T) {
	st := newFakePlanesStore()
	body := `{"name":"us-east","baseUrl":"https://us-east.example.com","serviceToken":"mytoken","syncIntervalSeconds":30}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/planes", strings.NewReader(body))
	w := httptest.NewRecorder()

	RegisterPlane(st, testEncKey, noopSync{}).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["planeId"] == "" {
		t.Fatal("expected non-empty planeId in response")
	}
	// Verify the token is stored encrypted (not plaintext).
	planes, _ := st.ListRegionalPlanes()
	if len(planes) != 1 {
		t.Fatalf("expected 1 plane in store, got %d", len(planes))
	}
	if string(planes[0].EncryptedToken) == "mytoken" {
		t.Fatal("token must be stored encrypted, not as plaintext")
	}
}

func TestGetPlane_NotFound(t *testing.T) {
	st := newFakePlanesStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/"+uuid.NewString(), nil)
	req = withURLParam(req, "planeId", uuid.NewString())
	w := httptest.NewRecorder()

	GetPlane(st).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetPlane_Found_RedactsToken(t *testing.T) {
	st := newFakePlanesStore()
	planeID := uuid.NewString()
	_ = st.CreateRegionalPlane(globalplane.RegionalPlane{
		PlaneID:        planeID,
		Name:           "test-plane",
		BaseURL:        "https://test.example.com",
		EncryptedToken: []byte("very-secret"),
		Enabled:        true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/planes/"+planeID, nil)
	req = withURLParam(req, "planeId", planeID)
	w := httptest.NewRecorder()

	GetPlane(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "very-secret") {
		t.Fatal("encrypted token must be redacted from response")
	}
}

func TestDeletePlane_NotFound(t *testing.T) {
	st := newFakePlanesStore()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/planes/"+uuid.NewString(), nil)
	req = withURLParam(req, "planeId", uuid.NewString())
	w := httptest.NewRecorder()

	DeletePlane(st, noopSync{}).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeletePlane_Valid(t *testing.T) {
	st := newFakePlanesStore()
	planeID := uuid.NewString()
	_ = st.CreateRegionalPlane(globalplane.RegionalPlane{PlaneID: planeID, Name: "doomed"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/planes/"+planeID, nil)
	req = withURLParam(req, "planeId", planeID)
	w := httptest.NewRecorder()

	DeletePlane(st, noopSync{}).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if _, ok, _ := st.GetRegionalPlane(planeID); ok {
		t.Fatal("plane should have been deleted from store")
	}
}
