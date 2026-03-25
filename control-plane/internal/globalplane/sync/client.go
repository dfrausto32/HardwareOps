package sync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hardwareops/control-plane/internal/globalplane"
)

// regionalClient is a thin HTTP client for a single regional control plane.
type regionalClient struct {
	baseURL      string
	token        string
	httpClient   *http.Client
}

// NewRegionalClient constructs a client. If tlsCAPem is non-empty it is added
// to the TLS trust pool, enabling self-signed regional CPs.
func NewRegionalClient(baseURL, token, tlsCAPem string) (*regionalClient, error) {
	return newRegionalClient(baseURL, token, tlsCAPem)
}

func newRegionalClient(baseURL, token, tlsCAPem string) (*regionalClient, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsCAPem != "" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(tlsCAPem)) {
			return nil, fmt.Errorf("regional client: failed to parse TLS CA PEM")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	}
	return &regionalClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}, nil
}

// DeviceRow is the minimal shape we need from GET /api/v1/devices.
type DeviceRow struct {
	DeviceID string            `json:"deviceId"`
	Status   string            `json:"status"`
	LastSeen *time.Time        `json:"lastSeen"`
	Labels   map[string]string `json:"labels"`
	Metadata map[string]string `json:"metadata"`
}

// ArtifactRow is the minimal shape we need from GET /api/v1/artifacts.
type ArtifactRow struct {
	ArtifactID   string     `json:"artifactId"`
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	ArtifactType string     `json:"type"`
	Status       string     `json:"status"`
	SHA256       string     `json:"sha256"`
	SizeBytes    int64      `json:"sizeBytes"`
	CreatedAt    *time.Time `json:"createdAt"`
}

// HealthSummary is the shape returned by GET /api/v1/health/summary.
type HealthSummary struct {
	TotalDevices    int        `json:"totalDevices"`
	ActiveDevices   int        `json:"activeDevices"`
	StaleDevices    int        `json:"staleDevices"`
	OfflineDevices  int        `json:"offlineDevices"`
	DegradedDevices int        `json:"degradedDevices"`
	LastDeviceSeen  *time.Time `json:"lastDeviceSeen"`
}

// FetchDevices retrieves all devices from the regional plane.
func (c *regionalClient) FetchDevices(ctx context.Context) ([]globalplane.CachedDevice, error) {
	var all []DeviceRow
	offset := 0
	limit := 500
	for {
		url := fmt.Sprintf("%s/api/v1/devices?limit=%d&offset=%d", c.baseURL, limit, offset)
		var page []DeviceRow
		if err := c.getJSON(ctx, url, &page); err != nil {
			return nil, fmt.Errorf("fetch devices: %w", err)
		}
		all = append(all, page...)
		if len(page) < limit {
			break
		}
		offset += limit
	}
	out := make([]globalplane.CachedDevice, len(all))
	for i, d := range all {
		out[i] = globalplane.CachedDevice{
			DeviceID: d.DeviceID,
			Status:   d.Status,
			LastSeen: d.LastSeen,
			Labels:   d.Labels,
			Metadata: d.Metadata,
		}
	}
	return out, nil
}

// FetchHealth retrieves the health summary from the regional plane.
func (c *regionalClient) FetchHealth(ctx context.Context) (HealthSummary, error) {
	var h HealthSummary
	url := c.baseURL + "/api/v1/health/summary"
	return h, c.getJSON(ctx, url, &h)
}

// FetchArtifacts retrieves all artifact metadata from the regional plane.
func (c *regionalClient) FetchArtifacts(ctx context.Context) ([]globalplane.CachedArtifact, error) {
	var all []ArtifactRow
	offset := 0
	limit := 500
	for {
		url := fmt.Sprintf("%s/api/v1/artifacts?limit=%d&offset=%d", c.baseURL, limit, offset)
		var page []ArtifactRow
		if err := c.getJSON(ctx, url, &page); err != nil {
			return nil, fmt.Errorf("fetch artifacts: %w", err)
		}
		all = append(all, page...)
		if len(page) < limit {
			break
		}
		offset += limit
	}
	out := make([]globalplane.CachedArtifact, len(all))
	for i, a := range all {
		out[i] = globalplane.CachedArtifact{
			ArtifactID:   a.ArtifactID,
			Name:         a.Name,
			Version:      a.Version,
			ArtifactType: a.ArtifactType,
			Status:       a.Status,
			SHA256:       a.SHA256,
			SizeBytes:    a.SizeBytes,
			CreatedAt:    a.CreatedAt,
		}
	}
	return out, nil
}

// FederationArtifactPayload is the metadata pushed to regional planes.
type FederationArtifactPayload struct {
	ArtifactID           string            `json:"artifactId"`
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Type                 string            `json:"type"`
	ObjectKey            string            `json:"objectKey"`
	SHA256               string            `json:"sha256"`
	Signature            string            `json:"signature"`
	SignatureType        string            `json:"signatureType"`
	SignatureKeyID       string            `json:"signatureKeyId"`
	SizeBytes            int64             `json:"sizeBytes"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	GlobalPresignBaseURL string            `json:"globalPresignBaseUrl"`
	GlobalObjectKey      string            `json:"globalObjectKey"`
}

// PushArtifactMetadata sends artifact metadata to a regional plane.
// Returns nil on success or idempotent re-push (200/201).
func (c *regionalClient) PushArtifactMetadata(ctx context.Context, payload FederationArtifactPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := c.baseURL + "/api/v1/federation/artifacts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	return nil
}

// GetBlobStatus asks a regional plane whether the blob has arrived locally.
func (c *regionalClient) GetBlobStatus(ctx context.Context, artifactID string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/federation/artifacts/%s/blob-status", c.baseURL, artifactID)
	var resp struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := c.getJSON(ctx, url, &resp); err != nil {
		return false, err
	}
	return resp.Confirmed, nil
}

func (c *regionalClient) getJSON(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MB limit
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return json.Unmarshal(body, dest)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
