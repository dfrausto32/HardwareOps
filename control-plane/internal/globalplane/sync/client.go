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

	"github.com/parcel/control-plane/internal/globalplane"
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
// Labels and Metadata are map[string]interface{} because the regional plane
// stores them as json.RawMessage — values may not be plain strings.
type DeviceRow struct {
	DeviceID string                 `json:"deviceId"`
	Status   string                 `json:"status"`
	LastSeen *time.Time             `json:"lastSeen"`
	Labels   map[string]interface{} `json:"labels"`
	Metadata map[string]interface{} `json:"metadata"`
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
// The regional plane returns a paginated object {"items":[...],"total":N}.
func (c *regionalClient) FetchDevices(ctx context.Context) ([]globalplane.CachedDevice, error) {
	var all []DeviceRow
	offset := 0
	limit := 500
	for {
		url := fmt.Sprintf("%s/api/v1/devices?limit=%d&offset=%d", c.baseURL, limit, offset)
		var resp struct {
			Items []DeviceRow `json:"items"`
		}
		if err := c.getJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("fetch devices: %w", err)
		}
		all = append(all, resp.Items...)
		if len(resp.Items) < limit {
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
			Labels:   toStringMap(d.Labels),
			Metadata: toStringMap(d.Metadata),
		}
	}
	return out, nil
}

// toStringMap converts map[string]interface{} to map[string]string, keeping
// only entries whose value is a JSON string. Non-string values (objects,
// numbers, booleans) are silently skipped so they don't cause unmarshal errors.
func toStringMap(m map[string]interface{}) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// FetchHealth retrieves the health summary from the regional plane.
func (c *regionalClient) FetchHealth(ctx context.Context) (HealthSummary, error) {
	var h HealthSummary
	url := c.baseURL + "/api/v1/health/summary"
	return h, c.getJSON(ctx, url, &h)
}

// FetchArtifacts retrieves all artifact metadata from the regional plane.
// The regional plane returns a paginated object {"items":[...],"total":N}.
func (c *regionalClient) FetchArtifacts(ctx context.Context) ([]globalplane.CachedArtifact, error) {
	var all []ArtifactRow
	offset := 0
	limit := 500
	for {
		url := fmt.Sprintf("%s/api/v1/artifacts?limit=%d&offset=%d", c.baseURL, limit, offset)
		var resp struct {
			Items []ArtifactRow `json:"items"`
		}
		if err := c.getJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("fetch artifacts: %w", err)
		}
		all = append(all, resp.Items...)
		if len(resp.Items) < limit {
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

// FederationPolicyPayload is the metadata pushed to regional planes for global desired state.
type FederationPolicyPayload struct {
	GroupID          string `json:"groupId"`
	GroupName        string `json:"groupName"`
	SelectorJSON     []byte `json:"selectorJson"`
	ArtifactID       string `json:"artifactId"`
	DesiredVersion   string `json:"desiredVersion"`
	DesiredConfigRev string `json:"desiredConfigRev"`
	PolicyJSON       []byte `json:"policyJson"`
	ComponentsJSON   []byte `json:"componentsJson"`
	CheckinInterval  int    `json:"checkinInterval"`
}

// PushPolicy sends a global desired state policy to a regional plane.
func (c *regionalClient) PushPolicy(ctx context.Context, payload FederationPolicyPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := c.baseURL + "/api/v1/federation/policies"
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

// DeletePolicy removes a global desired state policy from a regional plane.
func (c *regionalClient) DeletePolicy(ctx context.Context, groupID string) error {
	url := fmt.Sprintf("%s/api/v1/federation/policies/%s", c.baseURL, groupID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// PendingEnrollmentRow is the minimal shape returned by GET /api/v1/pending-enrollments.
type PendingEnrollmentRow struct {
	RequestID           string          `json:"requestId"`
	ProfileID           string          `json:"profileId"`
	Status              string          `json:"status"`
	SourceIP            string          `json:"sourceIp"`
	AgentVersion        string          `json:"agentVersion"`
	HardwareID          string          `json:"hardwareId"`
	DeniedReason        string          `json:"deniedReason"`
	MetadataJSON        json.RawMessage `json:"metadata"`
	CapabilitiesJSON    json.RawMessage `json:"capabilities"`
	ExpiresAt           *time.Time      `json:"expiresAt"`
	ApprovalAvailableAt *time.Time      `json:"approvalAvailableAt"`
	CreatedAt           time.Time       `json:"createdAt"`
}

// pendingEnrollmentListResponse handles both array and wrapped-items responses.
type pendingEnrollmentListResponse struct {
	Items []PendingEnrollmentRow `json:"items"`
}

// FetchPendingEnrollments retrieves pending enrollments from the regional plane.
func (c *regionalClient) FetchPendingEnrollments(ctx context.Context) ([]PendingEnrollmentRow, error) {
	var all []PendingEnrollmentRow
	offset := 0
	limit := 500
	for {
		url := fmt.Sprintf("%s/api/v1/pending-enrollments?status=pending&limit=%d&offset=%d", c.baseURL, limit, offset)
		// The regional plane returns {"items": [...]}
		var page pendingEnrollmentListResponse
		if err := c.getJSON(ctx, url, &page); err != nil {
			return nil, fmt.Errorf("fetch pending enrollments: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < limit {
			break
		}
		offset += limit
	}
	return all, nil
}

// ApprovePendingEnrollment proxies an approve call to the regional plane.
func (c *regionalClient) ApprovePendingEnrollment(ctx context.Context, requestID string) error {
	url := fmt.Sprintf("%s/api/v1/pending-enrollments/%s/approve", c.baseURL, requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// DenyPendingEnrollment proxies a deny call to the regional plane.
func (c *regionalClient) DenyPendingEnrollment(ctx context.Context, requestID, reason string) error {
	body, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/v1/pending-enrollments/%s/deny", c.baseURL, requestID)
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
