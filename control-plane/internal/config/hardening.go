package config

import (
	"fmt"
	"strings"
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

	return nil
}
