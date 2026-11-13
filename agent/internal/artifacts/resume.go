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
	MaxRefreshes       int           // consecutive URL refreshes without progress before failing
	Sleep              func(time.Duration)

	// Transfer controls (roadmap T3). Zero values mean "unlimited".
	MaxBytesPerSec int64               // pace body reads to at most this rate
	ByteBudget     int64               // stop (deferred, not failed) after this many bytes this call
	Throttle       func(time.Duration) // sleep used for pacing; defaults to time.Sleep
	OnProgress     func(committed, total int64)

	// ChunkSource supplies the per-chunk hash manifest used to repair a
	// download whose full digest mismatches (T4). Nil disables repair.
	ChunkSource func(chunkSize int) (*httprange.ChunkManifest, error)
	ChunkSize   int // manifest chunk size to request; 0 = 1 MiB

	budget *transferBudget
}

type transferBudget struct{ left int64 }

var errBudgetReached = errors.New("byte budget reached")

// DownloadIncompleteError reports a download that did not finish for a
// transient reason (stalled link, busy partial, byte budget). It is NOT an
// apply failure: the partial is kept and the next check-in resumes it, so
// callers must not count it toward consecutive-failure safe-halt or post a
// failed apply result.
type DownloadIncompleteError struct {
	Committed, Total int64
	Reason           string
	Err              error
}

func (e *DownloadIncompleteError) Error() string {
	msg := fmt.Sprintf("download incomplete (%s) at %d/%d bytes", e.Reason, e.Committed, e.Total)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *DownloadIncompleteError) Unwrap() error { return e.Err }

var defaultResumeConfig = resumeConfig{
	MaxStalledAttempts: 6,
	BaseBackoff:        time.Second,
	MaxBackoff:         30 * time.Second,
	IdleTimeout:        60 * time.Second,
	CheckpointBytes:    4 << 20,
	MaxRefreshes:       3,
	Sleep:              time.Sleep,
}

// ConfigureDownloadRetry overrides the in-attempt retry policy (consecutive
// attempts without progress, and the jittered backoff bounds) and returns a
// function that restores the previous policy. Intended for tests and
// operator tuning at startup; not safe to call while downloads are running.
func ConfigureDownloadRetry(maxStalled int, base, max time.Duration) (restore func()) {
	prev := defaultResumeConfig
	defaultResumeConfig.MaxStalledAttempts = maxStalled
	defaultResumeConfig.BaseBackoff = base
	defaultResumeConfig.MaxBackoff = max
	return func() { defaultResumeConfig = prev }
}

// partState is the persisted sidecar describing a partial download.
type partState struct {
	ExpectedSHA string `json:"expectedSha256"`
	ETag        string `json:"etag"`
	Total       int64  `json:"total"`
	Committed   int64  `json:"committed"`
	// RepairTries counts chunk-repair rounds after a digest mismatch (T4) so a
	// repair that never succeeds ends in a real failure, not endless deferral.
	RepairTries int `json:"repairTries,omitempty"`
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
	return downloadResumableWith(cfg, rd, url, dest, expectedSHA, nil)
}

