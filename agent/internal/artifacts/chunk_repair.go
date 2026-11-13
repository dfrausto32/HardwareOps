package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/parcel/agent/internal/httprange"
)

const defaultManifestChunkSize = 1 << 20

var errBadManifest = errors.New("chunk manifest inconsistent with downloaded file")

// repairCorruptChunks compares the complete (but digest-mismatching) partial
// against the control plane's chunk manifest and re-fetches only the chunks
// whose hash differs. Re-fetched bytes are verified against the manifest
// before being written. It returns the number of chunks repaired. The caller
// must still verify the full SHA-256: the manifest is advisory only.
func repairCorruptChunks(cfg resumeConfig, partPath, expectedSHA string, fetch func(off, n int64) ([]byte, error)) (int, error) {
	cs := cfg.ChunkSize
	if cs <= 0 {
		cs = defaultManifestChunkSize
	}
	m, err := cfg.ChunkSource(cs)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(partPath, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := fi.Size()

	// A manifest for a different object, or a malformed one, is useless.
	if m == nil || !strings.EqualFold(m.SHA256, expectedSHA) || m.Size != size ||
		m.ChunkSize <= 0 || m.ChunkSize > 64<<20 ||
		int64(len(m.Chunks)) != (size+int64(m.ChunkSize)-1)/int64(m.ChunkSize) {
		return 0, errBadManifest
	}
	for _, c := range m.Chunks {
		if len(c) != 64 {
			return 0, errBadManifest
		}
	}

	chunk := int64(m.ChunkSize)
	span := func(i int) (off, n int64) {
		off = int64(i) * chunk
		n = chunk
		if off+n > size {
			n = size - off
		}
		return
	}

	var bad []int
	buf := make([]byte, m.ChunkSize)
	for i := range m.Chunks {
		off, n := span(i)
		if _, err := f.ReadAt(buf[:n], off); err != nil {
			return 0, err
		}
		sum := sha256.Sum256(buf[:n])
		if !strings.EqualFold(hex.EncodeToString(sum[:]), m.Chunks[i]) {
			bad = append(bad, i)
		}
	}
	// Digest mismatched but every chunk matches: the manifest and the file
	// disagree about something we cannot localise. Don't trust the manifest.
	if len(bad) == 0 {
		return 0, errBadManifest
	}

	repaired := 0
	for _, i := range bad {
		off, n := span(i)
		data, err := fetch(off, n)
		if err != nil {
			return repaired, err
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), m.Chunks[i]) {
			return repaired, fmt.Errorf("re-fetched chunk %d failed verification", i)
		}
		if _, err := f.WriteAt(data, off); err != nil {
			return repaired, err
		}
		repaired++
	}
	if err := f.Sync(); err != nil {
		return repaired, err
	}
	return repaired, nil
}

// fetchExact reads exactly n bytes at offset off via a ranged GET and closes
// the body (so only that span is transferred). It honours the byte budget and
// bandwidth cap, and refreshes an expired presigned URL once.
func fetchExact(cfg resumeConfig, rd RangeDownloader, url *string, refresh func() (string, error), off, n int64) ([]byte, error) {
	if cfg.budget != nil {
		if cfg.budget.left < n {
			return nil, errBudgetReached
		}
		cfg.budget.left -= n
	}
	var lastErr error
	for try := 0; try < 2; try++ {
		status, _, _, body, err := rd.DownloadArtifactRange(*url, off, "")
		if err != nil {
			var se httprange.StatusError
			if try == 0 && refresh != nil && errors.As(err, &se) &&
				(se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
				if nu, rerr := refresh(); rerr == nil {
					*url = nu
					continue
				}
			}
			return nil, err
		}
		if body == nil {
			return nil, fmt.Errorf("range fetch at %d: status %d", off, status)
		}
		rc := newIdleBody(body, cfg.IdleTimeout)
		if status == http.StatusOK && off > 0 {
			// Server ignored the range: skip to the offset (wasteful but correct).
			if _, err := io.CopyN(io.Discard, rc, off); err != nil {
				rc.Close()
				lastErr = err
				continue
			}
		}
		data := make([]byte, n)
		_, err = io.ReadFull(rc, data)
		rc.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if cfg.MaxBytesPerSec > 0 {
			throttle := cfg.Throttle
			if throttle == nil {
				throttle = time.Sleep
			}
			throttle(time.Duration(n * int64(time.Second) / cfg.MaxBytesPerSec))
		}
		return data, nil
	}
	return nil, lastErr
}
