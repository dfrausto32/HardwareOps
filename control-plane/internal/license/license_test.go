package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// genKey returns a fresh ed25519 key pair: public key base64 and private key.
func genKey(t *testing.T) (pubB64 string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

// signedLicenseFile creates a license file signed with priv and writes it to dir.
// Returns the file path.
func signedLicenseFile(t *testing.T, dir string, payload Payload, priv ed25519.PrivateKey) string {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sig := ed25519.Sign(priv, payloadBytes)
	lf := File{
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
	raw, _ := json.Marshal(lf)
	path := filepath.Join(dir, "license.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("write license: %v", err)
	}
	return path
}

func TestLicense_ValidLicense(t *testing.T) {
	dir := t.TempDir()
	pubB64, priv := genKey(t)
	path := signedLicenseFile(t, dir, Payload{IssuedTo: "Acme", MaxDevices: 100}, priv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if !info.Valid {
		t.Errorf("expected valid license, error: %s", info.Error)
	}
	if !info.Enabled {
		t.Error("expected Enabled=true")
	}
	if info.Payload.MaxDevices != 100 {
		t.Errorf("expected MaxDevices=100, got %d", info.Payload.MaxDevices)
	}
	if info.Payload.IssuedTo != "Acme" {
		t.Errorf("expected IssuedTo=Acme, got %q", info.Payload.IssuedTo)
	}
}

func TestLicense_InvalidSignature(t *testing.T) {
	dir := t.TempDir()
	pubB64, _ := genKey(t)
	_, wrongPriv := genKey(t) // signed with different key
	path := signedLicenseFile(t, dir, Payload{IssuedTo: "Acme", MaxDevices: 50}, wrongPriv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license when signature is from wrong key")
	}
}

func TestLicense_Expired(t *testing.T) {
	dir := t.TempDir()
	pubB64, priv := genKey(t)
	past := time.Now().Add(-24 * time.Hour)
	path := signedLicenseFile(t, dir, Payload{MaxDevices: 10, ExpiresAt: &past}, priv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license for expired license")
	}
}

func TestLicense_NotYetValid(t *testing.T) {
	dir := t.TempDir()
	pubB64, priv := genKey(t)
	future := time.Now().Add(24 * time.Hour)
	path := signedLicenseFile(t, dir, Payload{MaxDevices: 10, NotBefore: &future}, priv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license for not-yet-valid license")
	}
}

func TestLicense_ZeroMaxDevices(t *testing.T) {
	dir := t.TempDir()
	pubB64, priv := genKey(t)
	path := signedLicenseFile(t, dir, Payload{MaxDevices: 0}, priv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license when MaxDevices=0")
	}
}

func TestLicense_MissingFile(t *testing.T) {
	pubB64, _ := genKey(t)
	mgr, err := NewManager("/nonexistent/license.json", pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager (non-enforce): %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license when file missing")
	}
}

func TestLicense_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	pubB64, _ := genKey(t)
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0600)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if info.Valid {
		t.Error("expected invalid license for malformed JSON")
	}
}

func TestLicense_Disabled(t *testing.T) {
	// When enforce=false, Manager.Enabled() is false and Snapshot returns Enabled=false.
	mgr, err := NewManager("", "", "", false, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.Enabled() {
		t.Error("expected Enabled()=false when enforce=false")
	}
	info := mgr.Snapshot()
	if info.Enabled {
		t.Error("expected Snapshot().Enabled=false when enforce=false")
	}
}

func TestLicense_NilManager(t *testing.T) {
	var m *Manager
	if m.Enabled() {
		t.Error("nil manager should not be Enabled")
	}
	info := m.Snapshot()
	if info.Enabled {
		t.Error("nil manager Snapshot should not be Enabled")
	}
}

func TestLicense_ValidWithExpiryInFuture(t *testing.T) {
	dir := t.TempDir()
	pubB64, priv := genKey(t)
	future := time.Now().Add(365 * 24 * time.Hour)
	path := signedLicenseFile(t, dir, Payload{MaxDevices: 50, ExpiresAt: &future}, priv)

	mgr, err := NewManager(path, pubB64, "", true, 0)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info := mgr.Snapshot()
	if !info.Valid {
		t.Errorf("expected valid license with future expiry, got: %s", info.Error)
	}
}

func TestLicense_InvalidPublicKey(t *testing.T) {
	dir := t.TempDir()
	_, priv := genKey(t)
	path := signedLicenseFile(t, dir, Payload{MaxDevices: 10}, priv)

	// Supplying a bad public key with enforce=true should fail at NewManager.
	_, err := NewManager(path, "not-valid-base64!!!", "", true, 0)
	if err == nil {
		t.Fatal("expected error for invalid public key when enforce=true")
	}
}

func TestParsePublicKey_Base64(t *testing.T) {
	pubB64, _ := genKey(t)
	pub, err := parsePublicKey(pubB64)
	if err != nil {
		t.Fatalf("parsePublicKey: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("expected %d bytes, got %d", ed25519.PublicKeySize, len(pub))
	}
}

func TestParsePublicKey_Empty(t *testing.T) {
	_, err := parsePublicKey("")
	if err == nil {
		t.Fatal("expected error for empty public key")
	}
}
