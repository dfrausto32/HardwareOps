package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// fakeQMSObjStore satisfies qmsObjectStore for tests.
type fakeQMSObjStore struct {
	getContent map[string][]byte // key → content for GetObject
	uploaded   map[string][]byte // captures PutObject calls
	presignURL string
}

func newFakeQMSObjStore(presignURL string) *fakeQMSObjStore {
	return &fakeQMSObjStore{
		getContent: map[string][]byte{},
		uploaded:   map[string][]byte{},
		presignURL: presignURL,
	}
}

func (f *fakeQMSObjStore) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	data, ok := f.getContent[key]
	if !ok {
		data = []byte(`{"fake":"content","key":"` + key + `"}`)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeQMSObjStore) PutObject(_ context.Context, _, key string, body io.Reader, _ int64, _ string) (int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	f.uploaded[key] = data
	return int64(len(data)), nil
}

func (f *fakeQMSObjStore) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return f.presignURL, nil
}

// seedFullArtifact creates an artifact with all QMS prerequisites met.
func seedFullArtifact(t *testing.T, mem *memory.Store) store.Artifact {
	t.Helper()
	art := store.Artifact{
		ArtifactID:    "art-qms-1",
		Name:          "firmware-v2",
		Version:       "2.0.0",
		Type:          "firmware",
		Status:        "published",
		SafetyClass:   "ClassB",
		SBOMObjectKey: "sboms/art-qms-1.cdx.json",
		VexObjectKey:  "sboms/art-qms-1.vex.json",
	}
	if err := mem.CreateArtifact(art); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	// Approved change record.
	now := time.Now().UTC()
	_, err := mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:         "cr-1",
		ArtifactID:       "art-qms-1",
		SafetyClass:      "ClassB",
		ImpactSummary:    "firmware update",
		RiskControls:     "validated in staging",
		Status:           "approved",
		CreatedByUserID:  "user-1",
		ApprovedByUserID: "admin-1",
		ApprovedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create change record: %v", err)
	}

	// Completed vuln scan.
	if err := mem.CreateArtifactVulnScan(store.ArtifactVulnerabilityScan{
		ScanID:             "scan-1",
		ArtifactID:         "art-qms-1",
		ScannerType:        "trivy",
		ScannerVersion:     "0.50.0",
		ScanStatus:         "completed",
		FindingsJSON:       []byte(`[{"id":"CVE-2024-1","severity":"low","package":"openssl","version":"3.0"}]`),
		SeverityCountsJSON: []byte(`{"critical":0,"high":0,"medium":0,"low":1,"unknown":0}`),
		ScannedAt:          now,
	}); err != nil {
		t.Fatalf("create vuln scan: %v", err)
	}

	return art
}

func TestPostQMSPackage_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedFullArtifact(t, mem)

	objStore := newFakeQMSObjStore("https://minio.example.com/qms.zip?sig=abc")

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-qms-1")
	w := httptest.NewRecorder()

	PostQMSPackage(logger, mem, objStore, "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["url"] != "https://minio.example.com/qms.zip?sig=abc" {
		t.Errorf("url = %q", resp["url"])
	}
	if !strings.HasPrefix(resp["packageKey"], "qms/art-qms-1/") {
		t.Errorf("packageKey = %q, want qms/art-qms-1/ prefix", resp["packageKey"])
	}
}

func TestPostQMSPackage_ZipContents(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedFullArtifact(t, mem)

	objStore := newFakeQMSObjStore("https://minio.example.com/qms.zip")

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-qms-1")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, objStore, "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Find the uploaded ZIP.
	var zipBytes []byte
	for _, data := range objStore.uploaded {
		zipBytes = data
		break
	}
	if len(zipBytes) == 0 {
		t.Fatal("no ZIP was uploaded")
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	want := map[string]bool{
		"manifest.json":                  false,
		"sbom.cdx.json":                  false,
		"sbom.vex.json":                  false,
		"attestations.json":              false,
		"change-record.json":             false,
		"change-record-audit-trail.json": false,
		"vuln-scan-summary.json":         false,
		"deployment-audit-trail.json":    false,
	}
	for _, f := range zr.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		} else {
			t.Errorf("unexpected file in ZIP: %s", f.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing file in ZIP: %s", name)
		}
	}
}

