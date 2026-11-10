package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/parcel/agent/internal/httprange"
)

// RangeDownloader is implemented by transports that can fetch a byte range of
// an artifact. It uses only builtin types so transport packages need not
// import this package. status is 200 (server ignored the range / object
// changed: body is the whole object), 206 (body continues at offset) or 416
// (offset not satisfiable; body is nil). total is the full object size or -1.
type RangeDownloader interface {
	DownloadArtifactRange(url string, offset int64, validator string) (status int, etag string, total int64, body io.ReadCloser, err error)
}

// DownloadArtifactRange makes HTTPDownloader resumable.
func (d HTTPDownloader) DownloadArtifactRange(url string, offset int64, validator string) (int, string, int64, io.ReadCloser, error) {
	res, err := httprange.Get(d.Client, url, offset, validator)
	if err != nil {
		return 0, "", 0, nil, err
	}
	return res.Status, res.ETag, res.Total, res.Body, nil
}

type resumeConfig struct {
	MaxStalledAttempts int           // consecutive attempts without progress before giving up
	BaseBackoff        time.Duration // first retry delay (full jitter applied)
	MaxBackoff         time.Duration
	IdleTimeout        time.Duration // abort a body read that delivers no bytes for this long
	CheckpointBytes    int64         // fsync + persist sidecar every N bytes
	Sleep              func(time.Duration)
}

var defaultResumeConfig = resumeConfig{
	MaxStalledAttempts: 6,
	BaseBackoff:        time.Second,
	MaxBackoff:         30 * time.Second,
	IdleTimeout:        60 * time.Second,
	CheckpointBytes:    4 << 20,
	Sleep:              time.Sleep,
}

// partState is the persisted sidecar describing a partial download.
type partState struct {
	ExpectedSHA string `json:"expectedSha256"`
	ETag        string `json:"etag"`
	Total       int64  `json:"total"`
	Committed   int64  `json:"committed"`
}

func partPaths(dest string) (part, meta string) { return dest + ".part", dest + ".part.meta" }

func loadPartState(metaPath string) (partState, bool) {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return partState{}, false
	}
	var s partState
	if json.Unmarshal(b, &s) != nil {
		return partState{}, false
	}
	return s, true
}

func savePartState(metaPath string, s partState) {
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp := metaPath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, metaPath)
	}
}

func discardPartial(partPath, metaPath string) {
	_ = os.Remove(partPath)
	_ = os.Remove(metaPath)
}

// downloadResumable fetches url into dest, resuming from a persisted partial
// file across connection drops and agent restarts. The finished file is only
// moved into place after its full SHA-256 matches expectedSHA; a mismatch
// discards the partial (fail closed). Transient failures keep the partial so
// the next attempt, or the next check-in, continues from the last byte.
func downloadResumable(cfg resumeConfig, rd RangeDownloader, url, dest, expectedSHA string) (string, error) {
	partPath, metaPath := partPaths(dest)

	st, ok := loadPartState(metaPath)
	// A partial is only trusted when it belongs to this exact artifact hash.
	// Without a known hash, only an ETag-validated partial may resume.
	if !ok || st.ExpectedSHA != expectedSHA || (expectedSHA == "" && st.ETag == "") {
		st = partState{ExpectedSHA: expectedSHA}
		discardPartial(partPath, metaPath)
	}

	stalled := 0
	var lastErr error
	for {
		if stalled > cfg.MaxStalledAttempts {
			return "", fmt.Errorf("download stalled after %d attempts (partial kept at %d bytes): %w", stalled, st.Committed, lastErr)
		}
		if stalled > 0 {
			cfg.Sleep(backoff(cfg, stalled))
		}

		progressed, done, err := resumeAttempt(cfg, rd, url, partPath, metaPath, &st)
		if err == nil && done {
			return finalizeDownload(partPath, metaPath, dest, expectedSHA)
		}
		if err != nil {
			var se httprange.StatusError
			if errors.As(err, &se) && !se.Retryable() {
				return "", err // e.g. 401/403 expired presign (T2), 404
			}
			lastErr = err
		} else {
			lastErr = errors.New("incomplete response")
		}
		if progressed {
			stalled = 0
		} else {
			stalled++
		}
	}
}

