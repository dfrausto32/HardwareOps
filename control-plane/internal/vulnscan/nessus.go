package vulnscan

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NessusClient is a minimal HTTP client for the Tenable/Nessus Pro API.
// Auth uses X-ApiKeys header — no external SDK.
type NessusClient struct {
	baseURL   string
	accessKey string
	secretKey string
	client    *http.Client
}

// NewNessusClient creates a NessusClient.
func NewNessusClient(baseURL, accessKey, secretKey string) *NessusClient {
	return &NessusClient{
		baseURL:   baseURL,
		accessKey: accessKey,
		secretKey: secretKey,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// NessusScan is one entry from GET /scans.
type NessusScan struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// NessusHost is one host entry from GET /scans/{id}.
type NessusHost struct {
	HostID   int    `json:"host_id"`
	Hostname string `json:"hostname"`
}

// NessusVuln is one finding from GET /scans/{id}/hosts/{hostId}.
type NessusVuln struct {
	PluginID   int    `json:"plugin_id"`
	PluginName string `json:"plugin_name"`
	Severity   int    `json:"severity"` // 0=info,1=low,2=medium,3=high,4=critical
	Count      int    `json:"count"`
}

// ListScans returns all scans from GET /scans.
func (c *NessusClient) ListScans() ([]NessusScan, error) {
	var resp struct {
		Scans []NessusScan `json:"scans"`
	}
	if err := c.get("/scans", &resp); err != nil {
		return nil, err
	}
	return resp.Scans, nil
}

// GetScanHosts returns hosts from GET /scans/{id}.
func (c *NessusClient) GetScanHosts(scanID string) ([]NessusHost, error) {
	var resp struct {
		Hosts []NessusHost `json:"hosts"`
	}
	if err := c.get("/scans/"+scanID, &resp); err != nil {
		return nil, err
	}
	return resp.Hosts, nil
}

// GetHostFindings returns vulnerability findings from GET /scans/{id}/hosts/{hostId}.
func (c *NessusClient) GetHostFindings(scanID, hostID string) ([]NessusVuln, error) {
	var resp struct {
		Vulnerabilities []NessusVuln `json:"vulnerabilities"`
	}
	if err := c.get("/scans/"+scanID+"/hosts/"+hostID, &resp); err != nil {
		return nil, err
	}
	return resp.Vulnerabilities, nil
}

func (c *NessusClient) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("nessus: build request: %w", err)
	}
	req.Header.Set("X-ApiKeys", fmt.Sprintf("accessKey=%s;secretKey=%s", c.accessKey, c.secretKey))
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("nessus: %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("nessus: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("nessus: %s returned %d: %s", path, resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

// nessusVulnToFinding converts a NessusVuln to our common finding type.
func nessusVulnToFinding(v NessusVuln) VulnerabilityFinding {
	return VulnerabilityFinding{
		ID:       fmt.Sprintf("nessus-%d", v.PluginID),
		Severity: nessusIntToSeverity(v.Severity),
		Package:  v.PluginName,
	}
}

func nessusIntToSeverity(s int) string {
	switch s {
	case 4:
		return "critical"
	case 3:
		return "high"
	case 2:
		return "medium"
	case 1:
		return "low"
	default:
		return "unknown"
	}
}
