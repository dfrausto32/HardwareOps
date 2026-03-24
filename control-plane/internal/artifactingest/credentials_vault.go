package artifactingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LoadStaticCredentialsFromVault fetches pull credentials from a HashiCorp Vault
// KV v2 secret. The path must be the full KV v2 data path
// (e.g. "secret/data/hardwareops/pull-creds"). The secret value must be a
// JSON object with the same shape as the static credentials format:
//
//	{ "<ref>": { "username": "...", "password": "..." }, ... }
func LoadStaticCredentialsFromVault(ctx context.Context, addr, token, path string) (map[string]map[string]string, error) {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	token = strings.TrimSpace(token)
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if addr == "" || token == "" || path == "" {
		return nil, nil
	}

	url := addr + "/v1/" + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault: request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vault: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// KV v2 response shape: { "data": { "data": { ... } } }
	var envelope struct {
		Data struct {
			Data json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("vault: parse response: %w", err)
	}
	if len(envelope.Data.Data) == 0 {
		return nil, fmt.Errorf("vault: secret at %q has empty data", path)
	}

	var creds map[string]map[string]string
	if err := json.Unmarshal(envelope.Data.Data, &creds); err != nil {
		return nil, fmt.Errorf("vault: parse credentials data: %w", err)
	}
	return normalizeCredentialMap(creds), nil
}
