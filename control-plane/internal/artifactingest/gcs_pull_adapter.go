package artifactingest

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	gcsScopeReadOnly = "https://www.googleapis.com/auth/devstorage.read_only"
	gcsXMLAPIBase    = "https://storage.googleapis.com"
)

// GCSPullAdapter fetches artifacts directly from Google Cloud Storage.
//
// URI format: gs://bucket-name/path/to/object
//             gcs://bucket-name/path/to/object  (alias accepted)
//
// Credential keys (in Credentials.Values):
//   - "oauth_token" — a pre-obtained OAuth2 bearer token (short-lived use / testing)
//   - "service_account_json" — Google service account JSON key (full file contents)
//   - Empty/no credential keys — uses Application Default Credentials
//     (Workload Identity on GKE, GCE instance service account, GOOGLE_APPLICATION_CREDENTIALS env var)
type GCSPullAdapter struct {
	timeout        time.Duration
	// newTokenSource is injectable for tests; nil means use the default credential logic.
	newTokenSource func(ctx context.Context, creds Credentials) (oauth2.TokenSource, error)
}

// NewGCSPullAdapter creates a GCS pull adapter with the given request timeout.
// A zero timeout defaults to 15 minutes.
func NewGCSPullAdapter(timeout time.Duration) *GCSPullAdapter {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &GCSPullAdapter{timeout: timeout}
}

func (a *GCSPullAdapter) Kind() string { return "gcs" }

func (a *GCSPullAdapter) Pull(ctx context.Context, req PullRequest) (PullResponse, error) {
	bucket, object, err := parseGCSURI(req.URI)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: %v", ErrInvalidSource, err)
	}

	ts, err := a.buildTokenSource(ctx, req.Credentials)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: build gcs credentials: %v", ErrSourceRequest, err)
	}

	// GCS XML API: GET https://storage.googleapis.com/{bucket}/{encoded-object}
	objectEncoded := url.PathEscape(object)
	downloadURL := gcsXMLAPIBase + "/" + bucket + "/" + objectEncoded

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: build gcs request: %v", ErrInvalidSource, err)
	}

	// Attach Authorization header from token source.
	token, err := ts.Token()
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: obtain gcs token: %v", ErrSourceRequest, err)
	}
	token.SetAuthHeader(httpReq)

	client := &http.Client{Timeout: a.timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: gcs request failed: %v", ErrSourceRequest, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return PullResponse{}, fmt.Errorf("%w: gcs returned %d", ErrSourceRequest, resp.StatusCode)
	}
	return PullResponse{
		Body:          resp.Body,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, nil
}

func (a *GCSPullAdapter) buildTokenSource(ctx context.Context, creds Credentials) (oauth2.TokenSource, error) {
	if a.newTokenSource != nil {
		return a.newTokenSource(ctx, creds)
	}
	return buildGCSTokenSource(ctx, creds)
}

// buildGCSTokenSource creates an OAuth2 token source from the resolved credentials.
func buildGCSTokenSource(ctx context.Context, creds Credentials) (oauth2.TokenSource, error) {
	// Static bearer token (useful for short-lived tokens or tests).
	if oauthToken := strings.TrimSpace(creds.Get("oauth_token")); oauthToken != "" {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: oauthToken}), nil
	}

	// Service account JSON key.
	if saJSON := strings.TrimSpace(creds.Get("service_account_json")); saJSON != "" {
		gcsCreds, err := google.CredentialsFromJSON(ctx, []byte(saJSON), gcsScopeReadOnly)
		if err != nil {
			return nil, fmt.Errorf("parse service_account_json: %w", err)
		}
		return gcsCreds.TokenSource, nil
	}

	// Application Default Credentials (GKE Workload Identity / GCE instance / env var).
	gcsCreds, err := google.FindDefaultCredentials(ctx, gcsScopeReadOnly)
	if err != nil {
		return nil, fmt.Errorf("application default credentials: %w", err)
	}
	return gcsCreds.TokenSource, nil
}

// parseGCSURI parses a gs:// or gcs:// URI into bucket and object path.
// Supported formats:
//   - gs://bucket/object
//   - gcs://bucket/path/to/object
func parseGCSURI(rawURI string) (bucket, object string, err error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return "", "", fmt.Errorf("uri required")
	}

	switch {
	case strings.HasPrefix(rawURI, "gs://"):
		rawURI = strings.TrimPrefix(rawURI, "gs://")
	case strings.HasPrefix(rawURI, "gcs://"):
		rawURI = strings.TrimPrefix(rawURI, "gcs://")
	default:
		return "", "", fmt.Errorf("invalid gcs uri: scheme must be gs:// or gcs://")
	}

	if rawURI == "" || strings.HasPrefix(rawURI, "/") {
		return "", "", fmt.Errorf("invalid gcs uri: bucket name missing")
	}
	idx := strings.IndexByte(rawURI, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid gcs uri: object path missing (format: gs://bucket/object)")
	}
	bucket = rawURI[:idx]
	object = rawURI[idx+1:]
	if bucket == "" {
		return "", "", fmt.Errorf("invalid gcs uri: empty bucket")
	}
	if object == "" {
		return "", "", fmt.Errorf("invalid gcs uri: empty object path")
	}
	return bucket, object, nil
}
