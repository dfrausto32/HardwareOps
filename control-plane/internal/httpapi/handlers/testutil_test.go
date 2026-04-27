package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

func withURLParam(req *http.Request, key, val string) *http.Request {
	routeCtx, _ := req.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if routeCtx == nil {
		routeCtx = chi.NewRouteContext()
	}
	routeCtx.URLParams.Add(key, val)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	return req.WithContext(ctx)
}

func attachMTLSDevice(t *testing.T, mem *memory.Store, req *http.Request, deviceID string) (*http.Request, string) {
	t.Helper()

	if deviceID == "" {
		deviceID = uuid.NewString()
	}

	certDER, fingerprint := newTestCert(t, deviceID)
	if err := mem.CreateDevice(store.Device{
		DeviceID:        deviceID,
		CertFingerprint: fingerprint,
		Status:          "active",
		LastSeen:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}

	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{certDER},
	}

	return req, deviceID
}

func attachExistingMTLSDevice(t *testing.T, mem *memory.Store, req *http.Request, deviceID string) *http.Request {
	t.Helper()

	certDER, fingerprint := newTestCert(t, deviceID)
	device, ok, err := mem.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if !ok {
		t.Fatalf("device %s not found", deviceID)
	}
	device.CertFingerprint = fingerprint
	if err := mem.UpsertDevice(device); err != nil {
		t.Fatalf("update device fingerprint: %v", err)
	}

	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{certDER},
	}

	return req
}

func newTestCert(t *testing.T, commonName string) (*x509.Certificate, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return cert, hex.EncodeToString(sum[:])
}
