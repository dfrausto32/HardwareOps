package client

import (
	"fmt"
	"io"
	"net/http"

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
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}
	return resp.Body, nil
}
