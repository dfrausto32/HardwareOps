package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestCreateArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	body := []byte(`{"name":"agent","version":"1.0.0","objectKey":"artifacts/a.tar.gz","sha256":"abc","sizeBytes":10}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", bytes.NewReader(body))
	w := httptest.NewRecorder()

	CreateArtifact(logger, mem, false).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUploadArtifact(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	obj := &fakeObjectStore{}

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

	UploadArtifact(logger, mem, obj, "artifacts", false).ServeHTTP(w, req)

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

	depsStore := fakeObjectStore{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+artifactID+"/presign", nil)
	req = withURLParam(req, "artifactId", artifactID)
	w := httptest.NewRecorder()

	PresignArtifact(logger, mem, depsStore, "artifacts", time.Minute, false).ServeHTTP(w, req)

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

type fakeObjectStore struct{}

func (f fakeObjectStore) PresignGet(_ context.Context, _ string, _ string, _ time.Duration) (string, error) {
	return "http://example.com/artifact", nil
}

func (f fakeObjectStore) PutObject(_ context.Context, _ string, _ string, body io.Reader, _ int64, _ string) (int64, error) {
	data, _ := io.ReadAll(body)
	return int64(len(data)), nil
}

func (f fakeObjectStore) EnsureBucket(_ context.Context, _ string) error {
	return nil
}

func (f fakeObjectStore) DeleteObject(_ context.Context, _ string, _ string) error {
	return nil
}
