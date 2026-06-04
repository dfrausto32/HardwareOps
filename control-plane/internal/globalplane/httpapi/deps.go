package httpapi

import (
	"context"
	"io"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/globalplane"
	"github.com/parcel/control-plane/internal/globalplane/sync"
)

// ObjectStore is the subset of MinIO operations used by the global-plane.
type ObjectStore interface {
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	EnsureBucket(ctx context.Context, bucket string) error
}

// Dependencies holds all runtime dependencies for the global-plane HTTP layer.
type Dependencies struct {
	Store              globalplane.Store
	SyncManager        *sync.Manager
	Auth               *auth.Manager
	LoginBackoff       *auth.LoginBackoff
	OIDCProvider       *auth.OIDCProvider
	// OIDCLoginURL is the URL operators navigate to for SSO sign-in.
	// Non-empty only when OIDCProvider is configured.
	OIDCLoginURL       string
	// PostLoginURL is where the OIDC callback sends the browser after auth.
	PostLoginURL       string
	TokenEncryptionKey []byte
	CORSAllowedOrigins []string
	TrustProxy         bool
	// Artifact federation
	ObjectStore    ObjectStore
	S3Bucket       string
	PresignExpires time.Duration
	// PublicBaseURL is the externally-reachable base URL of this global-plane,
	// used as the globalPresignBaseUrl sent in federation metadata pushes.
	PublicBaseURL string
}
