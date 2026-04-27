package artifacttrust

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/parcel/control-plane/internal/store/memory"
)

func TestLoadBootstrapTrustedSigningKeys(t *testing.T) {
	pubPEM, keyID := testEd25519PublicKey(t)
	keys, err := LoadBootstrapTrustedSigningKeys("", `{"keys":[{"displayName":"demo","algorithm":"ed25519","publicKeyPem":`+quoteJSON(pubPEM)+`}]}`)
	if err != nil {
		t.Fatalf("LoadBootstrapTrustedSigningKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].KeyID != keyID {
		t.Fatalf("expected computed key id %s, got %s", keyID, keys[0].KeyID)
	}
	if keys[0].State != KeyStateActive {
		t.Fatalf("expected active key state, got %s", keys[0].State)
	}
}

func TestSeedBootstrapStateSeedsPolicyAndKeys(t *testing.T) {
	pubPEM, keyID := testEd25519PublicKey(t)
	mem := memory.New()
	policy, keys, err := SeedBootstrapState(context.Background(), mem, BootstrapConfig{
		StaticJSON: `{"keys":[{"keyId":"` + keyID + `","displayName":"demo","algorithm":"ed25519","publicKeyPem":` + quoteJSON(pubPEM) + `}]}`,
		DefaultPolicy: DefaultPolicyConfig{
			VerificationMode:      VerificationModeRequire,
			AllowedSigningKeyIDs:  []string{keyID},
			AllowedSignatureTypes: []string{SignatureTypeEd25519},
		},
	})
	if err != nil {
		t.Fatalf("SeedBootstrapState() error = %v", err)
	}
	if policy.VerificationMode != VerificationModeRequire {
		t.Fatalf("expected strict policy, got %s", policy.VerificationMode)
	}
	if len(keys) != 1 {
		t.Fatalf("expected seeded key, got %d", len(keys))
	}
	if saved, ok, err := mem.GetTrustedSigningKey(keyID); err != nil || !ok {
		t.Fatalf("expected trusted key in store, ok=%v err=%v", ok, err)
	} else if !strings.Contains(saved.PublicKeyPEM, "BEGIN PUBLIC KEY") {
		t.Fatalf("unexpected stored public key")
	}
}

func TestSeedBootstrapStateRejectsStrictPolicyWithoutKeys(t *testing.T) {
	mem := memory.New()
	_, _, err := SeedBootstrapState(context.Background(), mem, BootstrapConfig{
		DefaultPolicy: DefaultPolicyConfig{
			VerificationMode: VerificationModeRequire,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires at least one active trusted signing key") {
		t.Fatalf("expected strict policy bootstrap error, got %v", err)
	}
}

func testEd25519PublicKey(t *testing.T) (string, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	keyID, err := ComputeKeyIDFromPublicKeyPEM(string(pemBytes))
	if err != nil {
		t.Fatalf("ComputeKeyIDFromPublicKeyPEM() error = %v", err)
	}
	return string(pemBytes), keyID
}

func quoteJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
