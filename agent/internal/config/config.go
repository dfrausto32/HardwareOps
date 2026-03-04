package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ControlPlaneURL        string
	EnrollmentMode         string
	EnrollmentProfileToken string
	StatePath              string
	BootstrapStatePath     string
	ArtifactRoot           string
	CheckinInterval        time.Duration
	CheckinJitterPercent   float64
	CheckinJitterMax       time.Duration
	CheckinStartupJitter   time.Duration
	BootstrapPollInterval  time.Duration
	BootstrapRetryInterval time.Duration
	DeviceCertPath         string
	DeviceKeyPath          string
	DeviceIDPath           string
	CACertPath             string
	LogExportAddr          string
	LogLevel               string
	AllowUnsupportedApply  bool
	SigningPubKeyPath      string
	SigningKeyID           string
	RequireSignature       bool
	AutoReenroll           bool
}

func FromEnv() Config {
	url := os.Getenv("CONTROL_PLANE_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	enrollmentMode := strings.TrimSpace(strings.ToLower(os.Getenv("AGENT_ENROLL_MODE")))
	statePath := os.Getenv("STATE_PATH")
	if statePath == "" {
		statePath = "./agent-state.json"
	}
	bootstrapStatePath := os.Getenv("BOOTSTRAP_STATE_PATH")
	if bootstrapStatePath == "" {
		bootstrapStatePath = filepath.Join(filepath.Dir(statePath), "bootstrap-state.json")
	}
	root := os.Getenv("ARTIFACT_ROOT")
	if root == "" {
		root = "./agent-data"
	}
	interval := 5 * time.Second
	if raw := os.Getenv("CHECKIN_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			interval = d
		}
	} else if raw := os.Getenv("CHECKIN_INTERVAL_SEC"); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			interval = time.Duration(sec) * time.Second
		}
	}
	jitterPct := parseFloatEnv("CHECKIN_JITTER_PCT", 0.1)
	if jitterPct < 0 {
		jitterPct = 0
	}
	deviceCertPath := os.Getenv("DEVICE_CERT_PATH")
	deviceKeyPath := os.Getenv("DEVICE_KEY_PATH")
	deviceIDPath := os.Getenv("DEVICE_ID_PATH")
	if enrollmentMode == "approval" {
		if deviceCertPath == "" {
			deviceCertPath = filepath.Join(filepath.Dir(statePath), "device.crt")
		}
		if deviceKeyPath == "" {
			deviceKeyPath = filepath.Join(filepath.Dir(statePath), "device.key")
		}
		if deviceIDPath == "" {
			deviceIDPath = filepath.Join(filepath.Dir(statePath), "device-id")
		}
	}
	return Config{
		ControlPlaneURL:        url,
		EnrollmentMode:         enrollmentMode,
		EnrollmentProfileToken: strings.TrimSpace(os.Getenv("ENROLLMENT_PROFILE_TOKEN")),
		StatePath:              statePath,
		BootstrapStatePath:     bootstrapStatePath,
		ArtifactRoot:           root,
		CheckinInterval:        interval,
		CheckinJitterPercent:   jitterPct,
		CheckinJitterMax:       parseDurationEnv("CHECKIN_JITTER_MAX", 0),
		CheckinStartupJitter:   parseDurationEnv("CHECKIN_STARTUP_JITTER", 0),
		BootstrapPollInterval:  parseDurationEnv("BOOTSTRAP_POLL_INTERVAL", 5*time.Second),
		BootstrapRetryInterval: parseDurationEnv("BOOTSTRAP_RETRY_INTERVAL", 10*time.Second),
		DeviceCertPath:         deviceCertPath,
		DeviceKeyPath:          deviceKeyPath,
		DeviceIDPath:           deviceIDPath,
		CACertPath:             os.Getenv("CONTROL_PLANE_CA_CERT_PATH"),
		LogExportAddr:          os.Getenv("LOG_EXPORT_ADDR"),
		LogLevel:               os.Getenv("LOG_LEVEL"),
		AllowUnsupportedApply:  parseBoolEnv("ALLOW_UNSUPPORTED_APPLY"),
		SigningPubKeyPath:      os.Getenv("SIGNING_PUB_KEY_PATH"),
		SigningKeyID:           os.Getenv("SIGNING_KEY_ID"),
		RequireSignature:       parseBoolEnv("REQUIRE_ARTIFACT_SIGNATURE"),
		AutoReenroll:           parseBoolEnvDefault("AUTO_REENROLL", true),
	}
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

func parseDurationEnv(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return def
}

func parseFloatEnv(key string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return val
}