func TestPostQMSPackage_ManifestFields(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedFullArtifact(t, mem)

	objStore := newFakeQMSObjStore("https://minio.example.com/qms.zip")
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-qms-1")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, objStore, "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}

	var zipBytes []byte
	for _, d := range objStore.uploaded {
		zipBytes = d
	}
	zr, _ := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, _ := f.Open()
		var m qmsManifest
		_ = json.NewDecoder(rc).Decode(&m)
		rc.Close()
		if m.ArtifactID != "art-qms-1" {
			t.Errorf("manifest.artifactId = %q", m.ArtifactID)
		}
		if m.SafetyClass != "ClassB" {
			t.Errorf("manifest.safetyClass = %q", m.SafetyClass)
		}
		if m.GeneratedAt == "" {
			t.Error("manifest.generatedAt must be set")
		}
	}
}

func TestPostQMSPackage_VulnScanSummaryHighCritOnly(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	art := seedFullArtifact(t, mem)

	// Replace vuln scan with critical+high findings mixed with low.
	_ = art
	if err := mem.CreateArtifactVulnScan(store.ArtifactVulnerabilityScan{
		ScanID:      "scan-2",
		ArtifactID:  "art-qms-1",
		ScannerType: "grype",
		ScanStatus:  "completed",
		FindingsJSON: []byte(`[
			{"id":"CVE-2024-1","severity":"critical","package":"libssl","version":"1.1.1"},
			{"id":"CVE-2024-2","severity":"high","package":"curl","version":"7.80"},
			{"id":"CVE-2024-3","severity":"low","package":"zlib","version":"1.2.11"}
		]`),
		SeverityCountsJSON: []byte(`{"critical":1,"high":1,"medium":0,"low":1,"unknown":0}`),
		ScannedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create scan: %v", err)
	}

	objStore := newFakeQMSObjStore("https://minio.example.com/qms.zip")
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-qms-1")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, objStore, "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}

	var zipBytes []byte
	for _, d := range objStore.uploaded {
		zipBytes = d
	}
	zr, _ := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	for _, f := range zr.File {
		if f.Name != "vuln-scan-summary.json" {
			continue
		}
		rc, _ := f.Open()
		var summary vulnScanSummary
		_ = json.NewDecoder(rc).Decode(&summary)
		rc.Close()
		if len(summary.CriticalHigh) != 2 {
			t.Errorf("criticalHighFindings = %d, want 2 (critical+high only)", len(summary.CriticalHigh))
		}
		for _, f := range summary.CriticalHigh {
			sev := strings.ToLower(f.Severity)
			if sev != "critical" && sev != "high" {
				t.Errorf("low-severity finding leaked into criticalHigh: %s", f.Severity)
			}
		}
	}
}

func TestPostQMSPackage_MissingSBOM(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	// Artifact missing SBOMObjectKey and VexObjectKey.
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: "art-no-sbom",
		Name:       "app",
		Version:    "1.0",
		Type:       "app_bundle",
		Status:     "published",
	}); err != nil {
		t.Fatal(err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-no-sbom")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore(""), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "prerequisites not met" {
		t.Errorf("error = %q", resp["error"])
	}
	prereqs, ok := resp["prerequisites"].(map[string]any)
	if !ok {
		t.Fatal("prerequisites field missing or wrong type")
	}
	sbom, _ := prereqs["sbom"].(map[string]any)
	if sbom["ready"] == true {
		t.Error("sbom should not be ready")
	}
}

