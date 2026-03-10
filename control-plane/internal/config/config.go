package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                             string
	DatabaseURL                          string
	CACertPath                           string
	CAKeyPath                            string
	CABundlePath                         string
	ActiveCACertPath                     string
	ActiveCAKeyPath                      string
	TLSCertPath                          string
	TLSKeyPath                           string
	TLSClientCA                          string
	TrustProxy                           bool
	TrustedProxyCIDRs                    []string
	TrustedProxyCIDRsExplicit            bool
	ClientCertHeader                     string
	AutoMigrate                          bool
	MigrationsDir                        string
	S3Endpoint                           string
	S3Bucket                             string
	S3AccessKey                          string
	S3SecretKey                          string
	S3Region                             string
	S3UseSSL                             bool
	PresignTTL                           time.Duration
	EnrollmentTokenRPM                   int
	EnrollRPM                            int
	CheckinRPM                           int
	ApplyResultRPM                       int
	LogIngestAddr                        string
	LogDir                               string
	LogLevel                             string
	DeviceStaleTTL                       time.Duration
	DeviceCleanupInterval                time.Duration
	DeviceStatusStaleAfter               time.Duration
	DeviceStatusOfflineAfter             time.Duration
	DeviceStatusRefreshInterval          time.Duration
	ArtifactPruneEnabled                 bool
	ArtifactPruneInterval                time.Duration
	ArtifactPruneBatchLimit              int
	ArtifactPruneAlertRefThreshold       int
	ArtifactPullAllowedHosts             []string
	ArtifactPullMaxBytes                 int64
	ArtifactPullTimeout                  time.Duration
	ArtifactPullAllowInsecureHTTP        bool
	ArtifactSignatureRequireDefault      bool
	ArtifactSignatureEnforceIngest       bool
	ArtifactSignatureKeyID               string
	ArtifactPullCredentialsJSON          string
	ArtifactPullCredentialsFile          string
	ArtifactPullCredentialsAWSSecretID   string
	ArtifactPullCredentialsAWSRegion     string
	ReleaseAutoUpdateInterval            time.Duration
	MaintenanceEnabled                   bool
	MaintenanceMessage                   string
	MaintenanceToken                     string
	UpgradeApplyCmd                      string
	UpgradeWorkDir                       string
	UpgradeLogDir                        string
	UpgradeUpdatesDir                    string
	UpgradeRunnerMode                    string
	UpgradeRunnerImage                   string
	UpgradeRunnerURL                     string
	UpgradeRunnerToken                   string
	BackupCmd                            string
	BackupWorkDir                        string
	BackupLogDir                         string
	BackupDir                            string
	BackupRunnerMode                     string
	BackupRunnerImage                    string
	BackupRunnerURL                      string
	BackupRunnerToken                    string
	BackupPostgresContainer              string
	BackupMinioContainer                 string
	BackupPostgresUser                   string
	BackupPostgresDB                     string
	RestoreCmd                           string
	MetricsEnabled                       bool
	MetricsPath                          string
	MetricsRefreshInterval               time.Duration
	CertRotationGracePeriod              time.Duration
	DisableHTTP2                         bool
	CORSAllowedOrigins                   []string
	AuditRetentionDays                   int
	AuditRetentionCleanupInterval        time.Duration
	RuntimeEventRetentionDays            int
	RuntimeEventRetentionCleanupInterval time.Duration
	AuthMode                             string
	AuthJWTSecret                        string
	AuthTokenTTL                         time.Duration
	AuthLoginRPM                         int
	PendingEnrollRequestRPMPerSource     int
	PendingEnrollRequestRPMPerProfile    int
	PendingEnrollMaxActive               int
	PendingEnrollMaxActivePerProfile     int
	PendingEnrollMaxActivePerSource      int
	AuthLoginBackoffEnabled              bool
	AuthLoginBackoffThreshold            int
	AuthLoginBackoffBase                 time.Duration
	AuthLoginBackoffMax                  time.Duration
	AuthLoginBackoffWindow               time.Duration
	AuthIssuer                           string
	AuthBootstrapEmail                   string
	AuthBootstrapPassword                string
	AuthOIDCIssuer                       string
	AuthOIDCClientID                     string
	AuthOIDCClientSecret                 string
	AuthOIDCRedirectURL                  string
	AuthOIDCScopes                       string
	AuthOIDCGroupClaim                   string
	AuthOIDCRoleMap                      string
	AuthOIDCDefaultRole                  string
	BootstrapToken                       string
	LicensePath                          string
	LicensePublicKey                     string
	LicensePublicKeyPath                 string
	LicenseKeyMode                       string
	LicenseEnforce                       bool
	LicenseCacheTTL                      time.Duration
	DeviceIdentityMode                   string
	DeviceIdentityRequireOnEnroll        bool
	DeviceIdentityRequireOnCheckin       bool
	HardenedProfile                      bool
}

