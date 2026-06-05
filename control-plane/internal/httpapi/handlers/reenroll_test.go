package handlers

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cpcrypto "github.com/parcel/control-plane/internal/crypto"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// generateTestCSR creates a fresh RSA key and returns a PEM-encoded CSR for
// the given Common Name. Shared with reenroll tests.
func generateTestCSR(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := cpcrypto.GenerateRSAKey()
	if err != nil {
		t.Fatalf("GenerateRSAKey: %v", err)
	}
	csrPEM, err := cpcrypto.GenerateCSR(key, pkix.Name{CommonName: cn})
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}
	return csrPEM
}

func TestDeviceReenroll_NoClientCert(t *testing.T) {
	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/reenroll", strings.NewReader(`{"csr":"x"}`))
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), memory.New(), signer, false, "").ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without mTLS cert, got %d", w.Code)
	}
}

func TestDeviceReenroll_NilSigner(t *testing.T) {
	mem := memory.New()
	req := httptest.NewRequest(http.MethodPost, "/reenroll", strings.NewReader(`{"csr":"x"}`))
	req, deviceID := attachMTLSDevice(t, mem, req, "")
	_ = deviceID
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, nil, false, "").ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when signer is nil, got %d", w.Code)
	}
}

func TestDeviceReenroll_MissingCSR(t *testing.T) {
	mem := memory.New()
	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/reenroll", strings.NewReader(`{}`))
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, signer, false, "").ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing CSR, got %d", w.Code)
	}
}

func TestDeviceReenroll_InvalidCSR(t *testing.T) {
	mem := memory.New()
	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	body, _ := json.Marshal(DeviceReenrollRequest{CSR: "not-a-csr"})
	req := httptest.NewRequest(http.MethodPost, "/reenroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, signer, false, "").ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid CSR, got %d", w.Code)
	}
}

func TestDeviceReenroll_SignerError(t *testing.T) {
	mem := memory.New()
	signer := &fakeSigner{err: errors.New("crypto error")}
	csrPEM := generateTestCSR(t, "test-device")
	body, _ := json.Marshal(DeviceReenrollRequest{CSR: string(csrPEM)})
	req := httptest.NewRequest(http.MethodPost, "/reenroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req, _ = attachMTLSDevice(t, mem, req, "")
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, signer, false, "").ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when signer returns error, got %d", w.Code)
	}
}

func TestDeviceReenroll_Success(t *testing.T) {
	mem := memory.New()
	deviceID := "device-abc"

	// Build a real signed cert PEM to return from the signer.
	certPEM := realIssuedCertPEM(t, deviceID)
	signer := &fakeSigner{
		certPEM: certPEM,
		caPEM:   []byte("-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n"),
	}

	csrPEM := generateTestCSR(t, deviceID)
	body, _ := json.Marshal(DeviceReenrollRequest{CSR: string(csrPEM)})
	req := httptest.NewRequest(http.MethodPost, "/reenroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, signer, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DeviceReenrollResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DeviceID != deviceID {
		t.Errorf("expected deviceId=%s, got %s", deviceID, resp.DeviceID)
	}
	if resp.CertPEM == "" {
		t.Error("expected non-empty certPEM")
	}

	// Device fingerprint should be updated in store.
	dev, ok, err := mem.GetDevice(deviceID)
	if err != nil || !ok {
		t.Fatalf("get device after reenroll: ok=%t err=%v", ok, err)
	}
	if dev.CertFingerprint == "" {
		t.Error("expected device CertFingerprint to be updated")
	}
}

func TestDeviceReenroll_DeviceUpdatedInStore(t *testing.T) {
	mem := memory.New()
	deviceID := "device-reenroll-store"
	certPEM := realIssuedCertPEM(t, deviceID)
	signer := &fakeSigner{
		certPEM: certPEM,
		caPEM:   []byte("-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n"),
	}

	csrPEM := generateTestCSR(t, deviceID)
	body, _ := json.Marshal(DeviceReenrollRequest{CSR: string(csrPEM)})
	req := httptest.NewRequest(http.MethodPost, "/reenroll", bytes.NewReader(body))
	req, _ = attachMTLSDevice(t, mem, req, deviceID)
	w := httptest.NewRecorder()
	DeviceReenroll(log.New(&bytes.Buffer{}, "", 0), mem, signer, false, "").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Verify the store was updated with the new fingerprint.
	dev, ok, _ := mem.GetDevice(deviceID)
	if !ok {
		t.Fatal("device not found after reenroll")
	}
	if dev.CertFingerprint != "fingerprint" { // fakeSigner always returns "fingerprint"
		t.Errorf("expected fingerprint=fingerprint, got %q", dev.CertFingerprint)
	}
}

// realIssuedCertPEM returns a self-signed cert PEM usable as fakeSigner output.
// certSerialFromPEM requires valid PEM; we generate a minimal self-signed cert.
func realIssuedCertPEM(t *testing.T, cn string) []byte {
	t.Helper()
	cert, _ := newTestCert(t, cn)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// Compile-time check: store.Device is used here to ensure the import is needed.
var _ store.Device
