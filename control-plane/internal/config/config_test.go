package config

import (
	"os"
	"reflect"
	"testing"
)

func TestFromEnvTrustedProxyCIDRs(t *testing.T) {
	t.Run("uses built-in defaults when unset", func(t *testing.T) {
		unsetEnv(t, "TRUST_PROXY_CIDRS")

		cfg := FromEnv()

		if cfg.TrustedProxyCIDRsExplicit {
			t.Fatalf("expected TRUST_PROXY_CIDRS to be implicit when unset")
		}
		want := []string{
			"127.0.0.1/32",
			"::1/128",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"100.64.0.0/10",
			"fc00::/7",
			"fe80::/10",
		}
		if !reflect.DeepEqual(cfg.TrustedProxyCIDRs, want) {
			t.Fatalf("unexpected default TRUST_PROXY_CIDRS: got %v want %v", cfg.TrustedProxyCIDRs, want)
		}
	})

	t.Run("treats empty string as explicit empty list", func(t *testing.T) {
		t.Setenv("TRUST_PROXY_CIDRS", "")

		cfg := FromEnv()

		if !cfg.TrustedProxyCIDRsExplicit {
			t.Fatalf("expected TRUST_PROXY_CIDRS to be explicit when set")
		}
		if len(cfg.TrustedProxyCIDRs) != 0 {
			t.Fatalf("expected empty TRUST_PROXY_CIDRS, got %v", cfg.TrustedProxyCIDRs)
		}
	})

	t.Run("parses explicit values", func(t *testing.T) {
		t.Setenv("TRUST_PROXY_CIDRS", "10.40.64.0/20, 10.40.80.5")

		cfg := FromEnv()

		if !cfg.TrustedProxyCIDRsExplicit {
			t.Fatalf("expected TRUST_PROXY_CIDRS to be explicit when set")
		}
		want := []string{"10.40.64.0/20", "10.40.80.5"}
		if !reflect.DeepEqual(cfg.TrustedProxyCIDRs, want) {
			t.Fatalf("unexpected TRUST_PROXY_CIDRS: got %v want %v", cfg.TrustedProxyCIDRs, want)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if !ok {
			_ = os.Unsetenv(key)
			return
		}
		_ = os.Setenv(key, value)
	})
}
