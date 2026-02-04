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
	DeviceCertPath        string
	DeviceKeyPath         string
	CACertPath            string
	LogExportAddr         string
	LogLevel              string
	AllowUnsupportedApply bool
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
	return Config{
		ControlPlaneURL:       url,
		StatePath:             statePath,
		ArtifactRoot:          root,
		CheckinInterval:       interval,
		DeviceCertPath:        os.Getenv("DEVICE_CERT_PATH"),
		DeviceKeyPath:         os.Getenv("DEVICE_KEY_PATH"),
		CACertPath:            os.Getenv("CONTROL_PLANE_CA_CERT_PATH"),
		LogExportAddr:         os.Getenv("LOG_EXPORT_ADDR"),
		LogLevel:              os.Getenv("LOG_LEVEL"),
		AllowUnsupportedApply: parseBoolEnv("ALLOW_UNSUPPORTED_APPLY"),
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
