//go:build medical

package medical_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/httpapi/handlers"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

const presignExpires = 15 * time.Minute

// TestQMS_AllPrerequisitesMissingReturns422WithChecklist verifies that the
// structured prerequisite checklist is returned when no prerequisites are met.
func TestQMS_AllPrerequisitesMissingReturns422WithChecklist(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{
		ArtifactID: "art-bare",
		Name:       "fw",
		Version:    "1.0",
		Type:       "firmware",
		SafetyClass: "ClassB",
		// No SBOMObjectKey, VexObjectKey; no change record; no vuln scan
	})

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art-bare")
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error         string                    `json:"error"`
		Prerequisites handlers.QMSPrerequisites `json:"prerequisites"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error == "" {
		t.Fatal("expected non-empty error message")
	}
	if body.Prerequisites.SBOM.Ready {
		t.Error("SBOM should not be ready")
	}
	if body.Prerequisites.VEX.Ready {
		t.Error("VEX should not be ready")
	}
	if body.Prerequisites.ChangeRecord.Ready {
		t.Error("ChangeRecord should not be ready")
	}
	if body.Prerequisites.VulnScan.Ready {
		t.Error("VulnScan should not be ready")
	}
}

// TestQMS_SBOMPrerequisiteMissing verifies the SBOM-specific prerequisite
// reason string is present when only the SBOM key is absent.
func TestQMS_SBOMPrerequisiteMissing(t *testing.T) {
	mem := memory.New()
	a := store.Artifact{
		ArtifactID:   "art-nosbom",
		Name:         "fw",
		Version:      "1.0",
		Type:         "firmware",
		SafetyClass:  "ClassB",
		VexObjectKey: "sboms/art.vex.json",
		// No SBOMObjectKey
	}
	_ = mem.CreateArtifact(a)
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:    "cr-nosbom",
		ArtifactID:  a.ArtifactID,
		SafetyClass: "ClassB",
		Status:      "approved",
	})
	_ = mem.CreateArtifactVulnScan(store.ArtifactVulnerabilityScan{
		ScanID:     "scan-nosbom",
		ArtifactID: a.ArtifactID,
		ScanStatus: "completed",
		ScannedAt:  time.Now().UTC(),
	})

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", a.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Prerequisites handlers.QMSPrerequisites `json:"prerequisites"`
	}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body.Prerequisites.SBOM.Ready {
		t.Error("SBOM should not be ready")
	}
	if body.Prerequisites.SBOM.Reason == "" {
		t.Error("SBOM prerequisite should include a reason string")
	}
}

// TestQMS_ChangeRecordNotApprovedBlocked verifies a non-approved change record
// is an explicit prerequisite failure with the current status in the reason.
func TestQMS_ChangeRecordNotApprovedBlocked(t *testing.T) {
	mem := memory.New()
	a := store.Artifact{
		ArtifactID:    "art-crpending",
		Name:          "fw",
		Version:       "1.0",
		Type:          "firmware",
		SafetyClass:   "ClassB",
		SBOMObjectKey: "sboms/art.cdx.json",
		VexObjectKey:  "sboms/art.vex.json",
	}
	_ = mem.CreateArtifact(a)
	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:    "cr-pending",
		ArtifactID:  a.ArtifactID,
		SafetyClass: "ClassB",
		Status:      "pending_approval", // not approved
	})
	_ = mem.CreateArtifactVulnScan(store.ArtifactVulnerabilityScan{
		ScanID:     "scan-crpending",
		ArtifactID: a.ArtifactID,
		ScanStatus: "completed",
		ScannedAt:  time.Now().UTC(),
	})

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", a.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Prerequisites handlers.QMSPrerequisites `json:"prerequisites"`
	}
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body.Prerequisites.ChangeRecord.Ready {
		t.Error("ChangeRecord should not be ready for pending_approval status")
	}
	if body.Prerequisites.ChangeRecord.Reason == "" {
		t.Error("ChangeRecord prerequisite should include the current status in the reason")
	}
}

// TestQMS_FullPrerequisitesReturn200WithPresignedURL is the happy-path test:
// all 4 prerequisites satisfied → 200 with presigned download URL.
func TestQMS_FullPrerequisitesReturn200WithPresignedURL(t *testing.T) {
	mem := memory.New()
	artifact := seedApprovedArtifact(t, mem)

	expectedURL := "https://storage.example.com/qms/package.zip?sig=xyz"
	objStore := newFakeObjStore(expectedURL)

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", artifact.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["url"] != expectedURL {
		t.Fatalf("expected url=%q, got %q", expectedURL, resp["url"])
	}
	if resp["packageKey"] == "" {
		t.Fatal("packageKey must be returned in response")
	}
}

// TestQMS_ZipContainsExactly8Files verifies the QMS evidence bundle always
// contains the mandated 8-file structure defined by the roadmap spec.
func TestQMS_ZipContainsExactly8Files(t *testing.T) {
	mem := memory.New()
	artifact := seedApprovedArtifact(t, mem)

	objStore := newFakeObjStore("https://example.com/pkg")

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", artifact.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Find the uploaded zip in the fake object store.
	var zipData []byte
	for _, data := range objStore.data {
		if len(data) > 0 {
			zipData = data
			break
		}
	}
	if len(zipData) == 0 {
		t.Fatal("no zip data found in fake object store")
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	expectedFiles := map[string]bool{
		"manifest.json":                   false,
		"sbom.cdx.json":                   false,
		"sbom.vex.json":                   false,
		"attestations.json":               false,
		"change-record.json":              false,
		"change-record-audit-trail.json":  false,
		"vuln-scan-summary.json":          false,
		"deployment-audit-trail.json":     false,
	}
	for _, f := range zr.File {
		expectedFiles[f.Name] = true
	}
	for name, found := range expectedFiles {
		if !found {
			t.Errorf("QMS zip missing required file: %s", name)
		}
	}
	if len(zr.File) != 8 {
		t.Errorf("expected exactly 8 files, got %d", len(zr.File))
	}
}

// TestQMS_ManifestContainsSafetyClass verifies the manifest.json inside the
// ZIP carries the artifact's safety class for regulatory review.
func TestQMS_ManifestContainsSafetyClass(t *testing.T) {
	mem := memory.New()
	artifact := seedApprovedArtifact(t, mem)

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", artifact.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var zipData []byte
	for _, data := range objStore.data {
		if len(data) > 0 {
			zipData = data
			break
		}
	}

	zr, _ := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open manifest: %v", err)
		}
		var manifest map[string]any
		_ = json.NewDecoder(rc).Decode(&manifest)
		rc.Close()

		if manifest["safetyClass"] != "ClassB" {
			t.Fatalf("manifest safetyClass should be ClassB, got %v", manifest["safetyClass"])
		}
		if manifest["generatedAt"] == "" || manifest["generatedAt"] == nil {
			t.Error("manifest must include generatedAt timestamp")
		}
		return
	}
	t.Fatal("manifest.json not found in zip")
}

// TestQMS_AuditEventFiredOnSuccess verifies the qms_package.generated event
// is written with status "success" after a successful package generation.
func TestQMS_AuditEventFiredOnSuccess(t *testing.T) {
	mem := memory.New()
	artifact := seedApprovedArtifact(t, mem)

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", artifact.ArtifactID)
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{
		Action: "qms_package.generated",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected qms_package.generated audit event on success")
	}
	if events[0].Status == "error" {
		t.Fatal("audit event on success should not have error status")
	}
}

// TestQMS_AuditEventFiredOnFailure verifies the qms_package.generated event
// is still written (with error status) when prerequisites fail — creating a
// traceable record of the failed attempt for regulatory review.
func TestQMS_AuditEventFiredOnFailure(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{
		ArtifactID:  "art-fail",
		Name:        "fw",
		Version:     "1.0",
		Type:        "firmware",
		SafetyClass: "ClassC",
	})

	objStore := newFakeObjStore("https://example.com/pkg")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art-fail")
	w := httptest.NewRecorder()
	handlers.PostQMSPackage(silentLogger(), mem, objStore, "bucket", presignExpires, false).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{
		Action: "qms_package.generated",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected qms_package.generated audit event even on prerequisite failure")
	}
	if events[0].Status != "error" {
		t.Fatalf("audit event on failure should have status=error, got %q", events[0].Status)
	}
}
