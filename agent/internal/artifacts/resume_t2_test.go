package artifacts

import (
	"bytes"
	"errors"
	"fmt"
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

// expiringServer models a presigned store: "/old" is an expired URL that
// serves one truncated body then rejects every ranged request with 403;
// "/new" is a valid URL with full Range support.
func expiringServer(payload []byte, cut int) (*httptest.Server, *int, *sync.Mutex) {
	var mu sync.Mutex
	oldHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			mu.Lock()
			oldHits++
			first := oldHits == 1
			mu.Unlock()
			if !first {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload[:cut])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		start := 0
		w.Header().Set("ETag", `"v1"`)
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(payload[start:])
	}))
	return srv, &oldHits, &mu
}

func TestExpiredPresignRefreshedAndResumed(t *testing.T) {
	payload, sum := testPayload(60_000)
	srv, _, _ := expiringServer(payload, 15_000)
	defer srv.Close()

	refreshes := 0
	refresh := func() (string, error) { refreshes++; return srv.URL + "/new", nil }
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	if _, err := downloadResumableWith(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/old", dest, sum, refresh); err != nil {
		t.Fatalf("download: %v", err)
	}
	if refreshes != 1 {
		t.Fatalf("want exactly 1 refresh, got %d", refreshes)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch after refresh")
	}
}

func TestRefreshFailureIsTransientAndPartialKept(t *testing.T) {
	payload, sum := testPayload(40_000)
	srv, _, _ := expiringServer(payload, 10_000)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")

	// Control plane unreachable: refresh fails. Must not be a permanent error
	// and must leave the partial for the next attempt.
	down := func() (string, error) { return "", errors.New("control plane unreachable") }
	_, err := downloadResumableWith(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/old", dest, sum, down)
	if err == nil || !strings.Contains(err.Error(), "refresh download url") {
		t.Fatalf("want refresh stall error, got %v", err)
	}
	st, ok := loadPartState(dest + ".part.meta")
	if !ok || st.Committed != 10_000 {
		t.Fatalf("partial should be kept at 10000 bytes, got %+v ok=%v", st, ok)
	}

	// Connectivity returns: next attempt finishes from the saved offset.
	up := func() (string, error) { return srv.URL + "/new", nil }
	if _, err := downloadResumableWith(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/old", dest, sum, up); err != nil {
		t.Fatalf("second run: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch")
	}
}

func TestRefreshGivesUpWhenNewURLAlsoRejected(t *testing.T) {
	payload, sum := testPayload(20_000)
	srv, _, _ := expiringServer(payload, 5_000)
	defer srv.Close()
	n := 0
	refresh := func() (string, error) { n++; return srv.URL + "/old", nil } // still expired
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	_, err := downloadResumableWith(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/old", dest, sum, refresh)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want 403, got %v", err)
	}
	if n != 3 {
		t.Fatalf("refresh attempts = %d, want bounded at 3", n)
	}
}

func TestBytesPastCheckpointAreDiscardedOnResume(t *testing.T) {
	payload, sum := testPayload(30_000)
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	part, meta := partPaths(dest)
	// Simulates kill -9: file holds 9000 bytes but only 5000 were checkpointed;
	// the tail past the checkpoint is garbage (torn write).
	torn := append(append([]byte{}, payload[:5_000]...), bytes.Repeat([]byte{0xAB}, 4_000)...)
	_ = os.WriteFile(part, torn, 0o600)
	savePartState(meta, partState{ExpectedSHA: sum, ETag: `"v1"`, Total: 30_000, Committed: 5_000})

	srv, _, _ := expiringServer(payload, len(payload))
	defer srv.Close()
	if _, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/new", dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("torn tail leaked into result")
	}
}

func TestConcurrentDownloaderRejectedByLock(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	release, err := acquirePartLock(dest + ".part.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	payload, sum := testPayload(1_000)
	srv, _, _ := expiringServer(payload, len(payload))
	defer srv.Close()
	_, err = downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL+"/new", dest, sum)
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("want lock rejection, got %v", err)
	}
}

func writeAged(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, bytes.Repeat([]byte{1}, size), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Add(-age)
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestPruneRemovesOnlyAbandonedPartials(t *testing.T) {
	dir := t.TempDir()
	day := 24 * time.Hour
	writeAged(t, filepath.Join(dir, "old.tar.gz.part"), 100, 10*day)
	writeAged(t, filepath.Join(dir, "old.tar.gz.part.meta"), 10, 10*day)
	writeAged(t, filepath.Join(dir, "fresh.tar.gz.part"), 100, time.Hour)
	writeAged(t, filepath.Join(dir, "keep.tar.gz.part"), 100, 10*day)
	writeAged(t, filepath.Join(dir, "orphan.tar.gz.part.meta"), 10, 10*day)
	writeAged(t, filepath.Join(dir, "done.tar.gz"), 100, 10*day) // finished download: untouched

	pruneStalePartials(dir, filepath.Join(dir, "keep.tar.gz.part"), 7*day, 1<<30)

	for _, gone := range []string{"old.tar.gz.part", "old.tar.gz.part.meta", "orphan.tar.gz.part.meta"} {
		if exists(filepath.Join(dir, gone)) {
			t.Errorf("%s should have been pruned", gone)
		}
	}
	for _, kept := range []string{"fresh.tar.gz.part", "keep.tar.gz.part", "done.tar.gz"} {
		if !exists(filepath.Join(dir, kept)) {
			t.Errorf("%s should have been kept", kept)
		}
	}
}

func TestPruneEnforcesSizeCapOldestFirstAndSkipsLocked(t *testing.T) {
	dir := t.TempDir()
	writeAged(t, filepath.Join(dir, "a.part"), 1000, 3*time.Hour) // oldest
	writeAged(t, filepath.Join(dir, "b.part"), 1000, 2*time.Hour)
	writeAged(t, filepath.Join(dir, "c.part"), 1000, time.Hour)

	// a is being downloaded by someone else: must survive even though oldest.
	release, err := acquirePartLock(filepath.Join(dir, "a.part.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	pruneStalePartials(dir, "", 7*24*time.Hour, 2000)

	if !exists(filepath.Join(dir, "a.part")) {
		t.Error("locked partial must not be pruned")
	}
	if exists(filepath.Join(dir, "b.part")) {
		t.Error("b (next oldest) should have been pruned to meet the cap")
	}
	if !exists(filepath.Join(dir, "c.part")) {
		t.Error("c should remain once under the cap")
	}
}
