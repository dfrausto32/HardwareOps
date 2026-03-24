package artifactingest

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// mockS3Client implements s3ObjectGetter for tests.
type mockS3Client struct {
	output *s3.GetObjectOutput
	err    error
	// lastInput records the last GetObjectInput received.
	lastInput *s3.GetObjectInput
}

func (m *mockS3Client) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.lastInput = params
	return m.output, m.err
}

func mockS3Adapter(mock *mockS3Client) *S3PullAdapter {
	a := NewS3PullAdapter(0)
	a.newClient = func(_ context.Context, _ Credentials) (s3ObjectGetter, error) {
		return mock, nil
	}
	return a
}

// --- parseS3URI ---

func TestParseS3URI(t *testing.T) {
	tests := []struct {
		uri        string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{"s3://my-bucket/my-key.tar.gz", "my-bucket", "my-key.tar.gz", false},
		{"s3://my-bucket/path/to/artifact.tar.gz", "my-bucket", "path/to/artifact.tar.gz", false},
		{"", "", "", true},
		{"s3://", "", "", true},
		{"s3://bucket-only", "", "", true},
		{"s3://bucket/", "", "", true},
		{"https://bucket/key", "", "", true},
	}
	for _, tt := range tests {
		bucket, key, err := parseS3URI(tt.uri)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseS3URI(%q) expected error, got nil", tt.uri)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseS3URI(%q) unexpected error: %v", tt.uri, err)
			continue
		}
		if bucket != tt.wantBucket || key != tt.wantKey {
			t.Errorf("parseS3URI(%q) = (%q, %q), want (%q, %q)", tt.uri, bucket, key, tt.wantBucket, tt.wantKey)
		}
	}
}

// --- S3PullAdapter.Kind ---

func TestS3PullAdapterKind(t *testing.T) {
	if k := NewS3PullAdapter(0).Kind(); k != "s3" {
		t.Fatalf("expected kind s3, got %q", k)
	}
}

// --- S3PullAdapter.Pull success ---

func TestS3PullAdapterSuccess(t *testing.T) {
	body := "artifact bytes"
	mock := &mockS3Client{
		output: &s3.GetObjectOutput{
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: aws.Int64(int64(len(body))),
			ContentType:   aws.String("application/gzip"),
		},
	}
	adapter := mockS3Adapter(mock)
	resp, err := adapter.Pull(context.Background(), PullRequest{
		URI:         "s3://my-bucket/artifacts/app.tar.gz",
		Credentials: Credentials{Values: map[string]string{"region": "us-west-2"}},
	})
	if err != nil {
		t.Fatalf("Pull error: %v", err)
	}
	defer resp.Body.Close()

	if resp.ContentLength != int64(len(body)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(body))
	}
	if resp.ContentType != "application/gzip" {
		t.Errorf("ContentType = %q, want application/gzip", resp.ContentType)
	}
	// Verify the right bucket and key were requested.
	if mock.lastInput == nil {
		t.Fatal("GetObject was not called")
	}
	if aws.ToString(mock.lastInput.Bucket) != "my-bucket" {
		t.Errorf("Bucket = %q, want my-bucket", aws.ToString(mock.lastInput.Bucket))
	}
	if aws.ToString(mock.lastInput.Key) != "artifacts/app.tar.gz" {
		t.Errorf("Key = %q, want artifacts/app.tar.gz", aws.ToString(mock.lastInput.Key))
	}
}

// --- S3PullAdapter.Pull — S3 error propagation ---

func TestS3PullAdapterS3Error(t *testing.T) {
	mock := &mockS3Client{err: errors.New("NoSuchKey")}
	adapter := mockS3Adapter(mock)
	_, err := adapter.Pull(context.Background(), PullRequest{URI: "s3://my-bucket/missing.tar.gz"})
	if !errors.Is(err, ErrSourceRequest) {
		t.Fatalf("expected ErrSourceRequest, got %v", err)
	}
}

// --- S3PullAdapter.Pull — invalid URI ---

func TestS3PullAdapterInvalidURI(t *testing.T) {
	adapter := mockS3Adapter(&mockS3Client{})
	_, err := adapter.Pull(context.Background(), PullRequest{URI: "https://not-s3/key"})
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource for non-s3 URI, got %v", err)
	}
}

func TestS3PullAdapterMissingURI(t *testing.T) {
	adapter := mockS3Adapter(&mockS3Client{})
	_, err := adapter.Pull(context.Background(), PullRequest{URI: ""})
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource for empty URI, got %v", err)
	}
}

// --- S3PullAdapter.Pull — static credentials are passed through ---

func TestS3PullAdapterStaticCredentials(t *testing.T) {
	var capturedCreds Credentials
	mock := &mockS3Client{
		output: &s3.GetObjectOutput{
			Body: io.NopCloser(strings.NewReader("")),
		},
	}
	adapter := NewS3PullAdapter(0)
	adapter.newClient = func(_ context.Context, creds Credentials) (s3ObjectGetter, error) {
		capturedCreds = creds
		return mock, nil
	}

	creds := Credentials{Values: map[string]string{
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"region":            "eu-central-1",
	}}
	resp, err := adapter.Pull(context.Background(), PullRequest{
		URI:         "s3://prod-bucket/firmware-v2.tar.gz",
		Credentials: creds,
	})
	if err != nil {
		t.Fatalf("Pull error: %v", err)
	}
	defer resp.Body.Close()

	if capturedCreds.Get("access_key_id") != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("credentials not forwarded to client factory")
	}
}

// --- S3PullAdapter.Pull — client factory error ---

func TestS3PullAdapterClientBuildError(t *testing.T) {
	adapter := NewS3PullAdapter(0)
	adapter.newClient = func(_ context.Context, _ Credentials) (s3ObjectGetter, error) {
		return nil, errors.New("no AWS credentials found")
	}
	_, err := adapter.Pull(context.Background(), PullRequest{URI: "s3://my-bucket/key"})
	if !errors.Is(err, ErrSourceRequest) {
		t.Fatalf("expected ErrSourceRequest on client build failure, got %v", err)
	}
}
