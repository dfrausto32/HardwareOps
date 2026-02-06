package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                    string
	DatabaseURL                 string
	CACertPath                  string
	CAKeyPath                   string
	TLSCertPath                 string
	TLSKeyPath                  string
	TLSClientCA                 string
	TrustProxy                  bool
	ClientCertHeader            string
	AutoMigrate                 bool
	MigrationsDir               string
	S3Endpoint                  string
	S3Bucket                    string
	S3AccessKey                 string
	S3SecretKey                 string
	S3Region                    string
	S3UseSSL                    bool
	PresignTTL                  time.Duration
	EnrollmentTokenRPM          int
	EnrollRPM                   int
	CheckinRPM                  int
	ApplyResultRPM              int
	LogIngestAddr               string
	LogDir                      string
	LogLevel                    string
	DeviceStaleTTL              time.Duration
	DeviceCleanupInterval       time.Duration
	DeviceStatusStaleAfter      time.Duration
	DeviceStatusOfflineAfter    time.Duration
	DeviceStatusRefreshInterval time.Duration
	MaintenanceEnabled          bool
	MaintenanceMessage          string
	MaintenanceToken            string
	UpgradeApplyCmd             string
	UpgradeWorkDir              string
	UpgradeLogDir               string
	UpgradeUpdatesDir           string
	DisableHTTP2                bool
	CORSAllowedOrigins          []string
}

func FromEnv() Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	staleAfter := parseDurationDefault(getenvDefault("DEVICE_STATUS_STALE_AFTER", "2m"), 2*time.Minute)
	offlineAfter := parseDurationDefault(getenvDefault("DEVICE_STATUS_OFFLINE_AFTER", "10m"), 10*time.Minute)
	if staleAfter > 0 && offlineAfter > 0 && offlineAfter <= staleAfter {
		offlineAfter = staleAfter * 2
	}
	statusRefresh := parseDurationDefault(getenvDefault("DEVICE_STATUS_REFRESH_INTERVAL", "30s"), 30*time.Second)

	return Config{
		HTTPAddr:                    addr,
		DatabaseURL:                 os.Getenv("DATABASE_URL"),
		CACertPath:                  os.Getenv("CA_CERT_PATH"),
		CAKeyPath:                   os.Getenv("CA_KEY_PATH"),
		TLSCertPath:                 os.Getenv("TLS_CERT_PATH"),
		TLSKeyPath:                  os.Getenv("TLS_KEY_PATH"),
		TLSClientCA:                 os.Getenv("TLS_CLIENT_CA_PATH"),
		TrustProxy:                  os.Getenv("TRUST_PROXY") == "1",
		ClientCertHeader:            getenvDefault("CLIENT_CERT_HEADER", "X-Client-Cert"),
		AutoMigrate:                 os.Getenv("AUTO_MIGRATE") == "1",
		MigrationsDir:               getenvDefault("MIGRATIONS_DIR", "./migrations"),
		S3Endpoint:                  os.Getenv("S3_ENDPOINT"),
		S3Bucket:                    os.Getenv("S3_BUCKET"),
		S3AccessKey:                 os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:                 os.Getenv("S3_SECRET_KEY"),
		S3Region:                    os.Getenv("S3_REGION"),
		S3UseSSL:                    os.Getenv("S3_USE_SSL") == "1",
		PresignTTL:                  parseDuration(getenvDefault("S3_PRESIGN_TTL", "5m")),
		EnrollmentTokenRPM:          getenvInt("ENROLLMENT_TOKEN_RPM", 30),
		EnrollRPM:                   getenvInt("ENROLL_RPM", 60),
		CheckinRPM:                  getenvInt("CHECKIN_RPM", 300),
		ApplyResultRPM:              getenvInt("APPLY_RESULT_RPM", 300),
		LogIngestAddr:               os.Getenv("LOG_INGEST_ADDR"),
		LogDir:                      getenvDefault("LOG_DIR", "./logs"),
		LogLevel:                    getenvDefault("LOG_LEVEL", "info"),
		DeviceStaleTTL:              parseDurationDefault(getenvDefault("DEVICE_STALE_TTL", "1h"), time.Hour),
		DeviceCleanupInterval:       parseDurationDefault(getenvDefault("DEVICE_CLEANUP_INTERVAL", "5m"), 5*time.Minute),
		DeviceStatusStaleAfter:      staleAfter,
		DeviceStatusOfflineAfter:    offlineAfter,
		DeviceStatusRefreshInterval: statusRefresh,
		MaintenanceEnabled:          parseBoolEnv("MAINTENANCE_MODE"),
		MaintenanceMessage:          getenvDefault("MAINTENANCE_MESSAGE", ""),
		MaintenanceToken:            os.Getenv("MAINTENANCE_TOKEN"),
		UpgradeApplyCmd:             os.Getenv("UPGRADE_APPLY_CMD"),
		UpgradeWorkDir:              os.Getenv("UPGRADE_WORK_DIR"),
		UpgradeLogDir:               os.Getenv("UPGRADE_LOG_DIR"),
		UpgradeUpdatesDir:           os.Getenv("UPGRADE_UPDATES_DIR"),
		DisableHTTP2:                os.Getenv("DISABLE_HTTP2") == "1",
		CORSAllowedOrigins:          parseCSV(getenvDefault("CORS_ALLOWED_ORIGINS", "")),
	}
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

func parseBoolEnv(key string) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
