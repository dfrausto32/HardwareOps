package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

const testVexPresignExpires = 15 * time.Minute

// vexTestObjStore satisfies vexObjectStore for tests.
type vexTestObjStore struct{ url string }

func (o *vexTestObjStore) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return o.url, nil
}

func makeArtifactForVex(t *testing.T, mem *memory.Store, vexKey string) {
	t.Helper()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID:    "art-vex-1",
		Name:          "myapp",
		Version:       "2.0.0",
		Type:          "app_bundle",
		Status:        "published",
		SBOMObjectKey: "sboms/art-vex-1.cdx.json",
		VexObjectKey:  vexKey,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
}

func TestPostVexPresign_ArtifactNotFound(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "nonexistent")
	w := httptest.NewRecorder()

	PostVexPresign(logger, mem, &vexTestObjStore{url: ""}, "test-bucket", testVexPresignExpires, false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPostVexPresign_NoVexYet(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "") // VexObjectKey empty

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-vex-1")
	w := httptest.NewRecorder()

	PostVexPresign(logger, mem, &vexTestObjStore{url: "https://example.com/vex.json"}, "test-bucket", testVexPresignExpires, false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (VEX not yet generated), got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostVexPresign_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "sboms/art-vex-1.vex.json")

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", nil), "artifactId", "art-vex-1")
	w := httptest.NewRecorder()

	PostVexPresign(logger, mem, &vexTestObjStore{url: "https://minio.example.com/presigned"}, "test-bucket", testVexPresignExpires, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["url"] != "https://minio.example.com/presigned" {
		t.Errorf("url = %q, want presigned URL", resp["url"])
	}
}

func TestUpsertVexAssertion_HappyPath(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "")

	body, _ := json.Marshal(VexAssertionRequest{
		CVEID:         "CVE-2024-12345",
		ComponentName: "openssl",
		Assertion:     "not_affected",
		Justification: "not reachable from user input",
	})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "artifactId", "art-vex-1")
	w := httptest.NewRecorder()

	UpsertVexAssertion(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp VexAssertionResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.CVEID != "CVE-2024-12345" {
		t.Errorf("cveId = %q", resp.CVEID)
	}
	if resp.Assertion != "not_affected" {
		t.Errorf("assertion = %q", resp.Assertion)
	}
	if resp.AssertionID == "" {
		t.Error("assertionId must be non-empty")
	}
}

func TestUpsertVexAssertion_Idempotent(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "")

	upsert := func(assertion string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(VexAssertionRequest{
			CVEID:     "CVE-2024-99",
			Assertion: assertion,
		})
		req := withURLParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "artifactId", "art-vex-1")
		w := httptest.NewRecorder()
		UpsertVexAssertion(logger, mem, false).ServeHTTP(w, req)
		return w
	}

	w1 := upsert("under_investigation")
	if w1.Code != http.StatusOK {
		t.Fatalf("first upsert: %d %s", w1.Code, w1.Body.String())
	}
	// Second call overrides assertion value.
	w2 := upsert("fixed")
	if w2.Code != http.StatusOK {
		t.Fatalf("second upsert: %d %s", w2.Code, w2.Body.String())
	}
	var resp VexAssertionResponse
	_ = json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Assertion != "fixed" {
		t.Errorf("assertion after override = %q, want fixed", resp.Assertion)
	}
}

func TestUpsertVexAssertion_InvalidAssertion(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "")

	body, _ := json.Marshal(VexAssertionRequest{CVEID: "CVE-2024-1", Assertion: "bogus"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "artifactId", "art-vex-1")
	w := httptest.NewRecorder()

	UpsertVexAssertion(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpsertVexAssertion_MissingCVEID(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)
	makeArtifactForVex(t, mem, "")

	body, _ := json.Marshal(VexAssertionRequest{Assertion: "fixed"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "artifactId", "art-vex-1")
	w := httptest.NewRecorder()

	UpsertVexAssertion(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpsertVexAssertion_ArtifactNotFound(t *testing.T) {
	mem := memory.New()
	logger := log.New(&bytes.Buffer{}, "", 0)

	body, _ := json.Marshal(VexAssertionRequest{CVEID: "CVE-2024-1", Assertion: "fixed"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)), "artifactId", "nope")
	w := httptest.NewRecorder()

	UpsertVexAssertion(logger, mem, false).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
