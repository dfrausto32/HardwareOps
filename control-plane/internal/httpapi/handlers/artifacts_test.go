package handlers

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/artifactingest"
	"github.com/parcel/control-plane/internal/artifacttrust"
	"github.com/parcel/control-plane/internal/metrics"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestCreateArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	body := []byte(`{"name":"agent","version":"1.0.0","objectKey":"artifacts/a.tar.gz","sha256":"abc","sizeBytes":10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCreateArtifact_RejectsUnsignedWhenSignaturePolicyEnforced(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	body := []byte(`{"name":"agent","version":"1.0.0","objectKey":"artifacts/a.tar.gz","sha256":"abc","sizeBytes":10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{
		Require:       true,
		EnforceIngest: true,
	}).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateArtifact_RejectsWrongSignatureKeyIDWhenPolicyPinned(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	body := []byte(`{"name":"agent","version":"1.0.0","objectKey":"artifacts/a.tar.gz","sha256":"abc","sizeBytes":10,"signature":"abc123","signatureKeyId":"sha256:wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{
		EnforceIngest: true,
		KeyID:         "sha256:expected",
	}).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestUploadArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "agent")
	_ = mw.WriteField("version", "1.0.0")
	fw, _ := mw.CreateFormFile("file", "agent-1.0.0.tar.gz")
	_, _ = fw.Write([]byte("dummy"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	UploadArtifact(logger, mem, obj, "artifacts", false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp UploadArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.ArtifactID == "" || resp.ObjectKey == "" || resp.SHA256 == "" {
		t.Fatalf("missing response fields")
	}
}

func TestPresignArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := uuid.NewString()
	_ = mem.CreateArtifact(store.Artifact{ArtifactID: artifactID, Name: "agent", Version: "1.0.0", ObjectKey: "artifacts/a.tar.gz", SHA256: "abc", SizeBytes: 10, CreatedAt: time.Now().UTC()})

	depsStore := newFakeObjectStore()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+artifactID+"/presign", nil)
	req = withURLParam(req, "artifactId", artifactID)
	w := httptest.NewRecorder()

	PresignArtifact(logger, mem, depsStore, "artifacts", time.Minute, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PresignResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.DownloadURL == "" {
		t.Fatalf("expected downloadUrl")
	}
}

func TestGetAssignedArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "agent",
		Version:    "1.0.0",
		ObjectKey:  "artifacts/a.tar.gz",
		SHA256:     "abc",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/artifacts/"+artifactID, nil)
	req = withURLParam(req, "artifactId", artifactID)
	req, deviceID := attachMTLSDevice(t, mem, req, "")
	if err := mem.UpsertDesiredStateDevice(store.DesiredStateDevice{
		DeviceID:       deviceID,
		ArtifactID:     artifactID,
		DesiredVersion: "1.0.0",
		Source:         "manual",
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired state device: %v", err)
	}
	w := httptest.NewRecorder()

	GetAssignedArtifact(logger, mem, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp ArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.ArtifactID != artifactID {
		t.Fatalf("expected artifact id %s, got %s", artifactID, resp.ArtifactID)
	}
}

func TestGetAssignedArtifact_FromGroupDesiredState(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "customer",
		Version:    "2.0.0",
		ObjectKey:  "artifacts/customer.tar.gz",
		SHA256:     "abc",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	groupID := uuid.NewString()
	if err := mem.UpsertGroup(store.Group{
		GroupID:      groupID,
		Name:         "all-devices",
		SelectorJSON: []byte(`{}`),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("put group: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/artifacts/"+artifactID+"/presign", bytes.NewReader([]byte("{}")))
	req = withURLParam(req, "artifactId", artifactID)
	req, deviceID := attachMTLSDevice(t, mem, req, "")
	if err := mem.UpsertDesiredStateGroup(store.DesiredStateGroup{
		GroupID:        groupID,
		ArtifactID:     artifactID,
		DesiredVersion: "2.0.0",
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert desired state group: %v", err)
	}

	objStore := newFakeObjectStore()
	w := httptest.NewRecorder()
	PresignAssignedArtifact(logger, mem, objStore, "artifacts", time.Minute, false, "", nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp PresignResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.DownloadURL == "" {
		t.Fatalf("expected presigned download url")
	}
	if deviceID == "" {
		t.Fatalf("expected device id")
	}
}

func TestGetAssignedArtifact_RejectsUnassignedArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "agent",
		Version:    "1.0.0",
		ObjectKey:  "artifacts/a.tar.gz",
		SHA256:     "abc",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/artifacts/"+artifactID, nil)
	req = withURLParam(req, "artifactId", artifactID)
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()

	GetAssignedArtifact(logger, mem, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPresignAndCompleteArtifactUpload(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	presignReqBody := []byte(`{"filename":"agent-1.0.0.tar.gz","contentType":"application/gzip","expiresSeconds":120}`)
	presignReq := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/presign-upload", bytes.NewReader(presignReqBody))
	presignW := httptest.NewRecorder()
	PresignArtifactUpload(logger, mem, obj, "artifacts", 5*time.Minute, false, nil).ServeHTTP(presignW, presignReq)
	if presignW.Code != http.StatusOK {
		t.Fatalf("presign upload expected 200, got %d body=%s", presignW.Code, presignW.Body.String())
	}
	var presignResp PresignUploadResponse
	if err := json.Unmarshal(presignW.Body.Bytes(), &presignResp); err != nil {
		t.Fatalf("presign upload response json: %v", err)
	}
	if presignResp.ArtifactID == "" || presignResp.ObjectKey == "" || presignResp.UploadURL == "" {
		t.Fatalf("missing presign upload fields")
	}

	artifactBytes := []byte("demo artifact body")
	obj.objects[presignResp.ObjectKey] = artifactBytes
	sum := sha256.Sum256(artifactBytes)
	shaHex := hex.EncodeToString(sum[:])

	completePayload := map[string]any{
		"artifactId": presignResp.ArtifactID,
		"name":       "agent",
		"version":    "1.0.0",
		"type":       "agent_bundle",
		"objectKey":  presignResp.ObjectKey,
		"sha256":     shaHex,
		"sizeBytes":  len(artifactBytes),
	}
	completeJSON, _ := json.Marshal(completePayload)
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/complete", bytes.NewReader(completeJSON))
	completeW := httptest.NewRecorder()
	CompleteArtifactUpload(logger, mem, obj, "artifacts", false, nil, ArtifactSignaturePolicy{}).ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusOK {
		t.Fatalf("complete upload expected 200, got %d body=%s", completeW.Code, completeW.Body.String())
	}
	var completeResp UploadArtifactResponse
	if err := json.Unmarshal(completeW.Body.Bytes(), &completeResp); err != nil {
		t.Fatalf("complete upload response json: %v", err)
	}
	if completeResp.ArtifactID != presignResp.ArtifactID {
		t.Fatalf("artifact id mismatch: %s vs %s", completeResp.ArtifactID, presignResp.ArtifactID)
	}
	if completeResp.SHA256 != shaHex {
		t.Fatalf("sha mismatch: %s vs %s", completeResp.SHA256, shaHex)
	}
}

func TestPullArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("pulled-artifact")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":      "customer-app",
		"version":   "0.1.0",
		"type":      "app_bundle",
		"sourceUrl": source.URL + "/artifact.tar.gz?token=secret",
		"sha256":    shaHex,
		"sizeBytes": len(payload),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	PullArtifact(logger, mem, obj, "artifacts", []string{"127.0.0.1", "localhost"}, 1024*1024, 10*time.Second, true, nil, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull artifact expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp UploadArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("pull artifact response json: %v", err)
	}
	if resp.ArtifactID == "" || resp.ObjectKey == "" {
		t.Fatalf("missing artifact response fields")
	}
	if _, ok := obj.objects[resp.ObjectKey]; !ok {
		t.Fatalf("expected object to be stored for key %s", resp.ObjectKey)
	}
}

func TestPullArtifactHostAllowlist(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("pulled-artifact")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":      "customer-app",
		"version":   "0.1.0",
		"type":      "app_bundle",
		"sourceUrl": source.URL + "/artifact.tar.gz",
		"sha256":    shaHex,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	// Deliberately disallow test server host.
	PullArtifact(logger, mem, obj, "artifacts", []string{"example.com"}, 1024*1024, 10*time.Second, true, nil, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("pull artifact expected 403 for disallowed host, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactSourceObject(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("pulled-artifact-source-object")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":    "source-object-app",
		"version": "0.1.0",
		"type":    "app_bundle",
		"source": map[string]any{
			"kind": "http",
			"uri":  source.URL + "/artifact.tar.gz",
		},
		"sha256": shaHex,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	PullArtifact(logger, mem, obj, "artifacts", []string{"127.0.0.1", "localhost"}, 1024*1024, 10*time.Second, true, nil, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull artifact source object expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactRejectsInsecureHTTPWhenDisabled(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("pulled-artifact-http-disabled")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":      "http-disabled-app",
		"version":   "0.1.0",
		"type":      "app_bundle",
		"sourceUrl": source.URL + "/artifact.tar.gz",
		"sha256":    shaHex,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	PullArtifact(logger, mem, obj, "artifacts", []string{"127.0.0.1", "localhost"}, 1024*1024, 10*time.Second, false, nil, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pull artifact expected 400 for insecure http, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactUnsupportedSourceKind(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":    "bad-source-kind",
		"version": "0.1.0",
		"source": map[string]any{
			"kind": "gcs",
			"uri":  "https://example.com/a.tar.gz",
		},
		"sha256": "abc123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	PullArtifact(logger, mem, obj, "artifacts", nil, 1024*1024, 10*time.Second, true, nil, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pull artifact unsupported source kind expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactArtifactoryCredentialRef(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("artifactory-pull-content")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-JFrog-Art-Api"); got != "jfrog-key-1" {
			http.Error(w, "missing jfrog key", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":    "artifactory-app",
		"version": "0.1.0",
		"source": map[string]any{
			"kind":          "artifactory",
			"uri":           source.URL + "/artifactory/generic-local/app.tar.gz",
			"credentialRef": "art-1",
		},
		"sha256": shaHex,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	resolver := artifactingest.NewStaticCredentialResolver(map[string]map[string]string{
		"art-1": {"artifactory_api_key": "jfrog-key-1"},
	})
	PullArtifact(logger, mem, obj, "artifacts", []string{"127.0.0.1", "localhost"}, 1024*1024, 10*time.Second, true, resolver, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull artifact artifactory expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactCredentialRef(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	payload := []byte("credential-ref-artifact")
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer source.Close()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":    "credential-ref-app",
		"version": "0.1.0",
		"source": map[string]any{
			"kind":          "http",
			"uri":           source.URL + "/artifact.tar.gz",
			"credentialRef": "repo-1",
		},
		"sha256": shaHex,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	resolver := artifactingest.NewStaticCredentialResolver(map[string]map[string]string{
		"repo-1": {"authorization": "Bearer secret-token"},
	})
	PullArtifact(logger, mem, obj, "artifacts", []string{"127.0.0.1", "localhost"}, 1024*1024, 10*time.Second, true, resolver, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull artifact credential ref expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPullArtifactCredentialRefNotFound(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	reqJSON, _ := json.Marshal(map[string]any{
		"name":    "credential-ref-missing",
		"version": "0.1.0",
		"source": map[string]any{
			"kind":          "http",
			"uri":           "https://example.com/artifact.tar.gz",
			"credentialRef": "missing",
		},
		"sha256": "abc123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/pull", bytes.NewReader(reqJSON))
	w := httptest.NewRecorder()

	resolver := artifactingest.NewStaticCredentialResolver(nil)
	PullArtifact(logger, mem, obj, "artifacts", nil, 1024*1024, 10*time.Second, true, resolver, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pull artifact missing credential ref expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestArtifactLifecycle_DeprecateRestoreDelete(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	artifactID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "agent",
		Version:    "1.0.0",
		Type:       "agent_bundle",
		ObjectKey:  "artifacts/a.tar.gz",
		SHA256:     "abc",
		SizeBytes:  10,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	deprecateReq := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+artifactID+"/deprecate", bytes.NewReader([]byte(`{"deleteAfterDays":7}`)))
	deprecateReq = withURLParam(deprecateReq, "artifactId", artifactID)
	deprecateW := httptest.NewRecorder()
	DeprecateArtifact(logger, mem, false, nil).ServeHTTP(deprecateW, deprecateReq)
	if deprecateW.Code != http.StatusOK {
		t.Fatalf("deprecate expected 200, got %d body=%s", deprecateW.Code, deprecateW.Body.String())
	}

	var depResp ArtifactResponse
	if err := json.Unmarshal(deprecateW.Body.Bytes(), &depResp); err != nil {
		t.Fatalf("deprecate response json: %v", err)
	}
	if depResp.Status != "deprecated" || depResp.DeleteAfter == nil {
		t.Fatalf("unexpected deprecate response: %+v", depResp)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+artifactID+"/restore", nil)
	restoreReq = withURLParam(restoreReq, "artifactId", artifactID)
	restoreW := httptest.NewRecorder()
	RestoreArtifact(logger, mem, false).ServeHTTP(restoreW, restoreReq)
	if restoreW.Code != http.StatusOK {
		t.Fatalf("restore expected 200, got %d body=%s", restoreW.Code, restoreW.Body.String())
	}

	var restoreResp ArtifactResponse
	if err := json.Unmarshal(restoreW.Body.Bytes(), &restoreResp); err != nil {
		t.Fatalf("restore response json: %v", err)
	}
	if restoreResp.Status != "active" {
		t.Fatalf("expected active status after restore, got %s", restoreResp.Status)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+artifactID, nil)
	deleteReq = withURLParam(deleteReq, "artifactId", artifactID)
	deleteW := httptest.NewRecorder()
	DeleteArtifact(logger, mem, newFakeObjectStore(), "artifacts", false, nil).ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusConflict {
		t.Fatalf("delete active expected 409, got %d", deleteW.Code)
	}

	deprecateReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+artifactID+"/deprecate", nil)
	deprecateReq2 = withURLParam(deprecateReq2, "artifactId", artifactID)
	deprecateW2 := httptest.NewRecorder()
	DeprecateArtifact(logger, mem, false, nil).ServeHTTP(deprecateW2, deprecateReq2)
	if deprecateW2.Code != http.StatusOK {
		t.Fatalf("deprecate expected 200, got %d", deprecateW2.Code)
	}

	deleteReq2 := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+artifactID, nil)
	deleteReq2 = withURLParam(deleteReq2, "artifactId", artifactID)
	deleteW2 := httptest.NewRecorder()
	DeleteArtifact(logger, mem, newFakeObjectStore(), "artifacts", false, nil).ServeHTTP(deleteW2, deleteReq2)
	if deleteW2.Code != http.StatusNoContent {
		t.Fatalf("delete deprecated expected 204, got %d body=%s", deleteW2.Code, deleteW2.Body.String())
	}
}

func TestArtifactLifecyclePolicy(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/lifecycle/policy", nil)
	getW := httptest.NewRecorder()
	GetArtifactLifecyclePolicy(logger, mem).ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get policy expected 200, got %d", getW.Code)
	}

	setReq := httptest.NewRequest(http.MethodPut, "/api/v1/artifacts/lifecycle/policy", bytes.NewReader([]byte(`{"deprecatedDeleteAfterDays":14}`)))
	setW := httptest.NewRecorder()
	SetArtifactLifecyclePolicy(logger, mem, false).ServeHTTP(setW, setReq)
	if setW.Code != http.StatusOK {
		t.Fatalf("set policy expected 200, got %d body=%s", setW.Code, setW.Body.String())
	}

	var resp ArtifactLifecyclePolicyResponse
	if err := json.Unmarshal(setW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("set policy response json: %v", err)
	}
	if resp.DeprecatedDeleteAfterDays != 14 {
		t.Fatalf("expected 14 days, got %d", resp.DeprecatedDeleteAfterDays)
	}
}

func TestUploadArtifact_VerifiesTrustedEd25519Signature(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()
	metricsCollector := metrics.New()

	keyID, privateKey := upsertTrustedEd25519Key(t, mem)
	if _, err := mem.SetArtifactTrustPolicy(store.ArtifactTrustPolicy{VerificationMode: artifacttrust.VerificationModeRequire}); err != nil {
		t.Fatalf("set trust policy: %v", err)
	}

	payload := []byte("signed artifact body")
	signature := signArtifactDigestBase64(privateKey, payload)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "agent")
	_ = mw.WriteField("version", "1.0.1")
	_ = mw.WriteField("signature", signature)
	_ = mw.WriteField("signatureType", artifacttrust.SignatureTypeEd25519)
	_ = mw.WriteField("signatureKeyId", keyID)
	fw, _ := mw.CreateFormFile("file", "agent-1.0.1.tar.gz")
	_, _ = fw.Write(payload)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	UploadArtifact(logger, mem, obj, "artifacts", false, metricsCollector, ArtifactSignaturePolicy{Store: mem}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp UploadArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	artifact, ok, err := mem.GetArtifact(resp.ArtifactID)
	if err != nil || !ok {
		t.Fatalf("artifact missing err=%v ok=%t", err, ok)
	}
	if artifact.VerificationStatus != artifacttrust.VerificationStatusVerified {
		t.Fatalf("expected verified status, got %s (%s)", artifact.VerificationStatus, artifact.VerificationError)
	}
	metricsBody := scrapeMetricsBody(t, metricsCollector)
	if !strings.Contains(metricsBody, `hwops_artifact_verification_total{operation="upload",signature_type="ed25519",status="verified"} 1`) {
		t.Fatalf("expected verification metric, got:\n%s", metricsBody)
	}
	if !strings.Contains(metricsBody, `hwops_artifact_verification_state_total{status="verified"} 1`) {
		t.Fatalf("expected verification state metric, got:\n%s", metricsBody)
	}
}

func TestUploadArtifact_RejectsUnsignedWhenStrictTrustPolicyEnabled(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := newFakeObjectStore()

	if _, err := mem.SetArtifactTrustPolicy(store.ArtifactTrustPolicy{VerificationMode: artifacttrust.VerificationModeRequire}); err != nil {
		t.Fatalf("set trust policy: %v", err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "agent")
	_ = mw.WriteField("version", "1.0.2")
	fw, _ := mw.CreateFormFile("file", "agent-1.0.2.tar.gz")
	_, _ = fw.Write([]byte("unsigned body"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	UploadArtifact(logger, mem, obj, "artifacts", false, nil, ArtifactSignaturePolicy{Store: mem}).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "artifact signature required but missing") {
		t.Fatalf("expected strict rejection, got %s", w.Body.String())
	}
}

func upsertTrustedEd25519Key(t *testing.T, mem *memory.Store) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkix, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
	keyID, err := artifacttrust.ComputeKeyIDFromPublicKeyPEM(publicKeyPEM)
	if err != nil {
		t.Fatalf("compute key id: %v", err)
	}
	if _, err := mem.UpsertTrustedSigningKey(store.TrustedSigningKey{
		KeyID:        keyID,
		DisplayName:  "test-ed25519",
		Algorithm:    artifacttrust.SignatureTypeEd25519,
		PublicKeyPEM: publicKeyPEM,
		State:        artifacttrust.KeyStateActive,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert trusted key: %v", err)
	}
	return keyID, priv
}

func signArtifactDigestBase64(privateKey ed25519.PrivateKey, body []byte) string {
	sum := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, sum[:]))
}

func scrapeMetricsBody(t *testing.T, metricsCollector *metrics.Metrics) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	metricsCollector.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status: %d", w.Code)
	}
	return w.Body.String()
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: map[string][]byte{}}
}

// --- Duplicate artifact policy tests ---

func TestCreateArtifact_IdempotentOnSameSHA256(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "2.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     "aabbcc",
		SizeBytes:  100,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	body := []byte(`{"name":"myapp","version":"2.0.0","type":"app_bundle","objectKey":"artifacts/new/artifact.tar.gz","sha256":"aabbcc","sizeBytes":100}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp ArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ArtifactID != existingID {
		t.Fatalf("expected existing artifact ID %s, got %s", existingID, resp.ArtifactID)
	}
	if !resp.Duplicate {
		t.Fatalf("expected duplicate=true in response")
	}
}

func TestCreateArtifact_ConflictOnDifferentSHA256(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "2.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     "aabbcc",
		SizeBytes:  100,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	body := []byte(`{"name":"myapp","version":"2.0.0","type":"app_bundle","objectKey":"artifacts/new/artifact.tar.gz","sha256":"ddeeff","sizeBytes":100}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "artifact_version_conflict") {
		t.Fatalf("expected conflict error in body, got: %s", w.Body.String())
	}
}

func TestCreateArtifact_SupersedeBypassesCheck(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "2.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     "aabbcc",
		SizeBytes:  100,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	body := []byte(`{"name":"myapp","version":"2.0.0","type":"app_bundle","objectKey":"artifacts/new/artifact.tar.gz","sha256":"ddeeff","sizeBytes":100}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts?supersede=true", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp ArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ArtifactID == existingID {
		t.Fatalf("expected new artifact ID, got same existing ID")
	}
	if resp.Duplicate {
		t.Fatalf("expected duplicate=false for supersede path")
	}
}

func TestCompleteArtifactUpload_IdempotentOnSameSHA256(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	objStore := newFakeObjectStore()
	bucket := "artifacts"

	content := []byte("artifact content v1")
	h := sha256.New()
	h.Write(content)
	sha := hex.EncodeToString(h.Sum(nil))

	newID := uuid.NewString()
	newKey := "artifacts/" + newID + "/artifact.tar.gz"
	objStore.objects[newKey] = content

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "3.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     sha,
		SizeBytes:  int64(len(content)),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"artifactId": newID,
		"name":       "myapp",
		"version":    "3.0.0",
		"type":       "app_bundle",
		"objectKey":  newKey,
		"sha256":     sha,
		"sizeBytes":  int64(len(content)),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload/complete", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	CompleteArtifactUpload(logger, mem, objStore, bucket, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp UploadArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ArtifactID != existingID {
		t.Fatalf("expected existing artifact ID %s, got %s", existingID, resp.ArtifactID)
	}
	if !resp.Duplicate {
		t.Fatalf("expected duplicate=true")
	}
	if _, ok := objStore.objects[newKey]; ok {
		t.Fatalf("expected orphaned object to be deleted on idempotent path")
	}
}

func TestCompleteArtifactUpload_ConflictOnDifferentSHA256(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	objStore := newFakeObjectStore()
	bucket := "artifacts"

	content := []byte("artifact content v1")
	h := sha256.New()
	h.Write(content)
	sha := hex.EncodeToString(h.Sum(nil))

	newID := uuid.NewString()
	newKey := "artifacts/" + newID + "/artifact.tar.gz"
	objStore.objects[newKey] = content

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "3.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     "totallydifferentsha",
		SizeBytes:  int64(len(content)),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"artifactId": newID,
		"name":       "myapp",
		"version":    "3.0.0",
		"type":       "app_bundle",
		"objectKey":  newKey,
		"sha256":     sha,
		"sizeBytes":  int64(len(content)),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload/complete", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	CompleteArtifactUpload(logger, mem, objStore, bucket, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "artifact_version_conflict") {
		t.Fatalf("expected conflict error in body, got: %s", w.Body.String())
	}
	if _, ok := objStore.objects[newKey]; ok {
		t.Fatalf("expected orphaned object to be deleted on conflict")
	}
}

