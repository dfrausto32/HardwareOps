package artifacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPayload(n int) ([]byte, string) {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	s := sha256.Sum256(b)
	return b, hex.EncodeToString(s[:])
}

func fastCfg() resumeConfig {
	return resumeConfig{
		MaxStalledAttempts: 5,
		BaseBackoff:        time.Millisecond,
		MaxBackoff:         time.Millisecond,
		IdleTimeout:        2 * time.Second,
		CheckpointBytes:    1024,
		Sleep:              func(time.Duration) {},
	}
}

// flakyServer serves payload with Range/If-Range support and drops the
// connection after `cut` bytes of each response for the first `drops` requests.
type flakyServer struct {
	mu        sync.Mutex
	payload   []byte
	etag      string
	drops     int
	cut       int
	requests  int
	bytesOut  int
	ranges    []string
	status    int // if non-zero, respond with this status instead
	failAfter int // if >0, answer 503 once this many requests were served
}

func (f *flakyServer) snap() (requests, bytesOut int, ranges []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.bytesOut, append([]string(nil), f.ranges...)
}

func (f *flakyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	n := f.requests
	f.ranges = append(f.ranges, r.Header.Get("Range"))
	etag, payload, status := f.etag, f.payload, f.status
	drop := n <= f.drops
	f.mu.Unlock()

	if f.failAfter > 0 && n > f.failAfter {
		status = http.StatusServiceUnavailable
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	start := 0
	if rg := r.Header.Get("Range"); rg != "" {
		ifr := r.Header.Get("If-Range")
		if ifr == "" || ifr == etag {
			v := strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-")
			start, _ = strconv.Atoi(v)
			if start >= len(payload) {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(payload)))
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
		}
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)-start))
	if start > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
	}
	body := payload[start:]
	if drop && f.cut < len(body) {
		_, _ = w.Write(body[:f.cut])
		f.mu.Lock()
		f.bytesOut += f.cut
		f.mu.Unlock()
		w.(http.Flusher).Flush()
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close() // abort mid-body
		return
	}
	_, _ = w.Write(body)
	f.mu.Lock()
	f.bytesOut += len(body)
	f.mu.Unlock()
}

func TestResumeAcrossRepeatedDrops(t *testing.T) {
	payload, sum := testPayload(100_000)
	fs := &flakyServer{payload: payload, etag: `"v1"`, drops: 3, cut: 20_000}
	srv := httptest.NewServer(fs)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	got, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if got != sum {
		t.Fatalf("sum %s != %s", got, sum)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch")
	}
	// Resume, not restart: total bytes sent ≈ payload size, not 4x.
	srv.Close()
	_, out, ranges := fs.snap()
	if out != len(payload) {
		t.Fatalf("bytes transferred %d, want exactly %d (resume, not restart)", out, len(payload))
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("partial file should be gone")
	}
	if ranges[1] == "" {
		t.Fatalf("second request had no Range header: %v", ranges)
	}
}

func TestResumeAcrossAgentRestart(t *testing.T) {
	payload, sum := testPayload(50_000)
	dir := t.TempDir()
	dest := filepath.Join(dir, "a.tar.gz")

	// First "process": one partial transfer, then the link goes fully dark
	// (503s) until the retry budget is exhausted and the apply gives up.
	fs := &flakyServer{payload: payload, etag: `"v1"`, drops: 1, cut: 12_000, failAfter: 1}
	srv := httptest.NewServer(fs)
	cfg := fastCfg()
	cfg.MaxStalledAttempts = 2
	_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	srv.Close()
	if err == nil {
		t.Fatal("expected failure on first run")
	}
	st, ok := loadPartState(dest + ".part.meta")
	if !ok || st.Committed == 0 {
		t.Fatalf("expected persisted progress, got %+v ok=%v", st, ok)
	}

	// Second "process": healthy server; must continue from the checkpoint.
	fs2 := &flakyServer{payload: payload, etag: `"v1"`}
	srv2 := httptest.NewServer(fs2)
	defer srv2.Close()
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv2.Client()}, srv2.URL, dest, sum); err != nil {
		t.Fatalf("second run: %v", err)
	}
	srv2.Close()
	if _, out, _ := fs2.snap(); out >= len(payload) {
		t.Fatalf("second run re-sent %d bytes; expected only the tail", out)
	}
}

