//go:build medical

package medical_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/httpapi/handlers"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// TestVex_AllFourAssertionStatesAccepted verifies the VEX standard allows
// exactly the four exploitability assertion states defined in CycloneDX.
func TestVex_AllFourAssertionStatesAccepted(t *testing.T) {
	for _, assertion := range []string{"affected", "not_affected", "under_investigation", "fixed"} {
		t.Run(assertion, func(t *testing.T) {
			mem := memory.New()
			_ = mem.CreateArtifact(store.Artifact{
				ArtifactID: "art-vex", Name: "fw", Version: "1.0", Type: "firmware",
			})
			body, _ := json.Marshal(handlers.VexAssertionRequest{
				CVEID:         "CVE-2025-9999",
				ComponentName: "openssl",
				Assertion:     assertion,
				Justification: "manual review completed",
			})
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req = withURLParam(req, "artifactId", "art-vex")
			w := httptest.NewRecorder()
			handlers.UpsertVexAssertion(silentLogger(), mem, false).ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d: %s", assertion, w.Code, w.Body.String())
			}
			var resp handlers.VexAssertionResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Assertion != assertion {
				t.Fatalf("response assertion mismatch: want %q, got %q", assertion, resp.Assertion)
			}
		})
	}
}

// TestVex_InvalidAssertionRejected ensures the handler enforces the closed
// CycloneDX VEX vocabulary — unknown states must not be persisted.
func TestVex_InvalidAssertionRejected(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art-vex", Name: "fw", Version: "1.0", Type: "firmware"})

	for _, invalid := range []string{"", "unknown", "maybe", "n/a"} {
		t.Run(invalid, func(t *testing.T) {
			body, _ := json.Marshal(handlers.VexAssertionRequest{
				CVEID:     "CVE-2025-9999",
				Assertion: invalid,
			})
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req = withURLParam(req, "artifactId", "art-vex")
			w := httptest.NewRecorder()
			handlers.UpsertVexAssertion(silentLogger(), mem, false).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("invalid assertion %q: expected 400, got %d", invalid, w.Code)
			}
		})
	}
}

// TestVex_AssertionRequiresCVEID verifies the handler rejects assertions
// that omit the CVE identifier — a mandatory CycloneDX VEX field.
func TestVex_AssertionRequiresCVEID(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art-vex", Name: "fw", Version: "1.0", Type: "firmware"})

	body, _ := json.Marshal(handlers.VexAssertionRequest{
		CVEID:     "", // missing
		Assertion: "not_affected",
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withURLParam(req, "artifactId", "art-vex")
	w := httptest.NewRecorder()
	handlers.UpsertVexAssertion(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing cveId, got %d", w.Code)
	}
}

// TestVex_PresignBlockedWhenNoVex confirms the presign endpoint returns 404
// when the VEX document has not yet been generated for the artifact.
func TestVex_PresignBlockedWhenNoVex(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{
		ArtifactID:   "art-novex",
		Name:         "fw",
		Version:      "1.0",
		Type:         "firmware",
		VexObjectKey: "", // not yet generated
	})

	objStore := newFakeObjStore("https://example.com/presigned")
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art-novex")
	w := httptest.NewRecorder()
	handlers.PostVexPresign(silentLogger(), mem, objStore, "bucket", time.Minute, false).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when VEX not generated, got %d: %s", w.Code, w.Body.String())
	}
}

// TestVex_PresignSucceedsWhenVexExists confirms the presigned URL is returned
// once the VEX document has been uploaded to object storage.
func TestVex_PresignSucceedsWhenVexExists(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{
		ArtifactID:   "art-hasvex",
		Name:         "fw",
		Version:      "1.0",
		Type:         "firmware",
		VexObjectKey: "sboms/art-hasvex.vex.json",
	})

	expectedURL := "https://storage.example.com/presigned?token=abc"
	objStore := newFakeObjStore(expectedURL)
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "art-hasvex")
	w := httptest.NewRecorder()
	handlers.PostVexPresign(silentLogger(), mem, objStore, "bucket", time.Minute, false).ServeHTTP(w, req)

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
}

// TestVex_AssertionUpsertIsIdempotent verifies that asserting the same
// CVE/component pair twice updates rather than duplicates the record.
func TestVex_AssertionUpsertIsIdempotent(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art-idem", Name: "fw", Version: "1.0", Type: "firmware"})

	post := func(assertion string) {
		body, _ := json.Marshal(handlers.VexAssertionRequest{
			CVEID:         "CVE-2025-1111",
			ComponentName: "libssl",
			Assertion:     assertion,
		})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req = withURLParam(req, "artifactId", "art-idem")
		handlers.UpsertVexAssertion(silentLogger(), mem, false).ServeHTTP(httptest.NewRecorder(), req)
	}

	post("under_investigation")
	post("fixed") // override

	assertions, err := mem.ListVexAssertions("art-idem")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assertions) != 1 {
		t.Fatalf("expected 1 assertion after upsert, got %d", len(assertions))
	}
	if assertions[0].Assertion != "fixed" {
		t.Fatalf("expected updated assertion=fixed, got %q", assertions[0].Assertion)
	}
}

// TestVex_PresignArtifactNotFound returns 404 for unknown artifact IDs.
func TestVex_PresignArtifactNotFound(t *testing.T) {
	mem := memory.New()
	objStore := newFakeObjStore("https://example.com/presigned")

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req = withURLParam(req, "artifactId", "nonexistent")
	w := httptest.NewRecorder()
	handlers.PostVexPresign(silentLogger(), mem, objStore, "bucket", time.Minute, false).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown artifact, got %d", w.Code)
	}
}

// TestVex_AssertionAuditEventWritten verifies each VEX assertion upsert
// is recorded in the audit trail for traceability.
func TestVex_AssertionAuditEventWritten(t *testing.T) {
	mem := memory.New()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: "art-audit", Name: "fw", Version: "1.0", Type: "firmware"})

	body, _ := json.Marshal(handlers.VexAssertionRequest{
		CVEID:         "CVE-2025-5555",
		ComponentName: "libc",
		Assertion:     "not_affected",
		Justification: "component not present in affected code path",
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req = withURLParam(req, "artifactId", "art-audit")
	w := httptest.NewRecorder()
	handlers.UpsertVexAssertion(silentLogger(), mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "vex.assertion.upserted", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected vex.assertion.upserted audit event")
	}
	if !strings.Contains(string(events[0].AfterJSON), "CVE-2025-5555") {
		t.Fatalf("audit event AfterJSON missing CVE: %s", string(events[0].AfterJSON))
	}
}