func TestCompleteArtifactUpload_SupersedeBypassesCheck(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	objStore := newFakeObjectStore()
	bucket := "artifacts"

	content := []byte("artifact content v2")
	h := sha256.New()
	h.Write(content)
	sha := hex.EncodeToString(h.Sum(nil))

	newID := uuid.NewString()
	newKey := "artifacts/" + newID + "/artifact.tar.gz"
	objStore.objects[newKey] = content

	existingID := uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: existingID,
		Name:       "myapp",
		Version:    "3.0.0",
		Type:       "app_bundle",
		Status:     "active",
		ObjectKey:  "artifacts/" + existingID + "/artifact.tar.gz",
		SHA256:     "totallydifferentsha",
		SizeBytes:  int64(len(content)),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"artifactId": newID,
		"name":       "myapp",
		"version":    "3.0.0",
		"type":       "app_bundle",
		"objectKey":  newKey,
		"sha256":     sha,
		"sizeBytes":  int64(len(content)),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload/complete?supersede=true", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	CompleteArtifactUpload(logger, mem, objStore, bucket, false, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp UploadArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ArtifactID != newID {
		t.Fatalf("expected new artifact ID %s, got %s", newID, resp.ArtifactID)
	}
	if resp.Duplicate {
		t.Fatalf("expected duplicate=false for supersede path")
	}
}

