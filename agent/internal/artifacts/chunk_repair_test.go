package artifacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parcel/agent/internal/httprange"
)

const testChunk = 65536

func manifestFor(data []byte, chunk int, shaOverride string) *httprange.ChunkManifest {
	full := sha256.Sum256(data)
	sha := hex.EncodeToString(full[:])
	if shaOverride != "" {
		sha = shaOverride
	}
	m := &httprange.ChunkManifest{SHA256: sha, Size: int64(len(data)), ChunkSize: chunk}
	for off := 0; off < len(data); off += chunk {
		end := off + chunk
		if end > len(data) {
			end = len(data)
		}
		s := sha256.Sum256(data[off:end])
		m.Chunks = append(m.Chunks, hex.EncodeToString(s[:]))
	}
	return m
}

// corruptPartial writes a *complete* partial (Committed == Total) whose bytes
// are wrong in the given chunks, as if bit rot hit the on-disk file.
func corruptPartial(t *testing.T, dest string, payload []byte, sum string, badChunks ...int) {
	t.Helper()
	bad := append([]byte{}, payload...)
	for _, c := range badChunks {
		off := c * testChunk
		for i := off; i < off+100 && i < len(bad); i++ {
			bad[i] ^= 0xFF
		}
	}
	part, meta := partPaths(dest)
	if err := os.WriteFile(part, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	savePartState(meta, partState{ExpectedSHA: sum, ETag: `"v1"`, Total: int64(len(payload)), Committed: int64(len(payload))})
}

func repairCfg(src func(int) (*httprange.ChunkManifest, error)) resumeConfig {
	cfg := fastCfg()
	cfg.ChunkSource = src
	cfg.ChunkSize = testChunk
	return cfg
}

func TestRepairsOnlyCorruptChunks(t *testing.T) {
	payload, sum := testPayload(200_000) // 4 chunks, last short
	cases := []struct {
		name string
		bad  []int
	}{{"one middle chunk", []int{1}}, {"first and short last", []int{0, 3}}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := &flakyServer{payload: payload, etag: `"v1"`}
			srv := httptest.NewServer(fs)
			defer srv.Close()
			dest := filepath.Join(t.TempDir(), "a.tar.gz")
			corruptPartial(t, dest, payload, sum, c.bad...)

			cfg := repairCfg(func(cs int) (*httprange.ChunkManifest, error) { return manifestFor(payload, cs, ""), nil })
			if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
				t.Fatalf("download: %v", err)
			}
			got, _ := os.ReadFile(dest)
			if !bytes.Equal(got, payload) {
				t.Fatal("repaired content mismatch")
			}
			srv.Close()
			_, _, ranges := fs.snap()
			if len(ranges) != len(c.bad) {
				t.Fatalf("want %d range requests (one per corrupt chunk, no full re-download), got %q", len(c.bad), ranges)
			}
		})
	}
}

func TestNoChunkSourceDiscardsAsBefore(t *testing.T) {
	payload, sum := testPayload(200_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	corruptPartial(t, dest, payload, sum, 1)
	_, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want sha mismatch, got %v", err)
	}
	if exists(dest+".part") || exists(dest) {
		t.Fatal("corrupt file must be discarded without a manifest")
	}
}

