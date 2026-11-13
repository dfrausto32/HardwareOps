// Package httprange performs a single ranged HTTP GET for resumable artifact
// downloads. It is shared by the agent's mTLS client and the plain-HTTP
// downloader so both speak the same resume protocol (roadmap T1).
package httprange

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ChunkManifest lists the SHA-256 of each fixed-size chunk of an artifact so a
// corrupt download can be repaired chunk by chunk (roadmap T4). It is wire
// data from the control plane and is NOT trusted for acceptance: the full
// artifact SHA-256 and signature are always re-verified after any repair.
type ChunkManifest struct {
	ArtifactID string   `json:"artifactId"`
	SHA256     string   `json:"sha256"`
	Size       int64    `json:"size"`
	ChunkSize  int      `json:"chunkSize"`
	Chunks     []string `json:"chunks"`
}

// ErrNoChunkManifest means the control plane cannot provide a manifest (older
// server, or it declined); callers fall back to discarding the corrupt file.
var ErrNoChunkManifest = errors.New("chunk manifest not available")

// StatusError is returned for HTTP statuses the resume protocol does not
// handle itself (anything other than 200, 206, 416).
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("download failed: status=%d", e.Code) }

// Retryable reports whether the status is worth retrying on the same URL.
func (e StatusError) Retryable() bool {
	return e.Code >= 500 || e.Code == http.StatusTooManyRequests || e.Code == http.StatusRequestTimeout
}

// Result is the outcome of one ranged GET.
type Result struct {
	Status int           // 200, 206 or 416
	ETag   string        // validator reported by the server, may be empty
	Total  int64         // full object size when known, else -1
	Body   io.ReadCloser // nil for 416
}

// Get requests bytes [offset, end). With offset 0 and no validator it sends a
// plain GET. validator (an ETag) is sent as If-Range so a changed object is
// returned whole (200) instead of as a mismatched tail.
func Get(c *http.Client, url string, offset int64, validator string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		if validator != "" {
			req.Header.Set("If-Range", validator)
		}
	}
	resp, err := c.Do(req)
	if err != nil {
		return Result{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Status: 200, ETag: resp.Header.Get("ETag"), Total: resp.ContentLength, Body: resp.Body}, nil
	case http.StatusPartialContent:
		total := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		return Result{Status: 206, ETag: resp.Header.Get("ETag"), Total: total, Body: resp.Body}, nil
	case http.StatusRequestedRangeNotSatisfiable:
		resp.Body.Close()
		return Result{Status: 416, Total: parseContentRangeTotal(resp.Header.Get("Content-Range"))}, nil
	default:
		resp.Body.Close()
		return Result{}, StatusError{Code: resp.StatusCode}
	}
}

// parseContentRangeTotal extracts N from "bytes a-b/N" or "bytes */N"; -1 if unknown.
func parseContentRangeTotal(v string) int64 {
	i := strings.LastIndex(v, "/")
	if i < 0 {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[i+1:]), 10, 64)
	if err != nil {
		return -1
	}
	return n
}
