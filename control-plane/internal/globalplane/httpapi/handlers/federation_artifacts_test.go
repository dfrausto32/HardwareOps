package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// fakeFederationStore satisfies federationStore for read-path tests.
type fakeFederationStore struct {
	artifacts   map[string]globalplane.GlobalArtifact
	replStatus  map[string][]globalplane.ArtifactReplicationStatus
	planes      []globalplane.RegionalPlane
}

func newFakeFederationStore() *fakeFederationStore {
	return &fakeFederationStore{
		artifacts:  make(map[string]globalplane.GlobalArtifact),
		replStatus: make(map[string][]globalplane.ArtifactReplicationStatus),
	}
}

func (f *fakeFederationStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	return f.planes, nil
}
func (f *fakeFederationStore) CreateGlobalArtifact(a globalplane.GlobalArtifact) error {
	f.artifacts[a.ArtifactID] = a
	return nil
}
func (f *fakeFederationStore) GetGlobalArtifact(id string) (globalplane.GlobalArtifact, bool, error) {
	a, ok := f.artifacts[id]
	return a, ok, nil
}
func (f *fakeFederationStore) ListGlobalArtifacts(_ string) ([]globalplane.GlobalArtifact, error) {
	out := make([]globalplane.GlobalArtifact, 0, len(f.artifacts))
	for _, a := range f.artifacts {
		out = append(out, a)
	}
	return out, nil
}
func (f *fakeFederationStore) CreateReplicationStatusRows(artifactID string, planeIDs []string) error {
	now := time.Now().UTC()
	for _, pid := range planeIDs {
		f.replStatus[artifactID] = append(f.replStatus[artifactID], globalplane.ArtifactReplicationStatus{
			ArtifactID: artifactID,
			PlaneID:    pid,
			BlobStatus: "pending",
			CreatedAt:  now,
		})
	}
	return nil
}
func (f *fakeFederationStore) UpdateReplicationStatusMetadataPush(artifactID, planeID string, pushedAt *time.Time, pushErr string) error {
	return nil
}
func (f *fakeFederationStore) ListReplicationStatus(artifactID string) ([]globalplane.ArtifactReplicationStatus, error) {
	rows := f.replStatus[artifactID]
	if rows == nil {
		rows = []globalplane.ArtifactReplicationStatus{}
	}
	return rows, nil
}

// ── ListFederatedArtifacts ────────────────────────────────────────────────────

