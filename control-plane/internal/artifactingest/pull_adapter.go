package artifactingest

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
)

var (
	ErrSourceNotAllowed   = errors.New("source not allowed")
	ErrSourceRequest      = errors.New("source request failed")
	ErrUnsupportedAdapter = errors.New("unsupported source kind")
	ErrInvalidSource      = errors.New("invalid source")
)

type PullRequest struct {
	URI           string
	CredentialRef string
	Credentials   Credentials
}

type PullResponse struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

type PullAdapter interface {
	Kind() string
	Pull(ctx context.Context, req PullRequest) (PullResponse, error)
}

type PullAdapterRegistry struct {
	adapters map[string]PullAdapter
}

func NewPullAdapterRegistry(adapters ...PullAdapter) *PullAdapterRegistry {
	r := &PullAdapterRegistry{adapters: map[string]PullAdapter{}}
	for _, adapter := range adapters {
		r.Register(adapter)
	}
	return r
}

func (r *PullAdapterRegistry) Register(adapter PullAdapter) {
	if r == nil || adapter == nil {
		return
	}
	kind := strings.TrimSpace(strings.ToLower(adapter.Kind()))
	if kind == "" {
		return
	}
	if r.adapters == nil {
		r.adapters = map[string]PullAdapter{}
	}
	r.adapters[kind] = adapter
}

func (r *PullAdapterRegistry) Get(kind string) (PullAdapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.adapters[strings.TrimSpace(strings.ToLower(kind))]
	return adapter, ok
}

func (r *PullAdapterRegistry) Kinds() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.adapters))
	for kind := range r.adapters {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

type PullSource struct {
	Kind          string
	URI           string
	CredentialRef string
}

func ResolvePullSource(source *PullSource, sourceURL string) (PullSource, error) {
	urlField := strings.TrimSpace(sourceURL)
	if source == nil {
		if urlField == "" {
			return PullSource{}, ErrInvalidSource
		}
		return PullSource{
			Kind: "http",
			URI:  urlField,
		}, nil
	}

	kind := strings.TrimSpace(strings.ToLower(source.Kind))
	uri := strings.TrimSpace(source.URI)
	credRef := strings.TrimSpace(source.CredentialRef)
	hasSource := kind != "" || uri != "" || credRef != ""
	if !hasSource {
		if urlField == "" {
			return PullSource{}, ErrInvalidSource
		}
		return PullSource{
			Kind: "http",
			URI:  urlField,
		}, nil
	}

	if uri == "" {
		if urlField == "" {
			return PullSource{}, ErrInvalidSource
		}
		uri = urlField
	}
	if urlField != "" && urlField != uri {
		return PullSource{}, ErrInvalidSource
	}
	if kind == "" {
		kind = "http"
	}

	return PullSource{
		Kind:          kind,
		URI:           uri,
		CredentialRef: credRef,
	}, nil
}
