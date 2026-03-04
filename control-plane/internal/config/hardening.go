package config

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// ValidateHardening enforces strict runtime guardrails when HARDENED_PROFILE=1.
func ValidateHardening(cfg Config) error {
	if !cfg.HardenedProfile {
		return nil
	}

	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if authMode == "" || authMode == "disabled" {
		return fmt.Errorf("HARDENED_PROFILE requires AUTH_MODE to be enabled")
	}
	if authMode == "local" {
		if strings.TrimSpace(cfg.AuthJWTSecret) == "" {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_JWT_SECRET")
		}
		if len(strings.TrimSpace(cfg.AuthJWTSecret)) < 32 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_JWT_SECRET length >= 32")
		}
		if looksPlaceholderSecret(cfg.AuthJWTSecret) {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_JWT_SECRET to be non-placeholder")
		}
		if cfg.AuthTokenTTL <= 0 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_TOKEN_TTL > 0")
		}
		if cfg.AuthTokenTTL > 24*time.Hour {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_TOKEN_TTL <= 24h")
		}
		if cfg.AuthLoginRPM <= 0 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_RPM > 0")
		}
		if !cfg.AuthLoginBackoffEnabled {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_BACKOFF_ENABLED=1")
		}
		if cfg.AuthLoginBackoffThreshold < 1 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_BACKOFF_THRESHOLD >= 1")
		}
		if cfg.AuthLoginBackoffBase <= 0 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_BACKOFF_BASE > 0")
		}
		if cfg.AuthLoginBackoffMax < cfg.AuthLoginBackoffBase {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_BACKOFF_MAX >= AUTH_LOGIN_BACKOFF_BASE")
		}
		if cfg.AuthLoginBackoffWindow <= 0 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_LOGIN_BACKOFF_WINDOW > 0")
		}
		if strings.TrimSpace(cfg.AuthBootstrapPassword) == "" {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_BOOTSTRAP_PASSWORD")
		}
		if len(strings.TrimSpace(cfg.AuthBootstrapPassword)) < 12 {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_BOOTSTRAP_PASSWORD length >= 12")
		}
		if looksPlaceholderSecret(cfg.AuthBootstrapPassword) {
			return fmt.Errorf("HARDENED_PROFILE requires AUTH_BOOTSTRAP_PASSWORD to be non-placeholder")
		}
	}

	if token := strings.TrimSpace(cfg.BootstrapToken); token != "" {
		if len(token) < 16 {
			return fmt.Errorf("HARDENED_PROFILE requires BOOTSTRAP_TOKEN length >= 16 when set")
		}
		if looksPlaceholderSecret(token) {
			return fmt.Errorf("HARDENED_PROFILE requires BOOTSTRAP_TOKEN to be non-placeholder")
		}
	}
	if token := strings.TrimSpace(cfg.MaintenanceToken); token != "" {
		if len(token) < 16 {
			return fmt.Errorf("HARDENED_PROFILE requires MAINTENANCE_TOKEN length >= 16 when set")
		}
		if looksPlaceholderSecret(token) {
			return fmt.Errorf("HARDENED_PROFILE requires MAINTENANCE_TOKEN to be non-placeholder")
		}
	}

	if !cfg.LicenseEnforce {
		return fmt.Errorf("HARDENED_PROFILE requires LICENSE_ENFORCE=1")
	}
	if strings.TrimSpace(cfg.LicensePath) == "" {
		return fmt.Errorf("HARDENED_PROFILE requires LICENSE_PATH")
	}

	identityMode := strings.ToLower(strings.TrimSpace(cfg.DeviceIdentityMode))
	if identityMode != "enforce" {
		return fmt.Errorf("HARDENED_PROFILE requires DEVICE_IDENTITY_MODE=enforce")
	}
	if !cfg.DeviceIdentityRequireOnEnroll {
		return fmt.Errorf("HARDENED_PROFILE requires DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1")
	}
	if !cfg.DeviceIdentityRequireOnCheckin {
		return fmt.Errorf("HARDENED_PROFILE requires DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1")
	}
	if len(cfg.ArtifactPullAllowedHosts) == 0 {
		return fmt.Errorf("HARDENED_PROFILE requires ARTIFACT_PULL_ALLOWED_HOSTS")
	}
	if cfg.ArtifactPullAllowInsecureHTTP {
		return fmt.Errorf("HARDENED_PROFILE requires ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0")
	}
	if !cfg.ArtifactSignatureRequireDefault {
		return fmt.Errorf("HARDENED_PROFILE requires ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1")
	}
	if !cfg.ArtifactSignatureEnforceIngest {
		return fmt.Errorf("HARDENED_PROFILE requires ARTIFACT_SIGNATURE_ENFORCE_INGEST=1")
	}
	if strings.TrimSpace(cfg.ArtifactSignatureKeyID) == "" {
		return fmt.Errorf("HARDENED_PROFILE requires ARTIFACT_SIGNATURE_KEY_ID")
	}
	if cfg.TrustProxy {
		if len(cfg.TrustedProxyCIDRs) == 0 {
			return fmt.Errorf("HARDENED_PROFILE requires TRUST_PROXY_CIDRS when TRUST_PROXY=1")
		}
	}
	if err := validateTrustedProxyCIDRs(cfg.TrustedProxyCIDRs, true); err != nil {
		return fmt.Errorf("HARDENED_PROFILE invalid TRUST_PROXY_CIDRS: %w", err)
	}
	upgradeMode := strings.ToLower(strings.TrimSpace(cfg.UpgradeRunnerMode))
	if upgradeMode == "docker" {
		return fmt.Errorf("HARDENED_PROFILE requires UPGRADE_RUNNER_MODE to avoid docker socket in control-plane (use remote)")
	}
	if upgradeMode == "remote" {
		if strings.TrimSpace(cfg.UpgradeRunnerURL) == "" {
			return fmt.Errorf("HARDENED_PROFILE requires UPGRADE_RUNNER_URL for remote mode")
		}
		if len(strings.TrimSpace(cfg.UpgradeRunnerToken)) < 16 || looksPlaceholderSecret(cfg.UpgradeRunnerToken) {
			return fmt.Errorf("HARDENED_PROFILE requires strong UPGRADE_RUNNER_TOKEN for remote mode")
		}
	}
	backupMode := strings.ToLower(strings.TrimSpace(cfg.BackupRunnerMode))
	if backupMode == "docker" {
		return fmt.Errorf("HARDENED_PROFILE requires BACKUP_RUNNER_MODE to avoid docker socket in control-plane (use remote)")
	}
	if backupMode == "remote" {
		if strings.TrimSpace(cfg.BackupRunnerURL) == "" {
			return fmt.Errorf("HARDENED_PROFILE requires BACKUP_RUNNER_URL for remote mode")
		}
		if len(strings.TrimSpace(cfg.BackupRunnerToken)) < 16 || looksPlaceholderSecret(cfg.BackupRunnerToken) {
			return fmt.Errorf("HARDENED_PROFILE requires strong BACKUP_RUNNER_TOKEN for remote mode")
		}
	}

	return nil
}

func validateTrustedProxyCIDRs(cidrs []string, rejectWildcard bool) error {
	for _, raw := range cidrs {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				return fmt.Errorf("invalid cidr %q", entry)
			}
			if rejectWildcard && network != nil {
				ones, _ := network.Mask.Size()
				if ones == 0 {
					return fmt.Errorf("wildcard cidr %q not allowed", entry)
				}
			}
			continue
		}
		if ip := net.ParseIP(entry); ip == nil {
			return fmt.Errorf("invalid proxy address %q", entry)
		}
	}
	return nil
}

func looksPlaceholderSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	if strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "changeme") ||
		strings.Contains(normalized, "replace-me") {
		return true
	}
	switch normalized {
	case "password",
		"password123",
		"changeme",
		"change_me",
		"dev-token",
		"devtoken",
		"dev-jwt-secret",
		"dev-jwt-secret-change-me",
		"bootstrap",
		"default":
		return true
	default:
		return false
	}
}
