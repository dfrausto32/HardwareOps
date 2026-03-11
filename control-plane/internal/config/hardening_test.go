package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateHardening(t *testing.T) {
	base := Config{
		HardenedProfile:                 true,
		AuthMode:                        "local",
		AuthJWTSecret:                   strings.Repeat("a", 32),
		AuthTokenTTL:                    12 * time.Hour,
		AuthLoginRPM:                    30,
		AuthLoginBackoffEnabled:         true,
		AuthLoginBackoffThreshold:       3,
		AuthLoginBackoffBase:            2 * time.Second,
		AuthLoginBackoffMax:             5 * time.Minute,
		AuthLoginBackoffWindow:          15 * time.Minute,
		AuthBootstrapPassword:           "bootstrap-secret-123",
		LicenseEnforce:                  true,
		LicensePath:                     "/opt/hardwareops/license.json",
		DeviceIdentityMode:              "enforce",
		DeviceIdentityRequireOnEnroll:   true,
		DeviceIdentityRequireOnCheckin:  true,
		ArtifactPullAllowedHosts:        []string{"artifacts.vendor.example"},
		ArtifactPullAllowInsecureHTTP:   false,
		ArtifactSignatureRequireDefault: true,
		ArtifactSignatureEnforceIngest:  true,
		ArtifactSignatureKeyID:          "sha256:test-signing-key",
		TrustedSigningKeysFile:          "/opt/hardwareops/signing/trusted-signing-keys.json",
	}

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid hardened config",
			cfg:     base,
			wantErr: false,
		},
		{
			name:    "disabled profile skips validation",
			cfg:     Config{},
			wantErr: false,
		},
		{
			name: "auth disabled rejected",
			cfg: func() Config {
				c := base
				c.AuthMode = "disabled"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth jwt required",
			cfg: func() Config {
				c := base
				c.AuthJWTSecret = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth jwt placeholder rejected",
			cfg: func() Config {
				c := base
				c.AuthJWTSecret = "change-me-change-me-change-me-change-me"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth jwt length enforced",
			cfg: func() Config {
				c := base
				c.AuthJWTSecret = "short-secret"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth token ttl max enforced",
			cfg: func() Config {
				c := base
				c.AuthTokenTTL = 25 * time.Hour
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login rpm required",
			cfg: func() Config {
				c := base
				c.AuthLoginRPM = 0
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login backoff enabled required",
			cfg: func() Config {
				c := base
				c.AuthLoginBackoffEnabled = false
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login backoff threshold required",
			cfg: func() Config {
				c := base
				c.AuthLoginBackoffThreshold = 0
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login backoff base required",
			cfg: func() Config {
				c := base
				c.AuthLoginBackoffBase = 0
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login backoff max required",
			cfg: func() Config {
				c := base
				c.AuthLoginBackoffMax = c.AuthLoginBackoffBase - time.Second
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth login backoff window required",
			cfg: func() Config {
				c := base
				c.AuthLoginBackoffWindow = 0
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth bootstrap password required",
			cfg: func() Config {
				c := base
				c.AuthBootstrapPassword = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth bootstrap password placeholder rejected",
			cfg: func() Config {
				c := base
				c.AuthBootstrapPassword = "change-me"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "auth bootstrap password length enforced",
			cfg: func() Config {
				c := base
				c.AuthBootstrapPassword = "short-pass"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "license enforce required",
			cfg: func() Config {
				c := base
				c.LicenseEnforce = false
				return c
			}(),
			wantErr: true,
		},
		{
			name: "license path required",
			cfg: func() Config {
				c := base
				c.LicensePath = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "identity enforce required",
			cfg: func() Config {
				c := base
				c.DeviceIdentityMode = "audit"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "identity required on enroll",
			cfg: func() Config {
				c := base
				c.DeviceIdentityRequireOnEnroll = false
				return c
			}(),
			wantErr: true,
		},
		{
			name: "identity required on checkin",
			cfg: func() Config {
				c := base
				c.DeviceIdentityRequireOnCheckin = false
				return c
			}(),
			wantErr: true,
		},
		{
			name: "pull allowlist required",
			cfg: func() Config {
				c := base
				c.ArtifactPullAllowedHosts = nil
				return c
			}(),
			wantErr: true,
		},
		{
			name: "strict artifact trust mode required",
			cfg: func() Config {
				c := base
				c.ArtifactTrustVerificationMode = "warn_unsigned"
				c.ArtifactSignatureRequireDefault = false
				c.ArtifactSignatureEnforceIngest = false
				return c
			}(),
			wantErr: true,
		},
		{
			name: "allowed signing key id required",
			cfg: func() Config {
				c := base
				c.ArtifactSignatureKeyID = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "insecure http pull rejected",
			cfg: func() Config {
				c := base
				c.ArtifactPullAllowInsecureHTTP = true
				return c
			}(),
			wantErr: true,
		},
		{
			name: "bootstrap token placeholder rejected",
			cfg: func() Config {
				c := base
				c.BootstrapToken = "change-me-bootstrap-token"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "maintenance token placeholder rejected",
			cfg: func() Config {
				c := base
				c.MaintenanceToken = "change-me-maintenance-token"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "trust proxy cidrs must be explicit when trust proxy enabled",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRs = []string{"10.40.64.0/20"}
				return c
			}(),
			wantErr: true,
		},
		{
			name: "trust proxy cidr required when trust proxy enabled",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = nil
				return c
			}(),
			wantErr: true,
		},
		{
			name: "invalid trust proxy cidr rejected",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"not-a-cidr"}
				return c
			}(),
			wantErr: true,
		},
		{
			name: "wildcard trust proxy cidr rejected",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"0.0.0.0/0"}
				return c
			}(),
			wantErr: true,
		},
		{
			name: "overbroad trust proxy cidr rejected",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"10.0.0.0/8"}
				return c
			}(),
			wantErr: true,
		},
		{
			name: "overbroad ipv6 trust proxy cidr rejected",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"fc00::/7"}
				return c
			}(),
			wantErr: true,
		},
		{
			name: "broad trust proxy cidrs ignored when proxy trust disabled",
			cfg: func() Config {
				c := base
				c.TrustedProxyCIDRs = []string{"10.0.0.0/8", "192.168.0.0/16"}
				return c
			}(),
			wantErr: false,
		},
		{
			name: "valid trust proxy cidr allowed",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"10.40.64.0/20", "127.0.0.1/32"}
				return c
			}(),
			wantErr: false,
		},
		{
			name: "single proxy ip allowed",
			cfg: func() Config {
				c := base
				c.TrustProxy = true
				c.TrustedProxyCIDRsExplicit = true
				c.TrustedProxyCIDRs = []string{"10.40.64.10"}
				return c
			}(),
			wantErr: false,
		},
		{
			name: "upgrade docker mode rejected",
			cfg: func() Config {
				c := base
				c.UpgradeRunnerMode = "docker"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "upgrade remote mode requires url and token",
			cfg: func() Config {
				c := base
				c.UpgradeRunnerMode = "remote"
				c.UpgradeRunnerURL = ""
				c.UpgradeRunnerToken = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "backup docker mode rejected",
			cfg: func() Config {
				c := base
				c.BackupRunnerMode = "docker"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "backup remote mode requires url and token",
			cfg: func() Config {
				c := base
				c.BackupRunnerMode = "remote"
				c.BackupRunnerURL = ""
				c.BackupRunnerToken = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "remote runner modes accepted with strong config",
			cfg: func() Config {
				c := base
				c.UpgradeRunnerMode = "remote"
				c.UpgradeRunnerURL = "http://maintenance-runner:8090"
				c.UpgradeRunnerToken = "strong-upgrade-runner-token"
				c.BackupRunnerMode = "remote"
				c.BackupRunnerURL = "http://maintenance-runner:8090"
				c.BackupRunnerToken = "strong-backup-runner-token"
				return c
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHardening(tt.cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
