package config

import "testing"

func TestValidateHardening(t *testing.T) {
	base := Config{
		HardenedProfile:                true,
		AuthMode:                       "local",
		LicenseEnforce:                 true,
		LicensePath:                    "/opt/hardwareops/license.json",
		DeviceIdentityMode:             "enforce",
		DeviceIdentityRequireOnEnroll:  true,
		DeviceIdentityRequireOnCheckin: true,
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
