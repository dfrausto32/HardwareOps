package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func TestDeviceFromMTLS_ProxyHeaderAllowedByCIDR(t *testing.T) {
	ConfigureTrustedProxyCIDRs([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ConfigureTrustedProxyCIDRs(nil) })

	mem := memory.New()
	cert := mustCertForMTLS(t, uuid.NewString())
	fingerprint := certFingerprint(cert)
	deviceID := uuid.NewString()
	if err := mem.CreateDevice(store.Device{
		DeviceID:        deviceID,
		CertFingerprint: fingerprint,
		Status:          "active",
		LastSeen:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/devices/checkin", nil)
	req.RemoteAddr = "10.2.3.4:43210"
	req.Header.Set("X-Client-Cert", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})))

	device, err := deviceFromMTLS(req, mem, true, "X-Client-Cert")
	if err != nil {
		t.Fatalf("deviceFromMTLS: %v", err)
	}
	if device.DeviceID != deviceID {
		t.Fatalf("expected device %s, got %s", deviceID, device.DeviceID)
	}
}

func TestDeviceFromMTLS_ProxyHeaderRejectedFromUntrustedRemote(t *testing.T) {
	ConfigureTrustedProxyCIDRs([]string{"10.0.0.0/8"})
	t.Cleanup(func() { ConfigureTrustedProxyCIDRs(nil) })

	mem := memory.New()
	cert := mustCertForMTLS(t, uuid.NewString())
	fingerprint := certFingerprint(cert)
	deviceID := uuid.NewString()
	if err := mem.CreateDevice(store.Device{
		DeviceID:        deviceID,
		CertFingerprint: fingerprint,
		Status:          "active",
		LastSeen:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/devices/checkin", nil)
	req.RemoteAddr = "8.8.8.8:5555"
	req.Header.Set("X-Client-Cert", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})))

	_, err := deviceFromMTLS(req, mem, true, "X-Client-Cert")
	if err != errClientCertRequired {
		t.Fatalf("expected errClientCertRequired, got %v", err)
	}
}

func mustCertForMTLS(t *testing.T, commonName string) *x509.Certificate {
	t.Helper()
	cert, _ := newTestCert(t, commonName)
	return cert
}

func certFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
