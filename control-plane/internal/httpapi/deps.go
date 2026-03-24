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
	"github.com/hardwareops/control-plane/internal/mailer"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/releaseautoupdate"
	"github.com/hardwareops/control-plane/internal/store"
	"github.com/hardwareops/control-plane/internal/upgrade"
	"github.com/hardwareops/control-plane/internal/vulnscan"
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
	AuthLoginRPM       int
}

type PendingEnrollmentGuardrailConfig struct {
	RequestRPMPerSource  int
	RequestRPMPerProfile int
	MaxActive            int
	MaxActivePerProfile  int
	MaxActivePerSource   int
}

type Dependencies struct {
	Store                           store.Store
	Signer                          CertSigner
	ObjectStore                     ObjectStore
	S3Bucket                        string
	PresignExpires                  time.Duration
	TrustProxy                      bool
	TrustedProxyCIDRs               []string
	ClientCertHeader                string
	RateLimits                      RateLimitConfig
	PendingEnrollmentGuardrails     PendingEnrollmentGuardrailConfig
	LogDir                          string
	Events                          *events.Hub
	CORSAllowedOrigins              []string
	Maintenance                     *MaintenanceState
	Upgrade                         *upgrade.Runner
	UpgradeUpdatesDir               string
	Backup                          *backup.Runner
	Restore                         *backup.Runner
	BackupDir                       string
	Metrics                         *metrics.Metrics
	MetricsPath                     string
	Auth                            *auth.Manager
	AuthLoginBackoff                *auth.LoginBackoff
	OIDCProvider                    *auth.OIDCProvider
	LDAPProvider                    *auth.LDAPProvider
	WorkloadIdentity                auth.WorkloadIdentityExchanger
	BootstrapToken                  string
	License                         *license.Manager
	CertManager                     *certs.Manager
	CertRotationGrace               time.Duration
	ArtifactLifecycle               *lifecycle.Manager
	ArtifactPullHosts               []string
	ArtifactPullMaxBytes            int64
	ArtifactPullTimeout             time.Duration
	ArtifactPullAllowInsecureHTTP   bool
	ArtifactTrustVerificationMode   string
	ArtifactTrustAllowedKeyIDs      []string
	ArtifactTrustAllowedSigTypes    []string
	ArtifactSignatureRequireDefault bool
	ArtifactSignatureEnforceIngest  bool
	ArtifactSignatureKeyID          string
	HardenedProfile                 bool
	ArtifactPullCreds               artifactingest.CredentialResolver
	ArtifactPullCredsManager        *artifactingest.PullCredentialManager
	ReleaseAutoUpdate               *releaseautoupdate.Manager
	DeviceIdentityMode              string
	DeviceIdentityRequireOnEnroll   bool
	DeviceIdentityRequireOnCheckin  bool
	// Keyless cosign / Sigstore verification options.
	ArtifactFulcioRootCert  string
	ArtifactRekorURL        string
	ArtifactRequireRekorLog bool
	// Vulnerability scanning (nil when disabled).
	ArtifactScanJob       *vulnscan.ArtifactScanJob
	NessusSyncJob         *vulnscan.NessusSyncJob
	VulnSkipArtifactTypes []string
	// Email delivery.
	Mailer       mailer.Mailer
	AppPublicURL string
	SMTPEnabled  bool
	// Webhook outbound delivery.
	// WebhookEncryptionKey is a 32-byte AES-256 key used to encrypt webhook
	// signing secrets at rest. If nil/empty, webhook creation is disabled.
	WebhookEncryptionKey []byte
	// Deploy triggers.
	TriggerFanoutLimit int // max devices per group trigger; 0 → default 500
}
