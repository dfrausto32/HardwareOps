package httpapi

import (
	"context"
	"io"
	"time"

	"github.com/hardwareops/control-plane/internal/artifactingest"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/backup"
	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/lifecycle"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/upgrade"
)

type CertSigner interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) (certPEM []byte, fingerprint string, err error)
	CACertPEM() []byte
}

type ObjectStore interface {
	PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error)
	PresignPut(ctx context.Context, bucket, key string, expires time.Duration, contentType string) (string, error)
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) (int64, error)
	EnsureBucket(ctx context.Context, bucket string) error
	DeleteObject(ctx context.Context, bucket, key string) error
	StatObject(ctx context.Context, bucket, key string) (int64, error)
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

type RateLimitConfig struct {
	EnrollmentTokenRPM int
	EnrollRPM          int
	CheckinRPM         int
	ApplyResultRPM     int
}

type Dependencies struct {
	Store                          store.Store
	Signer                         CertSigner
	ObjectStore                    ObjectStore
	S3Bucket                       string
	PresignExpires                 time.Duration
	TrustProxy                     bool
	TrustedProxyCIDRs              []string
	ClientCertHeader               string
	RateLimits                     RateLimitConfig
	LogDir                         string
	Events                         *events.Hub
	CORSAllowedOrigins             []string
	Maintenance                    *MaintenanceState
	MaintenanceToken               string
	Upgrade                        *upgrade.Runner
	UpgradeUpdatesDir              string
	Backup                         *backup.Runner
	Restore                        *backup.Runner
	BackupDir                      string
	Metrics                        *metrics.Metrics
	MetricsPath                    string
	Auth                           *auth.Manager
	BootstrapToken                 string
	License                        *license.Manager
	CertManager                    *certs.Manager
	CertRotationGrace              time.Duration
	ArtifactLifecycle              *lifecycle.Manager
	ArtifactPullHosts              []string
	ArtifactPullMaxBytes           int64
	ArtifactPullTimeout            time.Duration
	ArtifactPullCreds              artifactingest.CredentialResolver
	DeviceIdentityMode             string
	DeviceIdentityRequireOnEnroll  bool
	DeviceIdentityRequireOnCheckin bool
}