func TestListFederatedArtifacts_Empty(t *testing.T) {
	st := newFakeFederationStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts", nil)
	w := httptest.NewRecorder()
	ListFederatedArtifacts(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []any
	_ = json.NewDecoder(w.Body).Decode(&out)
	if out == nil {
		t.Error("expected empty array, not null")
	}
}

func TestListFederatedArtifacts_WithReplicationSummary(t *testing.T) {
	st := newFakeFederationStore()
	now := time.Now().UTC()
	_ = st.CreateGlobalArtifact(globalplane.GlobalArtifact{
		ArtifactID: "art-1", Name: "myapp", Version: "1.0.0",
		Status: "active", CreatedAt: now,
	})
	st.replStatus["art-1"] = []globalplane.ArtifactReplicationStatus{
		{ArtifactID: "art-1", PlaneID: "p1", BlobStatus: "confirmed"},
		{ArtifactID: "art-1", PlaneID: "p2", BlobStatus: "pending"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts", nil)
	w := httptest.NewRecorder()
	ListFederatedArtifacts(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []struct {
		ArtifactID       string `json:"ArtifactID"`
		ConfirmedRegions int    `json:"confirmedRegions"`
		TotalRegions     int    `json:"totalRegions"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(resp))
	}
	if resp[0].ConfirmedRegions != 1 {
		t.Errorf("expected confirmedRegions=1, got %d", resp[0].ConfirmedRegions)
	}
	if resp[0].TotalRegions != 2 {
		t.Errorf("expected totalRegions=2, got %d", resp[0].TotalRegions)
	}
}

// ── GetFederatedArtifact ──────────────────────────────────────────────────────

func TestGetFederatedArtifact_NotFound(t *testing.T) {
	st := newFakeFederationStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/nonexistent", nil)
	req = withURLParam(req, "artifactId", "nonexistent")
	w := httptest.NewRecorder()
	GetFederatedArtifact(st).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetFederatedArtifact_Found(t *testing.T) {
	st := newFakeFederationStore()
	now := time.Now().UTC()
	_ = st.CreateGlobalArtifact(globalplane.GlobalArtifact{
		ArtifactID: "art-1", Name: "myapp", Version: "2.0.0",
		Status: "active", CreatedAt: now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/art-1", nil)
	req = withURLParam(req, "artifactId", "art-1")
	w := httptest.NewRecorder()
	GetFederatedArtifact(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out globalplane.GlobalArtifact
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ArtifactID != "art-1" {
		t.Errorf("expected ArtifactID=art-1, got %q", out.ArtifactID)
	}
}

// ── GetReplicationStatus ──────────────────────────────────────────────────────

func TestGetReplicationStatus_NoRows(t *testing.T) {
	st := newFakeFederationStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/art-x/replication-status", nil)
	req = withURLParam(req, "artifactId", "art-x")
	w := httptest.NewRecorder()
	GetReplicationStatus(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		TotalRegions int `json:"totalRegions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.TotalRegions != 0 {
		t.Errorf("expected totalRegions=0, got %d", resp.TotalRegions)
	}
}

func TestGetReplicationStatus_WithRows(t *testing.T) {
	st := newFakeFederationStore()
	st.replStatus["art-2"] = []globalplane.ArtifactReplicationStatus{
		{ArtifactID: "art-2", PlaneID: "p1", BlobStatus: "confirmed"},
		{ArtifactID: "art-2", PlaneID: "p2", BlobStatus: "confirmed"},
		{ArtifactID: "art-2", PlaneID: "p3", BlobStatus: "pending"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/art-2/replication-status", nil)
	req = withURLParam(req, "artifactId", "art-2")
	w := httptest.NewRecorder()
	GetReplicationStatus(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		ConfirmedRegions int `json:"confirmedRegions"`
		TotalRegions     int `json:"totalRegions"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ConfirmedRegions != 2 {
		t.Errorf("expected confirmedRegions=2, got %d", resp.ConfirmedRegions)
	}
	if resp.TotalRegions != 3 {
		t.Errorf("expected totalRegions=3, got %d", resp.TotalRegions)
	}
}

// ── PresignFederatedArtifact ──────────────────────────────────────────────────

func TestPresignFederatedArtifact_NoObjectStore(t *testing.T) {
	st := newFakeFederationStore()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/a/presign", nil)
	req = withURLParam(req, "artifactId", "a")
	w := httptest.NewRecorder()
	PresignFederatedArtifact(st, nil, "bucket", time.Hour).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no object store, got %d", w.Code)
	}
}

func TestPresignFederatedArtifact_ArtifactNotFound(t *testing.T) {
	st := newFakeFederationStore()
	objStore := &stubObjectStore{presignURL: "https://example.com/presigned"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/artifacts/missing/presign", nil)
	req = withURLParam(req, "artifactId", "missing")
	w := httptest.NewRecorder()
	PresignFederatedArtifact(st, objStore, "bucket", time.Hour).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing artifact, got %d", w.Code)
	}
}

// stubObjectStore for presign tests.
type stubObjectStore struct {
	presignURL string
	putErr     error
}

func (s *stubObjectStore) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return s.presignURL, nil
}
func (s *stubObjectStore) PutObject(_ context.Context, _, _ string, _ io.Reader, _ int64, _ string) (int64, error) {
	if s.putErr != nil {
		return 0, s.putErr
	}
	return 0, nil
}
func (s *stubObjectStore) EnsureBucket(_ context.Context, _ string) error { return nil }
