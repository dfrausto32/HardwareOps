package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

type fakeSigner struct {
	certPEM []byte
	caPEM   []byte
	err     error
}

func (f *fakeSigner) SignDeviceCert(_ []byte, _ string, _ time.Duration) ([]byte, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	return f.certPEM, "fingerprint", nil
}

func (f *fakeSigner) CACertPEM() []byte {
	return f.caPEM
}

func TestCreateEnrollmentToken(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", bytes.NewReader([]byte(`{"expiresInSec":3600}`)))
	w := httptest.NewRecorder()

	CreateEnrollmentToken(logger, mem, nil, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp CreateEnrollmentTokenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.Token == "" || resp.ExpiresAt.IsZero() {
		t.Fatalf("expected token and expiresAt")
	}
}

func TestDeviceEnroll_Valid(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))

	csrPEM := mustCSR(t)
	payload := DeviceEnrollRequest{Token: token, CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp DeviceEnrollResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp.DeviceID == "" || resp.CertPEM == "" || resp.CACertPEM == "" {
		t.Fatalf("missing enrollment response fields")
	}
	if _, ok, _ := mem.GetDevice(resp.DeviceID); !ok {
		t.Fatalf("device not created")
	}
}

func TestDeviceEnroll_InvalidToken(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	csrPEM := mustCSR(t)
	payload := DeviceEnrollRequest{Token: "bad", CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDeviceEnroll_InvalidCSR(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))
	payload := DeviceEnrollRequest{Token: token, CSR: "not a pem"}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{err: errors.New("bad csr")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeviceEnroll_SignerMissing(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	csrPEM := mustCSR(t)
	payload := DeviceEnrollRequest{Token: "tok", CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, nil, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateEnrollmentToken_InvalidJSON(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", bytes.NewReader([]byte(`{bad-json`)))
	w := httptest.NewRecorder()

	CreateEnrollmentToken(logger, mem, nil, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateEnrollmentToken_TooLarge(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", bytes.NewReader([]byte(`{"expiresInSec":999999999}`)))
	w := httptest.NewRecorder()

	CreateEnrollmentToken(logger, mem, nil, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func mustCSR(t *testing.T) []byte {
	return mustCSRWithSubject(t, pkix.Name{CommonName: "device"}, nil)
}

func mustCSRWithSubject(t *testing.T, subj pkix.Name, dns []string) []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key gen: %v", err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  subj,
		DNSNames: dns,
	}, key)
	if err != nil {
		t.Fatalf("csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

func TestDeviceEnroll_RejectsEmptyCN(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))

	csrPEM := mustCSRWithSubject(t, pkix.Name{CommonName: ""}, nil)
	payload := DeviceEnrollRequest{Token: token, CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeviceEnroll_RejectsWildcardSAN(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))

	csrPEM := mustCSRWithSubject(t, pkix.Name{CommonName: "device"}, []string{"*.example.com"})
	payload := DeviceEnrollRequest{Token: token, CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{}, false, nil).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeviceEnroll_RequiresHardwareIdentityWhenConfigured(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))
	csrPEM := mustCSR(t)
	payload := DeviceEnrollRequest{Token: token, CSR: string(csrPEM)}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{Mode: "enforce", RequireOnEnroll: true}, false, nil).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeviceEnroll_RejectsHardwareIdentityReuse(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)
	mem := memory.New()
	hardwareID := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if err := mem.CreateDevice(store.Device{
		DeviceID:     "existing-device",
		Status:       "active",
		LastSeen:     time.Now().UTC(),
		MetadataJSON: []byte(`{"hwops":{"identity":{"hardwareId":"` + hardwareID + `"}}}`),
	}); err != nil {
		t.Fatalf("seed device failed: %v", err)
	}

	token := "test-token"
	_ = mem.CreateEnrollmentToken(hashToken(token), time.Now().UTC().Add(1*time.Hour))
	csrPEM := mustCSR(t)
	payload := DeviceEnrollRequest{
		Token: token,
		CSR:   string(csrPEM),
		Capabilities: json.RawMessage(`{
			"hw": {"identity": {"id": "` + hardwareID + `", "source": "machine-id"}}
		}`),
	}
	body, _ := json.Marshal(payload)

	signer := &fakeSigner{certPEM: []byte("CERT"), caPEM: []byte("CA")}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(body))
	w := httptest.NewRecorder()

	DeviceEnroll(logger, mem, nil, signer, DeviceIdentityPolicy{Mode: "enforce"}, false, nil).ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestClientIP_TrustProxyRequiresAllowedRemote(t *testing.T) {
	ConfigureTrustedProxyCIDRs([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ConfigureTrustedProxyCIDRs(nil) })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip := clientIP(req, true)
	if ip != "8.8.8.8" {
		t.Fatalf("expected remote IP for untrusted proxy, got %s", ip)
	}

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "10.1.2.3:9999"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	ip = clientIP(req, true)
	if ip != "1.2.3.4" {
		t.Fatalf("expected forwarded client IP for trusted proxy, got %s", ip)
	}
}