func TestPostQMSPackage_PrerequisitesAllFields(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: "art-prereqs",
		Name:       "app",
		Version:    "1.0",
		Type:       "app_bundle",
		Status:     "published",
		// Intentionally missing SBOM, VEX keys.
	}); err != nil {
		t.Fatal(err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-prereqs")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore(""), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	prereqs, _ := resp["prerequisites"].(map[string]any)
	for _, key := range []string{"sbom", "vex", "changeRecord", "vulnScan"} {
		if _, ok := prereqs[key]; !ok {
			t.Errorf("prerequisites.%s field missing", key)
		}
	}
}

func TestPostQMSPackage_PendingChangeRecordBlocked(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	// Artifact with SBOM+VEX but change record still pending.
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID:    "art-pending-cr",
		Name:          "app",
		Version:       "1.0",
		Type:          "app_bundle",
		Status:        "published",
		SBOMObjectKey: "sboms/art-pending-cr.cdx.json",
		VexObjectKey:  "sboms/art-pending-cr.vex.json",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := mem.CreateChangeRecord(store.ChangeRecord{
		RecordID: "cr-pending", ArtifactID: "art-pending-cr",
		SafetyClass: "ClassB", ImpactSummary: "x", RiskControls: "y",
		Status: "pending_approval", CreatedByUserID: "u1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mem.CreateArtifactVulnScan(store.ArtifactVulnerabilityScan{
		ScanID: "s1", ArtifactID: "art-pending-cr",
		ScannerType: "trivy", ScanStatus: "completed", ScannedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-pending-cr")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore(""), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	prereqs, _ := resp["prerequisites"].(map[string]any)
	cr, _ := prereqs["changeRecord"].(map[string]any)
	if cr["ready"] == true {
		t.Error("changeRecord should not be ready when pending_approval")
	}
}

func TestPostQMSPackage_AuditedOnSuccess(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	seedFullArtifact(t, mem)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-qms-1")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore("https://example.com/pkg.zip"), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}

	events, _ := mem.ListAuditEvents(store.AuditEventFilter{Action: "qms_package.generated"})
	if len(events) == 0 {
		t.Error("expected qms_package.generated audit event")
	}
	if events[0].Status != "success" {
		t.Errorf("audit status = %q, want success", events[0].Status)
	}
}

func TestPostQMSPackage_AuditedOnPrerequisiteFailure(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: "art-fail", Name: "app", Version: "1.0",
		Type: "app_bundle", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-fail")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore(""), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}

	events, _ := mem.ListAuditEvents(store.AuditEventFilter{Action: "qms_package.generated"})
	if len(events) == 0 {
		t.Error("expected audit event even on failure")
	}
	if events[0].Status != "error" {
		t.Errorf("audit status = %q, want error", events[0].Status)
	}
}

func TestPostQMSPackage_ArtifactNotFound(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "nonexistent")
	w := httptest.NewRecorder()
	PostQMSPackage(logger, mem, newFakeQMSObjStore(""), "test-bucket", 15*time.Minute, false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCheckQMSPrerequisites_AllReady(t *testing.T) {
	now := time.Now().UTC()
	art := store.Artifact{SBOMObjectKey: "sboms/x.cdx.json", VexObjectKey: "sboms/x.vex.json"}
	cr := store.ChangeRecord{Status: "approved", ApprovedAt: now}
	scan := store.ArtifactVulnerabilityScan{ScanStatus: "completed"}

	p := checkQMSPrerequisites(art, cr, true, scan, true)
	if !p.allReady() {
		data, _ := json.Marshal(p)
		t.Errorf("expected all ready, got: %s", data)
	}
}

func TestCheckQMSPrerequisites_Partial(t *testing.T) {
	art := store.Artifact{SBOMObjectKey: "sboms/x.cdx.json"} // VEX missing
	cr := store.ChangeRecord{Status: "draft"}
	scan := store.ArtifactVulnerabilityScan{ScanStatus: "running"}

	p := checkQMSPrerequisites(art, cr, true, scan, true)
	if p.SBOM.Ready != true {
		t.Error("SBOM should be ready")
	}
	if p.VEX.Ready != false {
		t.Error("VEX should not be ready")
	}
	if p.ChangeRecord.Ready != false {
		t.Error("ChangeRecord should not be ready (draft)")
	}
	if p.VulnScan.Ready != false {
		t.Error("VulnScan should not be ready (running)")
	}
	if p.allReady() {
		t.Error("allReady() should be false")
	}
}
