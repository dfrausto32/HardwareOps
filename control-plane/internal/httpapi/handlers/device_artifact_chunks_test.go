package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func chunkTestData(n int) ([]byte, string) {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*13 + i/97)
	}
	s := sha256.Sum256(b)
	return b, hex.EncodeToString(s[:])
}

type countingObjStore struct {
	*fakeObjectStore
	gets atomic.Int32
}

func (c *countingObjStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	c.gets.Add(1)
	time.Sleep(20 * time.Millisecond) // widen the window for concurrent callers
	return c.fakeObjectStore.GetObject(ctx, bucket, key)
}

// setupChunkRequest registers an artifact (with the given registered sha) and
// assigns it to a fresh mTLS device, returning a ready request builder.
func setupChunkRequest(t *testing.T, mem *memory.Store, objKey, sha string, size int64, assign bool) (artifactID string, newReq func(query string) *http.Request) {
	t.Helper()
	artifactID = uuid.NewString()
	if err := mem.CreateArtifact(store.Artifact{
		ArtifactID: artifactID, Name: "a", Version: "1.0.0", ObjectKey: objKey,
		SHA256: sha, SizeBytes: size, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	first := true
	var deviceID string
	return artifactID, func(query string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/artifacts/"+artifactID+"/chunks"+query, nil)
		req = withURLParam(req, "artifactId", artifactID)
		if first {
			req, deviceID = attachMTLSDevice(t, mem, req, "")
			if assign {
				if err := mem.UpsertDesiredStateDevice(store.DesiredStateDevice{
					DeviceID: deviceID, ArtifactID: artifactID, DesiredVersion: "1.0.0",
					Source: "manual", UpdatedAt: time.Now().UTC(),
				}); err != nil {
					t.Fatal(err)
				}
			}
			first = false
		} else {
			req = attachExistingMTLSDevice(t, mem, req, deviceID)
		}
		return req
	}
}

func TestChunkManifestHappyPathAndCached(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	data, sha := chunkTestData(300_000)
	objs := &fakeObjectStore{objects: map[string][]byte{"k": data}}
	_, newReq := setupChunkRequest(t, mem, "k", sha, int64(len(data)), true)
	h := GetAssignedArtifactChunks(logger, mem, objs, "bucket", false, "")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, newReq("?chunkSize=65536"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var m ChunkManifest
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.SHA256 != sha || m.Size != 300_000 || m.ChunkSize != 65536 || len(m.Chunks) != 5 {
		t.Fatalf("bad manifest header: %+v", m)
	}
	for i, c := range m.Chunks {
		lo, hi := i*65536, (i+1)*65536
		if hi > len(data) {
			hi = len(data)
		}
		s := sha256.Sum256(data[lo:hi])
		if c != hex.EncodeToString(s[:]) {
			t.Fatalf("chunk %d hash mismatch", i)
		}
	}

	// Content is immutable: second request is served from cache without
	// touching the object store (prove it by removing the object).
	delete(objs.objects, "k")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, newReq("?chunkSize=65536"))
	if w2.Code != http.StatusOK {
		t.Fatalf("cached request should succeed, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestChunkManifestConcurrentRequestsComputeOnce(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	data, sha := chunkTestData(200_000)
	objs := &countingObjStore{fakeObjectStore: &fakeObjectStore{objects: map[string][]byte{"k": data}}}
	_, newReq := setupChunkRequest(t, mem, "k", sha, int64(len(data)), true)
	h := GetAssignedArtifactChunks(logger, mem, objs, "bucket", false, "")

	// One authenticated request shared read-only by all goroutines: each
	// attach* call re-issues the device cert, which would invalidate the others.
	req := newReq("")
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("request %d: %d", i, c)
		}
	}
	if g := objs.gets.Load(); g != 1 {
		t.Fatalf("object read %d times; concurrent requests must share one pass", g)
	}
}

func TestChunkManifestRefusesObjectNotMatchingRegisteredSHA(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	data, _ := chunkTestData(100_000)
	objs := &fakeObjectStore{objects: map[string][]byte{"k": data}}
	_, newReq := setupChunkRequest(t, mem, "k", "00"+hex.EncodeToString(make([]byte, 31)), int64(len(data)), true)
	h := GetAssignedArtifactChunks(logger, mem, objs, "bucket", false, "")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, newReq(""))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("tampered/replaced object must not get a manifest, got %d", w.Code)
	}
}

func TestChunkManifestRequiresAssignment(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	data, sha := chunkTestData(1000)
	objs := &fakeObjectStore{objects: map[string][]byte{"k": data}}
	_, newReq := setupChunkRequest(t, mem, "k", sha, int64(len(data)), false)
	w := httptest.NewRecorder()
	GetAssignedArtifactChunks(logger, mem, objs, "bucket", false, "").ServeHTTP(w, newReq(""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("unassigned artifact must be 403, got %d", w.Code)
	}
}

func TestChunkManifestRejectsBadChunkSize(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	data, sha := chunkTestData(1000)
	objs := &fakeObjectStore{objects: map[string][]byte{"k": data}}
	_, newReq := setupChunkRequest(t, mem, "k", sha, int64(len(data)), true)
	h := GetAssignedArtifactChunks(logger, mem, objs, "bucket", false, "")
	for _, q := range []string{"?chunkSize=abc", "?chunkSize=1024", "?chunkSize=100000", "?chunkSize=16777216", "?chunkSize=-65536"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newReq(q))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", q, w.Code)
		}
	}
}

func TestComputeChunkManifest(t *testing.T) {
	cases := []struct{ size, chunk, wantChunks int }{
		{0, 64, 0}, {1, 64, 1}, {64, 64, 1}, {65, 64, 2}, {192, 64, 3}, {200, 64, 4},
	}
	for _, c := range cases {
		data, sha := chunkTestData(c.size)
		chunks, full, size, err := computeChunkManifest(bytes.NewReader(data), c.chunk)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunks) != c.wantChunks || size != int64(c.size) {
			t.Errorf("size=%d chunk=%d: got %d chunks size %d", c.size, c.chunk, len(chunks), size)
		}
		if c.size > 0 && full != sha {
			t.Errorf("size=%d: full sha mismatch", c.size)
		}
	}
}
