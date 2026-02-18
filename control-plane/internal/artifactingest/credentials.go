package artifactingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrCredentialResolverUnavailable = errors.New("credential resolver unavailable")
	ErrCredentialNotFound            = errors.New("credential ref not found")
)

type Credentials struct {
	Ref    string
	Values map[string]string
}

func (c Credentials) Get(key string) string {
	return c.Values[strings.ToLower(strings.TrimSpace(key))]
}

type CredentialResolver interface {
	Resolve(ctx context.Context, ref string) (Credentials, error)
}

type NoopCredentialResolver struct{}

func (NoopCredentialResolver) Resolve(_ context.Context, _ string) (Credentials, error) {
	return Credentials{}, ErrCredentialResolverUnavailable
}

type StaticCredentialResolver struct {
	values map[string]map[string]string
}

func NewStaticCredentialResolver(values map[string]map[string]string) *StaticCredentialResolver {
	out := normalizeCredentialMap(values)
	return &StaticCredentialResolver{values: out}
}

func normalizeCredentialMap(values map[string]map[string]string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for ref, fields := range values {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		normalized := map[string]string{}
		for k, v := range fields {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			normalized[k] = v
		}
		out[ref] = normalized
	}
	return out
}

func (r *StaticCredentialResolver) Resolve(_ context.Context, ref string) (Credentials, error) {
	if r == nil {
		return Credentials{}, ErrCredentialResolverUnavailable
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Credentials{}, ErrCredentialNotFound
	}
	values, ok := r.values[ref]
	if !ok {
		return Credentials{}, ErrCredentialNotFound
	}
	copied := make(map[string]string, len(values))
	for k, v := range values {
		copied[k] = v
	}
	return Credentials{
		Ref:    ref,
		Values: copied,
	}, nil
}

func LoadStaticCredentials(filePath string, inlineJSON string) (map[string]map[string]string, error) {
	filePath = strings.TrimSpace(filePath)
	inlineJSON = strings.TrimSpace(inlineJSON)
	raw := inlineJSON
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read credentials file: %w", err)
		}
		raw = strings.TrimSpace(string(data))
	}
	if raw == "" {
		return nil, nil
	}
	return parseStaticCredentialsRaw(raw)
}

func parseStaticCredentialsRaw(raw string) (map[string]map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var parsed map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse credentials json: %w", err)
	}
	return normalizeCredentialMap(parsed), nil
}

func MergeStaticCredentialSets(sets ...map[string]map[string]string) map[string]map[string]string {
	merged := map[string]map[string]string{}
	for _, set := range sets {
		for ref, values := range normalizeCredentialMap(set) {
			copied := make(map[string]string, len(values))
			for k, v := range values {
				copied[k] = v
			}
			merged[ref] = copied
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
