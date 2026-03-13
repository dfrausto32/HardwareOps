package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hardwareops/agent/internal/state"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type RateLimitError struct {
	RetryAfter time.Duration
	Status     int
}

type StatusError struct {
	StatusCode int
	Body       string
}

func (e RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited: retry after %s", e.RetryAfter)
	}
	return fmt.Sprintf("rate limited: status=%d", e.Status)
}

func (e StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("request failed: status=%d", e.StatusCode)
	}
	return fmt.Sprintf("request failed: status=%d body=%s", e.StatusCode, e.Body)
}

func New(baseURL string) *Client {
	return NewWithTLS(baseURL, nil)
}

func NewWithTLS(baseURL string, tlsConfig *tls.Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) HTTPClient() *http.Client {
	return c.http
}

type CheckinRequest struct {
	DeviceID          string                      `json:"deviceId"`
	AgentVersion      string                      `json:"agentVersion"`
	Current           CheckinCurrent              `json:"current,omitempty"`
	CurrentComponents map[string]CheckinComponent `json:"currentComponents,omitempty"`
	Capabilities      any                         `json:"capabilities,omitempty"`
	Labels            map[string]string           `json:"labels,omitempty"`
}

type CheckinCurrent struct {
	SoftwareVersion     string     `json:"softwareVersion,omitempty"`
	ConfigRev           string     `json:"configRev,omitempty"`
	LastApplyStatus     string     `json:"lastApplyStatus,omitempty"`
	LastApplyError      string     `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string     `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string     `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string     `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time `json:"lastPreApplyAt,omitempty"`
}

type CheckinComponent struct {
	SoftwareVersion     string     `json:"softwareVersion,omitempty"`
	ConfigRev           string     `json:"configRev,omitempty"`
	LastApplyStatus     string     `json:"lastApplyStatus,omitempty"`
	LastApplyError      string     `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string     `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string     `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string     `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time `json:"lastPreApplyAt,omitempty"`
}

type DesiredState struct {
	ArtifactID      string                      `json:"artifactId"`
	SoftwareVersion string                      `json:"softwareVersion"`
	ConfigRev       string                      `json:"configRev"`
	DownloadURL     string                      `json:"downloadUrl"`
	ApplyPolicy     json.RawMessage             `json:"applyPolicy"`
	CheckinInterval int                         `json:"checkinIntervalSec"`
	Source          string                      `json:"source,omitempty"`
	Components      map[string]DesiredComponent `json:"components,omitempty"`
}

type DesiredComponent struct {
	ArtifactID      string          `json:"artifactId"`
	SoftwareVersion string          `json:"softwareVersion"`
	ConfigRev       string          `json:"configRev"`
	DownloadURL     string          `json:"downloadUrl"`
	ApplyPolicy     json.RawMessage `json:"applyPolicy"`
	Source          string          `json:"source,omitempty"`
}

type CheckinResponse struct {
	Desired        *DesiredState       `json:"desired"`
	PendingActions []Action            `json:"pendingActions,omitempty"`
	ServerTime     time.Time           `json:"serverTime"`
	SigningTrust   *SigningTrustBundle `json:"signingTrust,omitempty"`
}

type Action struct {
	ActionID string          `json:"actionId"`
	Type     string          `json:"type"`
	Params   json.RawMessage `json:"params,omitempty"`
}

type ArtifactResponse struct {
	ArtifactID         string          `json:"artifactId"`
	Name               string          `json:"name"`
	Version            string          `json:"version"`
	Type               string          `json:"type"`
	ObjectKey          string          `json:"objectKey"`
	SHA256             string          `json:"sha256"`
	Signature          string          `json:"signature"`
	SignatureType      string          `json:"signatureType"`
	SignatureKeyID     string          `json:"signatureKeyId"`
	VerificationStatus string          `json:"verificationStatus"`
	VerificationError  string          `json:"verificationError"`
	SizeBytes          int64           `json:"sizeBytes"`
	Metadata           json.RawMessage `json:"metadata"`
}

type ReenrollResponse struct {
	DeviceID  string `json:"deviceId"`
	CertPEM   string `json:"certPem"`
	CACertPEM string `json:"caCertPem"`
}

type SigningTrustKey struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPem"`
}

type SigningTrustBundle struct {
	UpdatedAt time.Time         `json:"updatedAt,omitempty"`
	Keys      []SigningTrustKey `json:"keys"`
}

type PendingEnrollmentRequest struct {
	ProfileToken string `json:"profileToken"`
	CSR          string `json:"csr"`
	AgentVersion string `json:"agentVersion,omitempty"`
	Capabilities any    `json:"capabilities,omitempty"`
	Metadata     any    `json:"metadata,omitempty"`
}