func TestUnusableManifestFailsClosed(t *testing.T) {
	payload, sum := testPayload(200_000)
	other, _ := testPayload(199_999)
	var corruptDest string // set per subtest; read by the "agrees with corrupt file" source
	cases := map[string]func(int) (*httprange.ChunkManifest, error){
		"unsupported server": func(int) (*httprange.ChunkManifest, error) { return nil, httprange.ErrNoChunkManifest },
		"manifest wrong sha": func(cs int) (*httprange.ChunkManifest, error) {
			return manifestFor(payload, cs, strings.Repeat("a", 64)), nil
		},
		"manifest wrong size": func(cs int) (*httprange.ChunkManifest, error) { return manifestFor(other, cs, sum), nil },
		// Chunk hashes computed from the already-corrupt bytes (re-read from
		// disk) with the right overall sha: every chunk "matches", so the
		// corruption cannot be localised and the manifest must be distrusted.
		"manifest agrees with corrupt file": func(cs int) (*httprange.ChunkManifest, error) {
			b, err := os.ReadFile(corruptDest + ".part")
			if err != nil {
				return nil, err
			}
			return manifestFor(b, cs, sum), nil
		},
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
			defer srv.Close()
			dest := filepath.Join(t.TempDir(), "a.tar.gz")
			corruptDest = dest
			corruptPartial(t, dest, payload, sum, 2)
			_, err := downloadResumable(repairCfg(src), HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
			if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
				t.Fatalf("want sha mismatch, got %v", err)
			}
			if exists(dest) || exists(dest+".part") {
				t.Fatal("must not keep or promote a file that failed verification")
			}
		})
	}
}

func TestLyingManifestCannotMakeBadDataPass(t *testing.T) {
	payload, sum := testPayload(200_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	corruptPartial(t, dest, payload, sum, 1)

	// Manifest claims chunk 1 hashes to something the origin never serves.
	lie := manifestFor(payload, testChunk, "")
	lie.Chunks[1] = strings.Repeat("b", 64)
	cfg := repairCfg(func(int) (*httprange.ChunkManifest, error) { return lie, nil })

	var inc *DownloadIncompleteError
	for call := 1; call <= maxRepairTries; call++ {
		_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
		if !errors.As(err, &inc) || inc.Reason != "repairing corrupt chunks" {
			t.Fatalf("call %d: want repair-incomplete, got %v", call, err)
		}
		if exists(dest) {
			t.Fatal("corrupt file promoted")
		}
	}
	// Repair never converges: after the bounded retries it becomes a real failure.
	_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	if err == nil || errors.As(err, &inc) || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want hard sha mismatch after bounded repair tries, got %v", err)
	}
	if exists(dest) || exists(dest+".part") {
		t.Fatal("must be discarded")
	}
}

func TestManifestFetchNetworkErrorKeepsPartial(t *testing.T) {
	payload, sum := testPayload(200_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	corruptPartial(t, dest, payload, sum, 1)
	cfg := repairCfg(func(int) (*httprange.ChunkManifest, error) { return nil, errors.New("link down") })
	_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	var inc *DownloadIncompleteError
	if !errors.As(err, &inc) {
		t.Fatalf("transient manifest failure must defer, got %v", err)
	}
	if !exists(dest + ".part") {
		t.Fatal("partial must be kept for the next attempt")
	}
	// Link returns: repair completes.
	cfg = repairCfg(func(cs int) (*httprange.ChunkManifest, error) { return manifestFor(payload, cs, ""), nil })
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
}

func TestRepairRespectsByteBudget(t *testing.T) {
	payload, sum := testPayload(200_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	corruptPartial(t, dest, payload, sum, 1) // needs a 64 KiB re-fetch
	cfg := repairCfg(func(cs int) (*httprange.ChunkManifest, error) { return manifestFor(payload, cs, ""), nil })
	cfg.ByteBudget = 1000
	_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
	var inc *DownloadIncompleteError
	if !errors.As(err, &inc) || inc.Reason != "byte budget reached" {
		t.Fatalf("want byte-budget deferral, got %v", err)
	}
	if !exists(dest + ".part") {
		t.Fatal("partial must be kept")
	}
}

func TestRepairWorksWhenServerIgnoresRange(t *testing.T) {
	payload, sum := testPayload(200_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload) // always 200, whole body
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	corruptPartial(t, dest, payload, sum, 2)
	cfg := repairCfg(func(cs int) (*httprange.ChunkManifest, error) { return manifestFor(payload, cs, ""), nil })
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, payload) {
		t.Fatal("content mismatch")
	}
}
