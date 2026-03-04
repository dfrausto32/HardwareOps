package artifactingest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPPullAdapter struct {
	allowedHosts []string
	allowHTTP    bool
	client       *http.Client
}

func NewHTTPPullAdapter(allowedHosts []string, timeout time.Duration, allowInsecureHTTP bool) *HTTPPullAdapter {
	normalizedHosts := make([]string, 0, len(allowedHosts))
	for _, host := range allowedHosts {
		host = strings.TrimSpace(strings.ToLower(host))
		if host == "" {
			continue
		}
		normalizedHosts = append(normalizedHosts, host)
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &HTTPPullAdapter{
		allowedHosts: normalizedHosts,
		allowHTTP:    allowInsecureHTTP,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (a *HTTPPullAdapter) Kind() string {
	return "http"
}

func (a *HTTPPullAdapter) Pull(ctx context.Context, req PullRequest) (PullResponse, error) {
	rawURI := strings.TrimSpace(req.URI)
	if rawURI == "" {
		return PullResponse{}, fmt.Errorf("%w: uri required", ErrInvalidSource)
	}
	parsedURL, err := url.Parse(rawURI)
	if err != nil || parsedURL == nil {
		return PullResponse{}, fmt.Errorf("%w: invalid uri", ErrInvalidSource)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return PullResponse{}, fmt.Errorf("%w: scheme must be http/https", ErrInvalidSource)
	}
	if parsedURL.Scheme == "http" && !a.allowHTTP {
		return PullResponse{}, fmt.Errorf("%w: insecure http scheme disabled", ErrInvalidSource)
	}
	allowedHost := hostAllowed(parsedURL.Hostname(), a.allowedHosts)
	if isPrivateOrLoopbackHost(parsedURL.Hostname()) && !allowedHost {
		return PullResponse{}, fmt.Errorf("%w: host %q", ErrSourceNotAllowed, parsedURL.Hostname())
	}
	if len(a.allowedHosts) > 0 && !allowedHost {
		return PullResponse{}, fmt.Errorf("%w: host %q", ErrSourceNotAllowed, parsedURL.Hostname())
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURI, nil)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: invalid request", ErrInvalidSource)
	}
	applyRequestCredentials(httpReq, req.Credentials, false)
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: %v", ErrSourceRequest, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return PullResponse{}, fmt.Errorf("%w: source returned %d", ErrSourceRequest, resp.StatusCode)
	}
	return PullResponse{
		Body:          resp.Body,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, nil
}

func hostAllowed(host string, allowedHosts []string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "" {
		return false
	}
	for _, allow := range allowedHosts {
		allow = strings.TrimSpace(strings.ToLower(allow))
		if allow == "" {
			continue
		}
		if normalized == allow {
			return true
		}
		if strings.HasPrefix(allow, ".") && strings.HasSuffix(normalized, allow) {
			return true
		}
	}
	return false
}

func isPrivateOrLoopbackHost(host string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	if normalized == "" {
		return false
	}
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") {
		return true
	}
	ip := net.ParseIP(normalized)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	return false
}
