package artifactingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const s3DefaultRegion = "us-east-1"

// s3ObjectGetter is a minimal interface for S3 GetObject, used for testing.
type s3ObjectGetter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3PullAdapter fetches artifacts directly from Amazon S3.
//
// URI format: s3://bucket-name/path/to/object
//
// Credential keys (in Credentials.Values):
//   - "access_key_id" + "secret_access_key" [+ "session_token"] — static AWS credentials
//   - "region" — AWS region (default: us-east-1 or AWS_DEFAULT_REGION env)
//   - Empty/no credential keys — uses the AWS SDK default credential chain
//     (IAM role, IRSA, ECS task role, environment variables, ~/.aws/credentials)
type S3PullAdapter struct {
	timeout   time.Duration
	// newClient is injectable for tests; nil means use the default SDK factory.
	newClient func(ctx context.Context, creds Credentials) (s3ObjectGetter, error)
}

// NewS3PullAdapter creates an S3 pull adapter with the given request timeout.
// A zero timeout defaults to 15 minutes.
func NewS3PullAdapter(timeout time.Duration) *S3PullAdapter {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &S3PullAdapter{timeout: timeout}
}

func (a *S3PullAdapter) Kind() string { return "s3" }

func (a *S3PullAdapter) Pull(ctx context.Context, req PullRequest) (PullResponse, error) {
	bucket, key, err := parseS3URI(req.URI)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: %v", ErrInvalidSource, err)
	}

	client, err := a.buildClient(ctx, req.Credentials)
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: build s3 client: %v", ErrSourceRequest, err)
	}

	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return PullResponse{}, fmt.Errorf("%w: s3 GetObject: %v", ErrSourceRequest, err)
	}
	if out == nil {
		return PullResponse{}, fmt.Errorf("%w: s3 GetObject returned nil output", ErrSourceRequest)
	}

	resp := PullResponse{Body: out.Body}
	if out.ContentLength != nil {
		resp.ContentLength = *out.ContentLength
	}
	if out.ContentType != nil {
		resp.ContentType = *out.ContentType
	}
	return resp, nil
}

func (a *S3PullAdapter) buildClient(ctx context.Context, creds Credentials) (s3ObjectGetter, error) {
	if a.newClient != nil {
		return a.newClient(ctx, creds)
	}
	return buildS3Client(ctx, creds)
}

// buildS3Client constructs a real AWS S3 client from the provided credentials.
func buildS3Client(ctx context.Context, creds Credentials) (*s3.Client, error) {
	region := strings.TrimSpace(creds.Get("region"))
	if region == "" {
		region = s3DefaultRegion
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	accessKeyID := strings.TrimSpace(creds.Get("access_key_id"))
	secretKey := strings.TrimSpace(creds.Get("secret_access_key"))
	sessionToken := strings.TrimSpace(creds.Get("session_token"))

	if accessKeyID != "" && secretKey != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, sessionToken),
		))
	}
	// When no static creds are provided the SDK uses its default chain:
	// env vars → ~/.aws/credentials → IAM role (ECS task role / EC2 instance profile / IRSA).

	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

// parseS3URI parses an s3:// URI into bucket and key parts.
// Supported formats:
//   - s3://bucket/key
//   - s3://bucket/path/to/key
func parseS3URI(rawURI string) (bucket, key string, err error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return "", "", fmt.Errorf("uri required")
	}
	if !strings.HasPrefix(rawURI, "s3://") {
		return "", "", fmt.Errorf("invalid s3 uri: scheme must be s3://")
	}
	rawURI = strings.TrimPrefix(rawURI, "s3://")
	if rawURI == "" || strings.HasPrefix(rawURI, "/") {
		return "", "", fmt.Errorf("invalid s3 uri: bucket name missing")
	}
	idx := strings.IndexByte(rawURI, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid s3 uri: key missing (format: s3://bucket/key)")
	}
	bucket = rawURI[:idx]
	key = rawURI[idx+1:]
	if bucket == "" {
		return "", "", fmt.Errorf("invalid s3 uri: empty bucket")
	}
	if key == "" {
		return "", "", fmt.Errorf("invalid s3 uri: empty key")
	}
	return bucket, key, nil
}
