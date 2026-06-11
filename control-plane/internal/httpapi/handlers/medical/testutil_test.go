//go:build medical

// Package medical_test contains handler-level compliance tests for the IoMT
// medical deployment profile (G1–G5). Run with: go test -tags medical ./...
package medical_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func silentLogger() *log.Logger { return log.New(&bytes.Buffer{}, "", 0) }

// withURLParam injects chi route parameters into a request context.
func withURLParam(req *http.Request, key, val string) *http.Request {
	routeCtx, _ := req.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if routeCtx == nil {
		routeCtx = chi.NewRouteContext()
	}
	routeCtx.URLParams.Add(key, val)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	return req.WithContext(ctx)
}

// seedClassBArtifact creates a ClassB artifact in the store and returns its ID.
func seedClassBArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	a := store.Artifact{
		ArtifactID:  "art-classb-" + t.Name(),
		Name:        "firmware-b",
		Version:     "1.0.0",
		Type:        "firmware",
		SafetyClass: "ClassB",
	}
	if err := mem.CreateArtifact(a); err != nil {
		t.Fatalf("seed ClassB artifact: %v", err)
	}
	return a
}

// seedClassCArtifact creates a ClassC artifact.
func seedClassCArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	a := store.Artifact{
		ArtifactID:  "art-classc-" + t.Name(),
		Name:        "firmware-c",
		Version:     "1.0.0",
		Type:        "firmware",
		SafetyClass: "ClassC",
	}
	if err := mem.CreateArtifact(a); err != nil {
		t.Fatalf("seed ClassC artifact: %v", err)
	}
	return a
}

// seedClassAArtifact creates a ClassA artifact (no approval gate).
func seedClassAArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	a := store.Artifact{
		ArtifactID:  "art-classa-" + t.Name(),
		Name:        "config-a",
		Version:     "1.0.0",
		Type:        "config_bundle",
		SafetyClass: "ClassA",
	}
	if err := mem.CreateArtifact(a); err != nil {
		t.Fatalf("seed ClassA artifact: %v", err)
	}
	return a
}

// seedApprovedArtifact creates a ClassB artifact with an approved change record,
// completed vuln scan, and SBOM + VEX object keys — satisfying all QMS prerequisites.
func seedApprovedArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	a := store.Artifact{
		ArtifactID:    "art-full-" + t.Name(),
		Name:          "firmware-full",
		Version:       "2.0.0",
		Type:          "firmware",
		SafetyClass:   "ClassB",
		SBOMObjectKey: "sboms/art-full.cdx.json",
		VexObjectKey:  "sboms/art-full.vex.json",
	}
	if err := mem.CreateArtifact(a); err != nil {
		t.Fatalf("seed full artifact: %v", err)
	}

	rec, err := mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:      "cr-full-" + t.Name(),
		ArtifactID:    a.ArtifactID,
		SafetyClass:   "ClassB",
		ImpactSummary: "firmware security patch",
		RiskControls:  "QA review, unit tests",
		Status:        "approved",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed change record: %v", err)
	}
	_ = rec

	scan := store.ArtifactVulnerabilityScan{
		ScanID:     "scan-full-" + t.Name(),
		ArtifactID: a.ArtifactID,
		ScanStatus: "completed",
		ScannedAt:  time.Now().UTC(),
	}
	if err := mem.CreateArtifactVulnScan(scan); err != nil {
		t.Fatalf("seed vuln scan: %v", err)
	}

	return a
}

// ── fake object store ─────────────────────────────────────────────────────────

type fakeObjStore struct {
	presignURL string
	data       map[string][]byte
}

func newFakeObjStore(presignURL string) *fakeObjStore {
	return &fakeObjStore{presignURL: presignURL, data: make(map[string][]byte)}
}

func (f *fakeObjStore) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return f.presignURL, nil
}

func (f *fakeObjStore) PutObject(_ context.Context, _, key string, body io.Reader, _ int64, _ string) (int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	f.data[key] = data
	return int64(len(data)), nil
}

func (f *fakeObjStore) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	data := f.data[key]
	if data == nil {
		data = []byte(`{}`)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