type PendingEnrollmentRequestResponse struct {
	RequestID    string    `json:"requestId"`
	Status       string    `json:"status"`
	ClaimToken   string    `json:"claimToken"`
	PollAfterSec int       `json:"pollAfterSec"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type ClaimPendingEnrollmentRequest struct {
	RequestID  string `json:"requestId"`
	ClaimToken string `json:"claimToken"`
}

type ClaimPendingEnrollmentResponse struct {
	Status       string              `json:"status"`
	PollAfterSec int                 `json:"pollAfterSec,omitempty"`
	Reason       string              `json:"reason,omitempty"`
	DeviceID     string              `json:"deviceId,omitempty"`
	CertPEM      string              `json:"certPem,omitempty"`
	CACertPEM    string              `json:"caCertPem,omitempty"`
	ExpiresAt    time.Time           `json:"expiresAt,omitempty"`
	SigningTrust *SigningTrustBundle `json:"signingTrust,omitempty"`
}

func (c *Client) Reenroll(csrPEM []byte) (ReenrollResponse, error) {
	payload := map[string]string{"csr": string(csrPEM)}
	body, err := json.Marshal(payload)
	if err != nil {
		return ReenrollResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v1/devices/reenroll", c.baseURL)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return ReenrollResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReenrollResponse{}, fmt.Errorf("reenroll failed: status=%d", resp.StatusCode)
	}
	var out ReenrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ReenrollResponse{}, err
	}
	if out.CertPEM == "" {
		return ReenrollResponse{}, fmt.Errorf("reenroll failed: empty cert")
	}
	return out, nil
}

func (c *Client) RequestPendingEnrollment(req PendingEnrollmentRequest) (PendingEnrollmentRequestResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return PendingEnrollmentRequestResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v1/pending-enrollments/request", c.baseURL)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return PendingEnrollmentRequestResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return PendingEnrollmentRequestResponse{}, readStatusError(resp)
	}
	var out PendingEnrollmentRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PendingEnrollmentRequestResponse{}, err
	}
	return out, nil
}

func (c *Client) ClaimPendingEnrollment(req ClaimPendingEnrollmentRequest) (ClaimPendingEnrollmentResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ClaimPendingEnrollmentResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v1/pending-enrollments/claim", c.baseURL)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return ClaimPendingEnrollmentResponse{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusGone:
		var out ClaimPendingEnrollmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return ClaimPendingEnrollmentResponse{}, err
		}
		return out, nil
	case http.StatusConflict:
		var out ClaimPendingEnrollmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err == nil && out.Status != "" {
			return out, nil
		}
		return ClaimPendingEnrollmentResponse{}, readStatusError(resp)
	default:
		return ClaimPendingEnrollmentResponse{}, readStatusError(resp)
	}
}

type PresignResponse struct {
	DownloadURL string `json:"downloadUrl"`
}

type ApplyResultRequest struct {
	Status           string `json:"status"`
	ArtifactID       string `json:"artifactId,omitempty"`
	Component        string `json:"component,omitempty"`
	AppliedVersion   string `json:"appliedVersion,omitempty"`
	AppliedConfigRev string `json:"appliedConfigRev,omitempty"`
	Error            string `json:"error,omitempty"`
	PreApplyStatus   string `json:"preApplyStatus,omitempty"`
	PreApplyError    string `json:"preApplyError,omitempty"`
}

func (c *Client) CheckIn(st state.State, capabilities any) (*CheckinResponse, error) {
	payload := CheckinRequest{
		DeviceID:     st.DeviceID,
		AgentVersion: st.AgentVersion,
		Capabilities: capabilities,
		Current: CheckinCurrent{
			SoftwareVersion:     st.CurrentVersion,
			ConfigRev:           st.CurrentConfigRev,
			LastApplyStatus:     st.LastApplyStatus,
			LastApplyError:      st.LastApplyError,
			LastApplyAt:         timePtr(st.LastApplyAt),
			LastApplyArtifactID: st.LastApplyArtifactID,
			LastPreApplyStatus:  st.LastPreApplyStatus,
			LastPreApplyError:   st.LastPreApplyError,
			LastPreApplyAt:      timePtr(st.LastPreApplyAt),
		},
	}
	if len(st.Components) > 0 {
		payload.CurrentComponents = map[string]CheckinComponent{}
		for key, comp := range st.Components {
			payload.CurrentComponents[key] = CheckinComponent{
				SoftwareVersion:     comp.CurrentVersion,
				ConfigRev:           comp.CurrentConfigRev,
				LastApplyStatus:     comp.LastApplyStatus,
				LastApplyError:      comp.LastApplyError,
				LastApplyAt:         timePtr(comp.LastApplyAt),
				LastApplyArtifactID: comp.LastApplyArtifactID,
				LastPreApplyStatus:  comp.LastPreApplyStatus,
				LastPreApplyError:   comp.LastPreApplyError,
				LastPreApplyAt:      timePtr(comp.LastPreApplyAt),
			}
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/devices/checkin", c.baseURL)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, RateLimitError{RetryAfter: parseRetryAfter(resp), Status: resp.StatusCode}
		}
		return nil, fmt.Errorf("check-in failed: status=%d", resp.StatusCode)
	}

	var out CheckinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func readStatusError(resp *http.Response) error {
	if resp == nil {
		return StatusError{}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return StatusError{
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (c *Client) PostApplyResult(deviceID string, req ApplyResultRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/v1/devices/%s/apply-result", c.baseURL, deviceID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apply result failed: status=%d", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetArtifact(artifactID string) (*ArtifactResponse, error) {
	url := fmt.Sprintf("%s/api/v1/devices/artifacts/%s", c.baseURL, artifactID)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get artifact failed: status=%d", resp.StatusCode)
	}

	var out ArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PresignArtifact(artifactID string) (*PresignResponse, error) {
	url := fmt.Sprintf("%s/api/v1/devices/artifacts/%s/presign", c.baseURL, artifactID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("presign failed: status=%d", resp.StatusCode)
	}

	var out PresignResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