// resumeAttempt performs one ranged GET and streams it into the partial file.
// done is true when the full object has been received.
func resumeAttempt(cfg resumeConfig, rd RangeDownloader, url, partPath, metaPath string, st *partState) (progressed, done bool, err error) {
	// Reconcile the file with the sidecar: never trust bytes past the last
	// recorded checkpoint, and drop a partial that is shorter than recorded.
	if fi, e := os.Stat(partPath); e != nil || fi.Size() < st.Committed {
		st.Committed, st.ETag, st.Total = 0, "", 0
	}
	f, err := os.OpenFile(partPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return false, false, err
	}
	defer f.Close()
	if err := f.Truncate(st.Committed); err != nil {
		return false, false, err
	}

	// Everything already on disk is complete: skip the request.
	if st.Total > 0 && st.Committed == st.Total {
		return false, true, nil
	}

	status, etag, total, body, err := rd.DownloadArtifactRange(url, st.Committed, st.ETag)
	if err != nil {
		return false, false, err
	}

	switch status {
	case http.StatusRequestedRangeNotSatisfiable:
		// Our offset is beyond the object: the partial is bogus. Start over.
		if body != nil {
			body.Close()
		}
		st.Committed, st.ETag, st.Total = 0, "", 0
		savePartState(metaPath, *st)
		return false, false, errors.New("range not satisfiable; restarting from zero")
	case http.StatusOK:
		// Server sent the whole object (range ignored or validator changed).
		st.Committed, st.Total = 0, total
		st.ETag = etag
		if err := f.Truncate(0); err != nil {
			body.Close()
			return false, false, err
		}
	case http.StatusPartialContent:
		if st.ETag != "" && etag != "" && etag != st.ETag {
			// Object changed under us despite If-Range: discard and restart.
			body.Close()
			st.Committed, st.ETag, st.Total = 0, "", 0
			return false, false, errors.New("object changed during resume; restarting from zero")
		}
		if st.ETag == "" {
			st.ETag = etag
		}
		if total > 0 {
			st.Total = total
		}
	default:
		if body != nil {
			body.Close()
		}
		return false, false, fmt.Errorf("unexpected download status %d", status)
	}
	if st.Total <= 0 {
		st.Total = total
	}

	body = newIdleBody(body, cfg.IdleTimeout)
	defer body.Close()

	if _, err := f.Seek(st.Committed, io.SeekStart); err != nil {
		return false, false, err
	}

	buf := make([]byte, 32<<10)
	sinceCheckpoint := int64(0)
	start := st.Committed
	var copyErr error
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				copyErr = werr
				break
			}
			st.Committed += int64(n)
			sinceCheckpoint += int64(n)
			if sinceCheckpoint >= cfg.CheckpointBytes {
				if err := f.Sync(); err == nil {
					savePartState(metaPath, *st)
				}
				sinceCheckpoint = 0
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			copyErr = rerr
			break
		}
	}

	// Persist whatever we safely wrote so the next attempt continues here.
	if err := f.Sync(); err == nil {
		savePartState(metaPath, *st)
	} else if copyErr == nil {
		copyErr = err
	}
	progressed = st.Committed > start
	if copyErr != nil {
		return progressed, false, copyErr
	}
	if st.Total > 0 && st.Committed != st.Total {
		return progressed, false, fmt.Errorf("short body: have %d of %d bytes", st.Committed, st.Total)
	}
	return progressed, true, nil
}

// finalizeDownload hashes the complete partial, enforces the expected digest
// and atomically moves it into place.
func finalizeDownload(partPath, metaPath, dest, expectedSHA string) (string, error) {
	f, err := os.Open(partPath)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(sum, expectedSHA) {
		discardPartial(partPath, metaPath)
		return "", fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, sum)
	}
	if err := os.Rename(partPath, dest); err != nil {
		return "", fmt.Errorf("rename download: %w", err)
	}
	_ = os.Remove(metaPath)
	return sum, nil
}

// backoff returns an exponential delay with full jitter for the nth stalled attempt.
func backoff(cfg resumeConfig, n int) time.Duration {
	d := cfg.BaseBackoff << uint(n-1)
	if d <= 0 || d > cfg.MaxBackoff {
		d = cfg.MaxBackoff
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(d) + 1))
}

// idleBody closes the underlying body if no bytes arrive within d, turning a
// silent stall on a dead link into a retryable read error.
type idleBody struct {
	rc io.ReadCloser
	d  time.Duration
	t  *time.Timer
}

func newIdleBody(rc io.ReadCloser, d time.Duration) io.ReadCloser {
	b := &idleBody{rc: rc, d: d}
	if d > 0 {
		b.t = time.AfterFunc(d, func() { _ = rc.Close() })
	}
	return b
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if b.t != nil && n > 0 {
		b.t.Reset(b.d)
	}
	return n, err
}

func (b *idleBody) Close() error {
	if b.t != nil {
		b.t.Stop()
	}
	return b.rc.Close()
}