// downloadResumableWith is downloadResumable plus URL refresh (roadmap T2).
// When the server rejects the URL with 401/403 (an expired presign), refresh
// is called for a fresh locator and the transfer continues from the same
// offset. If refresh itself fails — typically because only the object store,
// not the control plane, is reachable — that is treated as a transient stall
// (wait and retry), never a permanent failure.
func downloadResumableWith(cfg resumeConfig, rd RangeDownloader, url, dest, expectedSHA string, refresh func() (string, error)) (string, error) {
	partPath, metaPath := partPaths(dest)

	// One writer per partial: two agents/applies must not interleave bytes.
	release, err := acquirePartLock(partPath + ".lock")
	if err != nil {
		return "", &DownloadIncompleteError{Reason: "busy", Err: err}
	}
	defer release()
	if cfg.ByteBudget > 0 {
		cfg.budget = &transferBudget{left: cfg.ByteBudget}
	}
	if cfg.Throttle == nil {
		cfg.Throttle = time.Sleep
	}

	st, ok := loadPartState(metaPath)
	// A partial is only trusted when it belongs to this exact artifact hash.
	// Without a known hash, only an ETag-validated partial may resume.
	if !ok || st.ExpectedSHA != expectedSHA || (expectedSHA == "" && st.ETag == "") {
		st = partState{ExpectedSHA: expectedSHA}
		discardPartial(partPath, metaPath)
	}

	stalled := 0
	refreshes := 0 // consecutive URL refreshes that produced no progress
	var lastErr error
	for {
		if stalled > cfg.MaxStalledAttempts {
			return "", &DownloadIncompleteError{Committed: st.Committed, Total: st.Total, Reason: fmt.Sprintf("stalled after %d attempts", stalled), Err: lastErr}
		}
		if stalled > 0 {
			cfg.Sleep(backoff(cfg, stalled))
		}

		progressed, done, err := resumeAttempt(cfg, rd, url, partPath, metaPath, &st)
		if err == nil && done {
			fetch := func(off, n int64) ([]byte, error) {
				return fetchExact(cfg, rd, &url, refresh, off, n)
			}
			sum, ferr := finalizeDownload(cfg, &st, fetch, partPath, metaPath, dest, expectedSHA)
			_ = os.Remove(partPath + ".lock")
			return sum, ferr
		}
		if errors.Is(err, errBudgetReached) {
			return "", &DownloadIncompleteError{Committed: st.Committed, Total: st.Total, Reason: "byte budget reached"}
		}
		if err != nil {
			var se httprange.StatusError
			if errors.As(err, &se) && !se.Retryable() {
				expired := se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden
				if !expired || refresh == nil || refreshes >= cfg.MaxRefreshes {
					return "", err // 404 etc., or no/exhausted refresh
				}
				newURL, rerr := refresh()
				if rerr != nil {
					// Control plane unreachable: wait and retry, keep the partial.
					lastErr = fmt.Errorf("refresh download url: %w", rerr)
					stalled++
					continue
				}
				url = newURL
				refreshes++
				continue
			}
			lastErr = err
		} else {
			lastErr = errors.New("incomplete response")
		}
		if progressed {
			stalled = 0
			refreshes = 0
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

	// Chunks are capped near one second of data at the paced rate so the
	// idle-read watchdog never mistakes deliberate pacing for a dead link.
	bufSize := int64(32 << 10)
	if cfg.MaxBytesPerSec > 0 && cfg.MaxBytesPerSec < bufSize {
		bufSize = cfg.MaxBytesPerSec
		if bufSize < 512 {
			bufSize = 512
		}
	}
	buf := make([]byte, bufSize)
	sinceCheckpoint := int64(0)
	start := st.Committed
	t0 := time.Now()
	budgetHit := false
	var copyErr error
	if cfg.OnProgress != nil {
		cfg.OnProgress(st.Committed, st.Total)
	}
	for {
		p := buf
		if cfg.budget != nil {
			if cfg.budget.left <= 0 {
				budgetHit = true
				break
			}
			if int64(len(p)) > cfg.budget.left {
				p = p[:cfg.budget.left]
			}
		}
		n, rerr := body.Read(p)
		if n > 0 {
			if _, werr := f.Write(p[:n]); werr != nil {
				copyErr = werr
				break
			}
			st.Committed += int64(n)
			sinceCheckpoint += int64(n)
			if cfg.budget != nil {
				cfg.budget.left -= int64(n)
			}
			if sinceCheckpoint >= cfg.CheckpointBytes {
				if err := f.Sync(); err == nil {
					savePartState(metaPath, *st)
				}
				sinceCheckpoint = 0
				if cfg.OnProgress != nil {
					cfg.OnProgress(st.Committed, st.Total)
				}
			}
			if cfg.MaxBytesPerSec > 0 {
				want := time.Duration((st.Committed - start) * int64(time.Second) / cfg.MaxBytesPerSec)
				if d := want - time.Since(t0); d > 0 {
					cfg.Throttle(d)
				}
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
	if cfg.OnProgress != nil {
		cfg.OnProgress(st.Committed, st.Total)
	}
	if copyErr != nil {
		return progressed, false, copyErr
	}
	if budgetHit && !(st.Total > 0 && st.Committed == st.Total) {
		return progressed, false, errBudgetReached
	}
	if st.Total > 0 && st.Committed != st.Total {
		return progressed, false, fmt.Errorf("short body: have %d of %d bytes", st.Committed, st.Total)
	}
	return progressed, true, nil
}

const maxRepairTries = 3

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// finalizeDownload hashes the complete partial, enforces the expected digest
// and atomically moves it into place. On a digest mismatch, if a chunk
// manifest source is configured (roadmap T4), only the corrupt chunks are
// re-fetched and the file re-verified in full; the full SHA-256 remains the
// sole authority, so a bad manifest can never make a corrupt file pass.
// Without a usable manifest the partial is discarded (fail closed) as before.
func finalizeDownload(cfg resumeConfig, st *partState, fetch func(off, n int64) ([]byte, error), partPath, metaPath, dest, expectedSHA string) (string, error) {
	sum, err := hashFile(partPath)
	if err != nil {
		return "", err
	}
	if expectedSHA != "" && !strings.EqualFold(sum, expectedSHA) {
		mismatch := fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, sum)
		if cfg.ChunkSource == nil || st.RepairTries >= maxRepairTries {
			discardPartial(partPath, metaPath)
			return "", mismatch
		}
		// Count the attempt before trying so a repair that keeps failing
		// (e.g. the server cannot produce a manifest) cannot defer forever.
		st.RepairTries++
		savePartState(metaPath, *st)
		repaired, rerr := repairCorruptChunks(cfg, partPath, expectedSHA, fetch)
		switch {
		case errors.Is(rerr, errBudgetReached):
			return "", &DownloadIncompleteError{Committed: st.Committed, Total: st.Total, Reason: "byte budget reached"}
		case errors.Is(rerr, httprange.ErrNoChunkManifest), errors.Is(rerr, errBadManifest):
			discardPartial(partPath, metaPath)
			return "", mismatch
		case rerr != nil:
			return "", &DownloadIncompleteError{Committed: st.Committed, Total: st.Total, Reason: "repairing corrupt chunks", Err: rerr}
		}
		if sum, err = hashFile(partPath); err != nil {
			return "", err
		}
		if !strings.EqualFold(sum, expectedSHA) {
			discardPartial(partPath, metaPath)
			return "", fmt.Errorf("sha256 mismatch after repairing %d chunks: expected %s got %s", repaired, expectedSHA, sum)
		}
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
