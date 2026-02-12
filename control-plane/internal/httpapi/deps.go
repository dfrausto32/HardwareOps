package httpapi

import (
	"context"
	"io"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/upgrade"
)

type CertSigner interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) (certPEM []byte, fingerprint string, err error)
	CACertPEM() []byte
}

type ObjectStore interface {
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	EnsureBucket(ctx context.Context, bucket string) error
	DeleteObject(ctx context.Context, bucket, key string) error
}

type RateLimitConfig struct {
	EnrollmentTokenRPM int
	EnrollRPM          int
	CheckinRPM         int
	ApplyResultRPM     int
}

type Dependencies struct {
	Store              store.Store
	Signer             CertSigner
	ObjectStore        ObjectStore
	S3Bucket           string
	PresignExpires     time.Duration
	TrustProxy         bool
	ClientCertHeader   string
	RateLimits         RateLimitConfig
	LogDir             string
	Events             *events.Hub
	CORSAllowedOrigins []string
	Maintenance        *MaintenanceState
	MaintenanceToken   string
	Upgrade            *upgrade.Runner
	UpgradeUpdatesDir  string
	Backup             *backup.Runner
	Restore            *backup.Runner
	BackupDir          string
	Auth               *auth.Manager
	License            *license.Manager
	CertManager        *certs.Manager
}
