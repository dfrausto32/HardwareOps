package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/globalplane"
)

// ── Fake stores ───────────────────────────────────────────────────────────────

type fakeDeviceStore struct {
	devices []globalplane.CachedDevice
}

func (f *fakeDeviceStore) ListDeviceCache(_ globalplane.DeviceCacheFilter) ([]globalplane.CachedDevice, error) {
	return f.devices, nil
}

type fakeArtifactStore struct {
	artifacts []globalplane.CachedArtifact
}

func (f *fakeArtifactStore) ListArtifactCache(_ globalplane.ArtifactCacheFilter) ([]globalplane.CachedArtifact, error) {
	return f.artifacts, nil
}

type fakeHealthStore struct {
	snaps  []globalplane.HealthSnapshot
	planes []globalplane.RegionalPlane
}

func (f *fakeHealthStore) ListHealthCache() ([]globalplane.HealthSnapshot, error) {
	return f.snaps, nil
}
func (f *fakeHealthStore) ListRegionalPlanes() ([]globalplane.RegionalPlane, error) {
	return f.planes, nil
}

// ── ListDevices ───────────────────────────────────────────────────────────────

func TestListDevices_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	ListDevices(&fakeDeviceStore{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []globalplane.CachedDevice
	_ = json.NewDecoder(w.Body).Decode(&out)
	if out == nil {
		t.Error("expected empty array, not null")
	}
}

func TestListDevices_WithData(t *testing.T) {
	now := time.Now().UTC()
	st := &fakeDeviceStore{
		devices: []globalplane.CachedDevice{
			{CacheID: "c1", PlaneID: "p1", DeviceID: "d1", Status: "active", LastSeen: &now},
			{CacheID: "c2", PlaneID: "p1", DeviceID: "d2", Status: "offline"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	ListDevices(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []globalplane.CachedDevice
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(out))
	}
}

func TestListDevices_FilterByStatus(t *testing.T) {
	// The handler passes the filter through to the store; the store applies it.
	// Here we verify the handler encodes whatever the store returns.
	active := globalplane.CachedDevice{DeviceID: "d1", Status: "active"}
	st := &fakeDeviceStore{devices: []globalplane.CachedDevice{active}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices?status=active", nil)
	w := httptest.NewRecorder()
	ListDevices(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []globalplane.CachedDevice
	_ = json.NewDecoder(w.Body).Decode(&out)
	if len(out) != 1 || out[0].DeviceID != "d1" {
		t.Errorf("unexpected result: %+v", out)
	}
}

// ── ListArtifacts ─────────────────────────────────────────────────────────────

func TestListArtifacts_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts", nil)
	w := httptest.NewRecorder()
	ListArtifacts(&fakeArtifactStore{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []globalplane.CachedArtifact
	_ = json.NewDecoder(w.Body).Decode(&out)
	if out == nil {
		t.Error("expected empty array, not null")
	}
}

func TestListArtifacts_WithData(t *testing.T) {
	st := &fakeArtifactStore{
		artifacts: []globalplane.CachedArtifact{
			{ArtifactID: "a1", Name: "myapp", Version: "1.0.0", Status: "active"},
			{ArtifactID: "a2", Name: "myapp", Version: "2.0.0", Status: "active"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts", nil)
	w := httptest.NewRecorder()
	ListArtifacts(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []globalplane.CachedArtifact
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(out))
	}
}

// ── HealthSummary ─────────────────────────────────────────────────────────────

func TestHealthSummary_Empty(t *testing.T) {
	st := &fakeHealthStore{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/summary", nil)
	w := httptest.NewRecorder()
	HealthSummary(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHealthSummary_Aggregates(t *testing.T) {
	now := time.Now().UTC()
	st := &fakeHealthStore{
		planes: []globalplane.RegionalPlane{
			{PlaneID: "p1", Name: "us-east"},
			{PlaneID: "p2", Name: "eu-west"},
		},
		snaps: []globalplane.HealthSnapshot{
			{PlaneID: "p1", TotalDevices: 10, ActiveDevices: 8, StaleDevices: 2, LastDeviceSeen: &now},
			{PlaneID: "p2", TotalDevices: 5, ActiveDevices: 4, OfflineDevices: 1, LastDeviceSeen: &now},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/summary", nil)
	w := httptest.NewRecorder()
	HealthSummary(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalDevices  int `json:"totalDevices"`
		ActiveDevices int `json:"activeDevices"`
		Planes        []struct {
			PlaneID   string `json:"planeId"`
			PlaneName string `json:"planeName"`
		} `json:"planes"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalDevices != 15 {
		t.Errorf("expected totalDevices=15, got %d", resp.TotalDevices)
	}
	if resp.ActiveDevices != 12 {
		t.Errorf("expected activeDevices=12, got %d", resp.ActiveDevices)
	}
	if len(resp.Planes) != 2 {
		t.Errorf("expected 2 planes in response, got %d", len(resp.Planes))
	}
}

func TestHealthSummary_FilterByPlaneID(t *testing.T) {
	now := time.Now().UTC()
	st := &fakeHealthStore{
		planes: []globalplane.RegionalPlane{
			{PlaneID: "p1", Name: "us-east"},
		},
		snaps: []globalplane.HealthSnapshot{
			{PlaneID: "p1", TotalDevices: 7, ActiveDevices: 5, LastDeviceSeen: &now},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/summary?planeId=p1", nil)
	w := httptest.NewRecorder()
	HealthSummary(st).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		TotalDevices int `json:"totalDevices"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.TotalDevices != 7 {
		t.Errorf("expected totalDevices=7 for single plane, got %d", resp.TotalDevices)
	}
}
