package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/hardwareops/agent/internal/logging"
)

func buildCapabilities(logger *logging.Logger) map[string]any {
	hardwareID, source := detectHardwareIdentity()
	if hardwareID == "" {
		if logger != nil {
			logger.Warnf("hardware identity unavailable; duplicate-device hardening may be reduced")
		}
		return map[string]any{}
	}
	return map[string]any{
		"hw": map[string]any{
			"identity": map[string]any{
				"id":     hardwareID,
				"source": source,
			},
		},
	}
}

func detectHardwareIdentity() (string, string) {
	if raw := strings.TrimSpace(os.Getenv("HARDWARE_IDENTITY")); raw != "" {
		return hashHardwareIdentity(raw), "env"
	}
	if raw := readFileTrimmed("/etc/machine-id"); raw != "" {
		return hashHardwareIdentity(raw), "machine-id"
	}
	if raw := readFileTrimmed("/var/lib/dbus/machine-id"); raw != "" {
		return hashHardwareIdentity(raw), "machine-id"
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return hashHardwareIdentity(host), "hostname"
	}
	return "", ""
}

func hashHardwareIdentity(raw string) string {
	salt := strings.TrimSpace(os.Getenv("HARDWARE_IDENTITY_SALT"))
	payload := strings.TrimSpace(raw)
	if salt != "" {
		payload = payload + "|" + salt
	}
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func readFileTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
