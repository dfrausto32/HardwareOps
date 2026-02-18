package artifactingest

import (
	"errors"
	"testing"
)

func TestResolvePullSource_BackwardCompatible(t *testing.T) {
	spec, err := ResolvePullSource(nil, "https://example.com/a.tar.gz")
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if spec.Kind != "http" {
		t.Fatalf("expected http kind, got %q", spec.Kind)
	}
	if spec.URI != "https://example.com/a.tar.gz" {
		t.Fatalf("unexpected uri: %q", spec.URI)
	}
}

func TestResolvePullSource_SourceObject(t *testing.T) {
	spec, err := ResolvePullSource(&PullSource{
		Kind:          "http",
		URI:           "https://example.com/a.tar.gz",
		CredentialRef: "cred-1",
	}, "")
	if err != nil {
		t.Fatalf("resolve source object: %v", err)
	}
	if spec.CredentialRef != "cred-1" {
		t.Fatalf("expected credential ref")
	}
}

func TestResolvePullSource_Mismatch(t *testing.T) {
	_, err := ResolvePullSource(&PullSource{
		Kind: "http",
		URI:  "https://example.com/a.tar.gz",
	}, "https://example.com/b.tar.gz")
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
}