func TestChangedETagRestartsFromZero(t *testing.T) {
	payload, sum := testPayload(30_000)
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	part, meta := partPaths(dest)
	// Stale partial recorded against a different ETag, with garbage content.
	_ = os.WriteFile(part, bytes.Repeat([]byte{0xEE}, 10_000), 0o600)
	savePartState(meta, partState{ExpectedSHA: sum, ETag: `"old"`, Total: 30_000, Committed: 10_000})

	fs := &flakyServer{payload: payload, etag: `"new"`}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("stale prefix leaked into result")
	}
}

func TestStalePartialForOtherArtifactDiscarded(t *testing.T) {
	payload, sum := testPayload(8_000)
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	part, meta := partPaths(dest)
	_ = os.WriteFile(part, bytes.Repeat([]byte{1}, 4_000), 0o600)
	savePartState(meta, partState{ExpectedSHA: "deadbeef", ETag: `"v1"`, Total: 8_000, Committed: 4_000})

	fs := &flakyServer{payload: payload, etag: `"v1"`}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	srv.Close()
	if _, _, ranges := fs.snap(); ranges[0] != "" {
		t.Fatalf("must not resume a partial for a different artifact hash; Range=%q", ranges[0])
	}
}

func TestSHAMismatchDiscardsPartialAndFailsClosed(t *testing.T) {
	payload, _ := testPayload(5_000)
	fs := &flakyServer{payload: payload, etag: `"v1"`}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	_, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha mismatch, got %v", err)
	}
	for _, p := range []string{dest, dest + ".part", dest + ".part.meta"} {
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatalf("%s should not exist after mismatch", p)
		}
	}
}

func TestServerIgnoresRangeRestartsCleanly(t *testing.T) {
	payload, sum := testPayload(20_000)
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	part, meta := partPaths(dest)
	_ = os.WriteFile(part, payload[:5_000], 0o600)
	savePartState(meta, partState{ExpectedSHA: sum, ETag: `"v1"`, Total: 20_000, Committed: 5_000})

	// Plain handler that always returns 200 with the full body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch after 200 restart")
	}
}

func TestPermanentStatusNotRetriedPartialKept(t *testing.T) {
	fs := &flakyServer{status: http.StatusForbidden}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	_, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, "")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want 403 error, got %v", err)
	}
	srv.Close()
	if reqs, _, _ := fs.snap(); reqs != 1 {
		t.Fatalf("403 must not be retried, got %d requests", reqs)
	}
}

func TestTransientStatusRetried(t *testing.T) {
	payload, sum := testPayload(3_000)
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		c := calls
		mu.Unlock()
		if c <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 calls, got %d", calls)
	}
}

func TestIdleStallAborted(t *testing.T) {
	payload, sum := testPayload(10_000)
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		c := calls
		mu.Unlock()
		if c == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload[:1000])
			w.(http.Flusher).Flush()
			time.Sleep(600 * time.Millisecond) // stall past the idle timeout
			return
		}
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(payload[start:])
	}))
	defer srv.Close()
	cfg := fastCfg()
	cfg.IdleTimeout = 150 * time.Millisecond
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch after stall recovery")
	}
}

// Non-range transports keep the old single-shot behavior.
type plainDownloader struct{ data []byte }

func (p plainDownloader) DownloadArtifact(string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(p.data)), nil
}

func TestNonRangeDownloaderFallsBack(t *testing.T) {
	payload, sum := testPayload(2_000)
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	got, err := downloadAndVerify(plainDownloader{payload}, "stub://x", dest, sum)
	if err != nil || got != sum {
		t.Fatalf("fallback failed: %v %s", err, got)
	}
}
