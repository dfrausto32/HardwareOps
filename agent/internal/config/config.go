package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ControlPlaneURL       string
	StatePath             string
	ArtifactRoot          string
	CheckinInterval       time.Duration
	CheckinJitterPercent  float64
	CheckinJitterMax      time.Duration
	CheckinStartupJitter  time.Duration
	DeviceCertPath        string
	DeviceKeyPath         string
	CACertPath            string
	LogExportAddr         string
	LogLevel              string
	AllowUnsupportedApply bool
	SigningPubKeyPath     string
	SigningKeyID          string
	RequireSignature      bool
}

func FromEnv() Config {
	url := os.Getenv("CONTROL_PLANE_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	statePath := os.Getenv("STATE_PATH")
	if statePath == "" {
		statePath = "./agent-state.json"
	}
	root := os.Getenv("ARTIFACT_ROOT")
	if root == "" {
		root = "./agent-data"
	}
	interval := 30 * time.Second
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
	return Config{
		ControlPlaneURL:       url,
		StatePath:             statePath,
		ArtifactRoot:          root,
		CheckinInterval:       interval,
		CheckinJitterPercent:  jitterPct,
		CheckinJitterMax:      parseDurationEnv("CHECKIN_JITTER_MAX", 0),
		CheckinStartupJitter:  parseDurationEnv("CHECKIN_STARTUP_JITTER", 0),
		DeviceCertPath:        os.Getenv("DEVICE_CERT_PATH"),
		DeviceKeyPath:         os.Getenv("DEVICE_KEY_PATH"),
		CACertPath:            os.Getenv("CONTROL_PLANE_CA_CERT_PATH"),
		LogExportAddr:         os.Getenv("LOG_EXPORT_ADDR"),
		LogLevel:              os.Getenv("LOG_LEVEL"),
		AllowUnsupportedApply: parseBoolEnv("ALLOW_UNSUPPORTED_APPLY"),
		SigningPubKeyPath:     os.Getenv("SIGNING_PUB_KEY_PATH"),
		SigningKeyID:          os.Getenv("SIGNING_KEY_ID"),
		RequireSignature:      parseBoolEnv("REQUIRE_ARTIFACT_SIGNATURE"),
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