const defaultTrustedProxyCIDRs = "127.0.0.1/32,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10,fc00::/7,fe80::/10"

func FromEnv() Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	trustedProxyCIDRs, trustedProxyCIDRsExplicit := trustedProxyCIDRsFromEnv()

	staleAfter := parseDurationDefault(getenvDefault("DEVICE_STATUS_STALE_AFTER", "2m"), 2*time.Minute)
	offlineAfter := parseDurationDefault(getenvDefault("DEVICE_STATUS_OFFLINE_AFTER", "10m"), 10*time.Minute)
	if staleAfter > 0 && offlineAfter > 0 && offlineAfter <= staleAfter {
		offlineAfter = staleAfter * 2
	}
	statusRefresh := parseDurationDefault(getenvDefault("DEVICE_STATUS_REFRESH_INTERVAL", "30s"), 30*time.Second)

	return Config{
		HTTPAddr:                             addr,
		DatabaseURL:                          os.Getenv("DATABASE_URL"),
		CACertPath:                           os.Getenv("CA_CERT_PATH"),
		CAKeyPath:                            os.Getenv("CA_KEY_PATH"),
		CABundlePath:                         os.Getenv("CA_BUNDLE_PATH"),
		ActiveCACertPath:                     os.Getenv("ACTIVE_CA_CERT_PATH"),
		ActiveCAKeyPath:                      os.Getenv("ACTIVE_CA_KEY_PATH"),
		TLSCertPath:                          os.Getenv("TLS_CERT_PATH"),
		TLSKeyPath:                           os.Getenv("TLS_KEY_PATH"),
		TLSClientCA:                          os.Getenv("TLS_CLIENT_CA_PATH"),
		TrustProxy:                           os.Getenv("TRUST_PROXY") == "1",
		TrustedProxyCIDRs:                    trustedProxyCIDRs,
		TrustedProxyCIDRsExplicit:            trustedProxyCIDRsExplicit,
		ClientCertHeader:                     getenvDefault("CLIENT_CERT_HEADER", "X-Client-Cert"),
		AutoMigrate:                          os.Getenv("AUTO_MIGRATE") == "1",
		MigrationsDir:                        getenvDefault("MIGRATIONS_DIR", "./migrations"),
		S3Endpoint:                           os.Getenv("S3_ENDPOINT"),
		S3Bucket:                             os.Getenv("S3_BUCKET"),
		S3AccessKey:                          os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:                          os.Getenv("S3_SECRET_KEY"),
		S3Region:                             os.Getenv("S3_REGION"),
		S3UseSSL:                             os.Getenv("S3_USE_SSL") == "1",
		PresignTTL:                           parseDuration(getenvDefault("S3_PRESIGN_TTL", "5m")),
		EnrollmentTokenRPM:                   getenvInt("ENROLLMENT_TOKEN_RPM", 30),
		EnrollRPM:                            getenvInt("ENROLL_RPM", 60),
		CheckinRPM:                           getenvInt("CHECKIN_RPM", 300),
		ApplyResultRPM:                       getenvInt("APPLY_RESULT_RPM", 300),
		LogIngestAddr:                        os.Getenv("LOG_INGEST_ADDR"),
		LogDir:                               getenvDefault("LOG_DIR", "./logs"),
		LogLevel:                             getenvDefault("LOG_LEVEL", "info"),
		DeviceStaleTTL:                       parseDurationDefault(getenvDefault("DEVICE_STALE_TTL", "1h"), time.Hour),
		DeviceCleanupInterval:                parseDurationDefault(getenvDefault("DEVICE_CLEANUP_INTERVAL", "5m"), 5*time.Minute),
		DeviceStatusStaleAfter:               staleAfter,
		DeviceStatusOfflineAfter:             offlineAfter,
		DeviceStatusRefreshInterval:          statusRefresh,
		ArtifactPruneEnabled:                 parseBoolEnvDefault("ARTIFACT_PRUNE_ENABLED", true),
		ArtifactPruneInterval:                parseDurationDefault(getenvDefault("ARTIFACT_PRUNE_INTERVAL", "24h"), 24*time.Hour),
		ArtifactPruneBatchLimit:              getenvInt("ARTIFACT_PRUNE_BATCH_LIMIT", 200),
		ArtifactPruneAlertRefThreshold:       getenvInt("ARTIFACT_PRUNE_ALERT_REF_THRESHOLD", 10),
		ArtifactPullAllowedHosts:             parseCSV(getenvDefault("ARTIFACT_PULL_ALLOWED_HOSTS", "")),
		ArtifactPullMaxBytes:                 getenvInt64("ARTIFACT_PULL_MAX_BYTES", 1024*1024*1024),
		ArtifactPullTimeout:                  parseDurationDefault(getenvDefault("ARTIFACT_PULL_TIMEOUT", "15m"), 15*time.Minute),
		ArtifactPullAllowInsecureHTTP:        parseBoolEnvDefault("ARTIFACT_PULL_ALLOW_INSECURE_HTTP", false),
		ArtifactSignatureRequireDefault:      parseBoolEnvDefault("ARTIFACT_SIGNATURE_REQUIRE_DEFAULT", false),
		ArtifactSignatureEnforceIngest:       parseBoolEnvDefault("ARTIFACT_SIGNATURE_ENFORCE_INGEST", false),
		ArtifactSignatureKeyID:               strings.TrimSpace(os.Getenv("ARTIFACT_SIGNATURE_KEY_ID")),
		ArtifactPullCredentialsJSON:          os.Getenv("ARTIFACT_PULL_CREDENTIALS_JSON"),
		ArtifactPullCredentialsFile:          os.Getenv("ARTIFACT_PULL_CREDENTIALS_FILE"),
		ArtifactPullCredentialsAWSSecretID:   os.Getenv("ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID"),
		ArtifactPullCredentialsAWSRegion:     os.Getenv("ARTIFACT_PULL_CREDENTIALS_AWS_REGION"),
		ReleaseAutoUpdateInterval:            parseDurationDefault(getenvDefault("RELEASE_AUTO_UPDATE_INTERVAL", "60s"), 60*time.Second),
		MaintenanceEnabled:                   parseBoolEnv("MAINTENANCE_MODE"),
		MaintenanceMessage:                   getenvDefault("MAINTENANCE_MESSAGE", ""),
		MaintenanceToken:                     os.Getenv("MAINTENANCE_TOKEN"),
		UpgradeApplyCmd:                      os.Getenv("UPGRADE_APPLY_CMD"),
		UpgradeWorkDir:                       os.Getenv("UPGRADE_WORK_DIR"),
		UpgradeLogDir:                        os.Getenv("UPGRADE_LOG_DIR"),
		UpgradeUpdatesDir:                    os.Getenv("UPGRADE_UPDATES_DIR"),
		UpgradeRunnerMode:                    getenvDefault("UPGRADE_RUNNER_MODE", "disabled"),
		UpgradeRunnerImage:                   os.Getenv("UPGRADE_RUNNER_IMAGE"),
		UpgradeRunnerURL:                     os.Getenv("UPGRADE_RUNNER_URL"),
		UpgradeRunnerToken:                   os.Getenv("UPGRADE_RUNNER_TOKEN"),
		BackupCmd:                            os.Getenv("BACKUP_CMD"),
		BackupWorkDir:                        os.Getenv("BACKUP_WORK_DIR"),
		BackupLogDir:                         os.Getenv("BACKUP_LOG_DIR"),
		BackupDir:                            getenvDefault("BACKUP_DIR", "/stack/backups"),
		BackupRunnerMode:                     getenvDefault("BACKUP_RUNNER_MODE", "disabled"),
		BackupRunnerImage:                    os.Getenv("BACKUP_RUNNER_IMAGE"),
		BackupRunnerURL:                      os.Getenv("BACKUP_RUNNER_URL"),
		BackupRunnerToken:                    os.Getenv("BACKUP_RUNNER_TOKEN"),
		BackupPostgresContainer:              os.Getenv("BACKUP_POSTGRES_CONTAINER"),
		BackupMinioContainer:                 os.Getenv("BACKUP_MINIO_CONTAINER"),
		BackupPostgresUser:                   getenvDefault("BACKUP_POSTGRES_USER", "hardwareops"),
		BackupPostgresDB:                     getenvDefault("BACKUP_POSTGRES_DB", "hardwareops"),
		RestoreCmd:                           os.Getenv("RESTORE_CMD"),
		MetricsEnabled:                       parseBoolEnvDefault("METRICS_ENABLED", true),
		MetricsPath:                          getenvDefault("METRICS_PATH", "/metrics"),
		MetricsRefreshInterval:               parseDurationDefault(getenvDefault("METRICS_REFRESH_INTERVAL", "30s"), 30*time.Second),
		CertRotationGracePeriod:              parseDurationDefault(getenvDefault("CERT_ROTATION_GRACE_PERIOD", "168h"), 168*time.Hour),
		DisableHTTP2:                         os.Getenv("DISABLE_HTTP2") == "1",
		CORSAllowedOrigins:                   parseCSV(getenvDefault("CORS_ALLOWED_ORIGINS", "")),
		AuditRetentionDays:                   getenvInt("AUDIT_RETENTION_DAYS", 90),
		AuditRetentionCleanupInterval:        parseDurationDefault(getenvDefault("AUDIT_RETENTION_CLEANUP_INTERVAL", "1h"), time.Hour),
		RuntimeEventRetentionDays:            getenvInt("EVENT_RETENTION_DAYS", 30),
		RuntimeEventRetentionCleanupInterval: parseDurationDefault(getenvDefault("EVENT_RETENTION_CLEANUP_INTERVAL", "1h"), time.Hour),
		AuthMode:                             getenvDefault("AUTH_MODE", "disabled"),
		AuthJWTSecret:                        os.Getenv("AUTH_JWT_SECRET"),
		AuthTokenTTL:                         parseDurationDefault(getenvDefault("AUTH_TOKEN_TTL", "12h"), 12*time.Hour),
		AuthLoginRPM:                         getenvInt("AUTH_LOGIN_RPM", 30),
		PendingEnrollRequestRPMPerSource:     getenvInt("PENDING_ENROLL_REQUEST_RPM_PER_SOURCE", 30),
		PendingEnrollRequestRPMPerProfile:    getenvInt("PENDING_ENROLL_REQUEST_RPM_PER_PROFILE", 120),
		PendingEnrollMaxActive:               getenvInt("PENDING_ENROLL_MAX_ACTIVE", 1000),
		PendingEnrollMaxActivePerProfile:     getenvInt("PENDING_ENROLL_MAX_ACTIVE_PER_PROFILE", 100),
		PendingEnrollMaxActivePerSource:      getenvInt("PENDING_ENROLL_MAX_ACTIVE_PER_SOURCE", 10),
		AuthLoginBackoffEnabled:              parseBoolEnvDefault("AUTH_LOGIN_BACKOFF_ENABLED", true),
		AuthLoginBackoffThreshold:            getenvInt("AUTH_LOGIN_BACKOFF_THRESHOLD", 3),
		AuthLoginBackoffBase:                 parseDurationDefault(getenvDefault("AUTH_LOGIN_BACKOFF_BASE", "2s"), 2*time.Second),
		AuthLoginBackoffMax:                  parseDurationDefault(getenvDefault("AUTH_LOGIN_BACKOFF_MAX", "5m"), 5*time.Minute),
		AuthLoginBackoffWindow:               parseDurationDefault(getenvDefault("AUTH_LOGIN_BACKOFF_WINDOW", "15m"), 15*time.Minute),
		AuthIssuer:                           getenvDefault("AUTH_ISSUER", "hardwareops"),
		AuthBootstrapEmail:                   os.Getenv("AUTH_BOOTSTRAP_EMAIL"),
		AuthBootstrapPassword:                os.Getenv("AUTH_BOOTSTRAP_PASSWORD"),
		AuthOIDCIssuer:                       os.Getenv("AUTH_OIDC_ISSUER"),
		AuthOIDCClientID:                     os.Getenv("AUTH_OIDC_CLIENT_ID"),
		AuthOIDCClientSecret:                 os.Getenv("AUTH_OIDC_CLIENT_SECRET"),
		AuthOIDCRedirectURL:                  os.Getenv("AUTH_OIDC_REDIRECT_URL"),
		AuthOIDCScopes:                       getenvDefault("AUTH_OIDC_SCOPES", "openid email profile groups"),
		AuthOIDCGroupClaim:                   getenvDefault("AUTH_OIDC_GROUP_CLAIM", "groups"),
		AuthOIDCRoleMap:                      os.Getenv("AUTH_OIDC_ROLE_MAP"),
		AuthOIDCDefaultRole:                  getenvDefault("AUTH_OIDC_DEFAULT_ROLE", "viewer"),
		BootstrapToken:                       os.Getenv("BOOTSTRAP_TOKEN"),
		LicensePath:                          os.Getenv("LICENSE_PATH"),
		LicensePublicKey:                     os.Getenv("LICENSE_PUBLIC_KEY"),
		LicensePublicKeyPath:                 os.Getenv("LICENSE_PUBLIC_KEY_PATH"),
		LicenseKeyMode:                       getenvDefault("LICENSE_KEY_MODE", "env"),
		LicenseEnforce:                       parseBoolEnv("LICENSE_ENFORCE"),
		LicenseCacheTTL:                      parseDurationDefault(getenvDefault("LICENSE_CACHE_TTL", "30s"), 30*time.Second),
		DeviceIdentityMode:                   getenvDefault("DEVICE_IDENTITY_MODE", "audit"),
		DeviceIdentityRequireOnEnroll:        parseBoolEnvDefault("DEVICE_IDENTITY_REQUIRE_ON_ENROLL", false),
		DeviceIdentityRequireOnCheckin:       parseBoolEnvDefault("DEVICE_IDENTITY_REQUIRE_ON_CHECKIN", false),
		HardenedProfile:                      parseBoolEnvDefault("HARDENED_PROFILE", false),
	}
}

func trustedProxyCIDRsFromEnv() ([]string, bool) {
	raw, ok := os.LookupEnv("TRUST_PROXY_CIDRS")
	if !ok {
		return parseCSV(defaultTrustedProxyCIDRs), false
	}
	return parseCSV(raw), true
}

func getenvDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func parseDuration(val string) time.Duration {
	d, err := time.ParseDuration(val)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

func parseDurationDefault(val string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(val)
	if err != nil {
		return def
	}
	return d
}

func parseCSV(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func getenvInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}

func getenvInt64(key string, def int64) int64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func parseBoolEnv(key string) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseBoolEnvDefault(key string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	return parseBoolEnv(key)
}

// OIDCEnabled returns true when OIDC SSO is configured.
func (c *Config) OIDCEnabled() bool {
	return strings.TrimSpace(c.AuthOIDCIssuer) != ""
}