type fakeObjectStore struct {
	objects map[string][]byte
}

func (f *fakeObjectStore) PresignGet(_ context.Context, _ string, _ string, _ time.Duration) (string, error) {
	return "http://example.com/artifact", nil
}

func (f *fakeObjectStore) PresignPut(_ context.Context, _ string, key string, _ time.Duration, _ string) (string, error) {
	return "http://example.com/upload/" + key, nil
}

func (f *fakeObjectStore) PutObject(_ context.Context, _ string, key string, body io.Reader, _ int64, _ string) (int64, error) {
	data, _ := io.ReadAll(body)
	f.objects[key] = append([]byte{}, data...)
	return int64(len(data)), nil
}

func (f *fakeObjectStore) EnsureBucket(_ context.Context, _ string) error {
	return nil
}

func (f *fakeObjectStore) DeleteObject(_ context.Context, _ string, key string) error {
	delete(f.objects, key)
	return nil
}

func (f *fakeObjectStore) StatObject(_ context.Context, _ string, key string) (int64, error) {
	data, ok := f.objects[key]
	if !ok {
		return 0, io.EOF
	}
	return int64(len(data)), nil
}

func (f *fakeObjectStore) GetObject(_ context.Context, _ string, key string) (io.ReadCloser, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Ransomware control R-04: force=true must produce the distinct
// artifact.force_delete audit action (the admin RBAC elevation is enforced in
// the router's force-aware guard).
func TestDeleteArtifact_ForceDeleteAuditAction(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	artifactID := "11111111-2222-3333-4444-555555555555"
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID,
		Name:       "force-del",
		Version:    "1.0.0",
		Type:       "app_bundle",
		Status:     "active", // not deprecated: only force can delete it
		ObjectKey:  "artifacts/force-del.tar.gz",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+artifactID+"?force=true", nil)
	req = withURLParam(req, "artifactId", artifactID)
	w := httptest.NewRecorder()
	DeleteArtifact(logger, mem, newFakeObjectStore(), "artifacts", false, nil).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("force delete expected 204, got %d body=%s", w.Code, w.Body.String())
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "artifact.force_delete", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 artifact.force_delete audit event, got %d", len(events))
	}
	plain, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "artifact.delete", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(plain) != 0 {
		t.Fatalf("force delete must not be recorded as plain artifact.delete (got %d)", len(plain))
	}
}
