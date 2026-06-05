package certs

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	cpcrypto "github.com/parcel/control-plane/internal/crypto"
)

// setupCA generates a dev CA and writes cert+key PEM files to dir.
// Returns (certPath, keyPath, certPEM).
func setupCA(t *testing.T, dir string) (certPath, keyPath string, certPEM []byte) {
	t.Helper()
	ca, err := cpcrypto.NewDevCA()
	if err != nil {
		t.Fatalf("NewDevCA: %v", err)
	}

	// Encode cert.
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Raw})

	// Encode private key as PKCS#1 (RSA).
	rsaKey, ok := ca.Key.(*rsa.PrivateKey)
	if !ok {
		t.Fatal("expected RSA private key from NewDevCA")
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})

	certPath = filepath.Join(dir, "ca.crt")
	keyPath = filepath.Join(dir, "ca.key")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath, certPEM
}

func TestManager_Load(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := setupCA(t, dir)

	m := NewManager()
	state, err := m.Load(certPath, keyPath, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.Signer == nil {
		t.Error("expected Signer set after Load with cert+key")
	}
	if len(state.ActiveCertPEM) == 0 {
		t.Error("expected non-empty ActiveCertPEM")
	}
	if state.ActiveFingerprint == "" {
		t.Error("expected non-empty ActiveFingerprint")
	}
	if state.ActivePool == nil {
		t.Error("expected ActivePool set")
	}
}

func TestManager_LoadCertOnly(t *testing.T) {
	dir := t.TempDir()
	certPath, _, _ := setupCA(t, dir)

	m := NewManager()
	state, err := m.Load(certPath, "", "")
	if err != nil {
		t.Fatalf("Load cert-only: %v", err)
	}
	if state.Signer != nil {
		t.Error("expected nil Signer when no key file given")
	}
	if state.ActiveFingerprint == "" {
		t.Error("expected fingerprint without key")
	}
}

func TestManager_LoadClientCABundle(t *testing.T) {
	dir := t.TempDir()
	activeCertPath, activeKeyPath, activePEM := setupCA(t, dir)
	_, _, clientPEM := setupCA(t, dir)

	// Bundle: active cert + client CA cert
	bundlePath := filepath.Join(dir, "client-bundle.crt")
	if err := os.WriteFile(bundlePath, append(activePEM, clientPEM...), 0600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	m := NewManager()
	state, err := m.Load(activeCertPath, activeKeyPath, bundlePath)
	if err != nil {
		t.Fatalf("Load with client CA bundle: %v", err)
	}
	if state.ClientPool == nil {
		t.Error("expected ClientPool set")
	}
	if len(state.ClientCerts) < 2 {
		t.Errorf("expected ≥2 client certs in bundle, got %d", len(state.ClientCerts))
	}
	// The active cert is in the bundle, so ClientContainsActive should be true.
	if !state.ClientContainsActive {
		t.Error("expected ClientContainsActive=true")
	}
}

func TestManager_LoadMissingFile(t *testing.T) {
	m := NewManager()
	_, err := m.Load("/nonexistent/ca.crt", "", "")
	if err == nil {
		t.Fatal("expected error for missing cert file")
	}
}

func TestManager_State_FreshIsNil(t *testing.T) {
	m := NewManager()
	state := m.State()
	if state == nil {
		t.Fatal("State() must never return nil")
	}
	if state.Signer != nil {
		t.Error("expected nil Signer on fresh manager")
	}
}

func TestManager_SignDeviceCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := setupCA(t, dir)

	m := NewManager()
	if _, err := m.Load(certPath, keyPath, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}

	devKey, err := cpcrypto.GenerateRSAKey()
	if err != nil {
		t.Fatalf("GenerateRSAKey: %v", err)
	}
	csrPEM, err := cpcrypto.GenerateCSR(devKey, pkix.Name{CommonName: "test-device-id"})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}

	certPEM, fingerprint, err := m.SignDeviceCert(csrPEM, "test-device-id", 24*time.Hour)
	if err != nil {
		t.Fatalf("SignDeviceCert: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("expected non-empty certPEM")
	}
	if fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("issued cert is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse issued cert: %v", err)
	}
	if cert.Subject.CommonName != "test-device-id" {
		t.Errorf("expected CN=test-device-id, got %q", cert.Subject.CommonName)
	}
}

func TestManager_SignDeviceCert_NoSignerReturnsError(t *testing.T) {
	m := NewManager()
	_, _, err := m.SignDeviceCert([]byte("fake-csr"), "device", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error when signer not configured")
	}
}

func TestManager_CACertPEM_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := setupCA(t, dir)

	m := NewManager()
	if _, err := m.Load(certPath, keyPath, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.CACertPEM()) == 0 {
		t.Error("expected non-empty CACertPEM")
	}
}

func TestManager_Reload(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := setupCA(t, dir)

	m := NewManager()
	if _, err := m.Load(certPath, keyPath, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
	fp1 := m.State().ActiveFingerprint

	state2, err := m.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if state2.ActiveFingerprint != fp1 {
		t.Errorf("fingerprint changed after Reload: %s → %s", fp1, state2.ActiveFingerprint)
	}
}

func TestCertFingerprint_Stable(t *testing.T) {
	ca, err := cpcrypto.NewDevCA()
	if err != nil {
		t.Fatalf("NewDevCA: %v", err)
	}
	fp1 := certFingerprint(ca.Cert)
	fp2 := certFingerprint(ca.Cert)
	if fp1 != fp2 {
		t.Error("certFingerprint is not deterministic")
	}
	if fp1 == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestContainsCert(t *testing.T) {
	ca1, _ := cpcrypto.NewDevCA()
	ca2, _ := cpcrypto.NewDevCA()

	fp1 := certFingerprint(ca1.Cert)
	certs := []*x509.Certificate{ca1.Cert}

	if !containsCert(certs, fp1) {
		t.Error("expected containsCert=true for cert in list")
	}
	if containsCert(certs, certFingerprint(ca2.Cert)) {
		t.Error("expected containsCert=false for cert not in list")
	}
	if containsCert(nil, fp1) {
		t.Error("expected containsCert=false for nil list")
	}
}

func TestLoadCerts_NoCertsInFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.pem")
	_ = os.WriteFile(f, []byte("not a cert"), 0600)

	_, _, err := loadCerts(f)
	if err == nil {
		t.Fatal("expected error when no certificates in file")
	}
}

// Ensure m.State() and m.ActivePool()/ClientPool() never panic on zero-value manager.
func TestManager_NilSafe(t *testing.T) {
	var m *Manager
	if m.State() == nil {
		t.Error("nil manager State() must not return nil")
	}
	if m.ActivePool() != nil {
		t.Error("nil manager ActivePool should be nil")
	}
	if m.ClientPool() != nil {
		t.Error("nil manager ClientPool should be nil")
	}
	if len(m.CACertPEM()) != 0 {
		t.Error("nil manager CACertPEM should be empty")
	}
}
