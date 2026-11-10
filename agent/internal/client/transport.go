package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/parcel/agent/internal/httprange"
	"github.com/parcel/agent/internal/state"
)

// Transport abstracts how the agent reaches its control plane (Phase F1).
//
// The check-in loop and apply dispatch operate only on this interface, so
// non-IP transports (BLE, serial — Phase F4/F6) can slot in without touching
// either. *Client is the HTTP/mTLS implementation.
//
// DownloadArtifact is part of the interface (beyond the original four-method
// sketch) because the apply path must fetch artifact content through the same
// transport: PresignArtifact returns a transport-specific locator and
// DownloadArtifact streams the bytes it points at. Without it, the dispatch
// would still leak an *http.Client.
//
// Enrollment, re-enrollment, and pending-enrollment flows are deliberately
// NOT on this interface: they are PKI bootstrap concerns tied to the
// transport's identity model, handled by the concrete implementation.
type Transport interface {
	// CheckIn reports current device state and returns desired state.
	CheckIn(st state.State, capabilities any) (*CheckinResponse, error)
	// GetArtifact fetches artifact metadata (hashes, signature, type).
	GetArtifact(artifactID string) (*ArtifactResponse, error)
	// PresignArtifact resolves a short-lived, transport-specific download
	// locator for the artifact content.
	PresignArtifact(artifactID string) (*PresignResponse, error)
	// PostApplyResult reports the outcome of an artifact apply.
	PostApplyResult(deviceID string, req ApplyResultRequest) error
	// DownloadArtifact streams artifact content from a locator produced by
	// PresignArtifact (or carried in desired state). The caller must close
	// the reader.
	DownloadArtifact(url string) (io.ReadCloser, error)
}

// Client implements Transport over HTTP/mTLS.
var _ Transport = (*Client)(nil)

// DownloadArtifact fetches artifact content from a presigned URL. A non-200
// response is an error (same semantics the apply path has always had).
func (c *Client) DownloadArtifact(url string) (io.ReadCloser, error) {
	if url == "" {
		return nil, fmt.Errorf("missing download url")
	}
	resp, err := c.downloadHTTP().Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}
	return resp.Body, nil
}

// DownloadArtifactRange performs one ranged GET for the resumable download
// path (roadmap T1). It satisfies artifacts.RangeDownloader; transports that
// cannot resume simply omit it and the apply path falls back to a full fetch.
func (c *Client) DownloadArtifactRange(url string, offset int64, validator string) (int, string, int64, io.ReadCloser, error) {
	if url == "" {
		return 0, "", 0, nil, fmt.Errorf("missing download url")
	}
	res, err := httprange.Get(c.downloadHTTP(), url, offset, validator)
	if err != nil {
		return 0, "", 0, nil, err
	}
	return res.Status, res.ETag, res.Total, res.Body, nil
}

// GetArtifactChunks fetches the per-chunk hash manifest used to repair a
// corrupt download at chunk granularity (roadmap T4). The control plane may
// need a full pass over the object on first request, so this uses the
// bulk-transfer client with a generous deadline instead of the 30s control
// timeout. A 404/501 (older control plane) maps to ErrNoChunkManifest.
func (c *Client) GetArtifactChunks(artifactID string, chunkSize int) (*httprange.ChunkManifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	url := fmt.Sprintf("%s/api/v1/devices/artifacts/%s/chunks?chunkSize=%d", c.baseURL, artifactID, chunkSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.downloadHTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusNotImplemented, http.StatusMethodNotAllowed:
		return nil, httprange.ErrNoChunkManifest
	default:
		return nil, fmt.Errorf("get chunk manifest failed: status=%d", resp.StatusCode)
	}
	var m httprange.ChunkManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<20)).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// downloadHTTP returns the bulk-transfer client: same transport/TLS as the
// control client but without the 30s whole-request deadline, which in Go
// includes reading the body and so would abort any longer download.
func (c *Client) downloadHTTP() *http.Client {
	if c.dl != nil {
		return c.dl
	}
	return c.http
}
