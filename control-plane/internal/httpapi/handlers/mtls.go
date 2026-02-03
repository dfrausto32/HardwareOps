package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/hardwareops/control-plane/internal/store"
)

var (
	errClientCertRequired = errors.New("client certificate required")
	errUnknownDeviceCert  = errors.New("unknown device certificate")
)

func deviceFromMTLS(r *http.Request, st store.Store, trustProxy bool, header string) (store.Device, error) {
	cert := peerCertFromRequest(r, trustProxy, header)
	if cert == nil {
		return store.Device{}, errClientCertRequired
	}
	hash := sha256.Sum256(cert.Raw)
	fingerprint := hex.EncodeToString(hash[:])

	device, ok, err := st.GetDeviceByFingerprint(fingerprint)
	if err != nil {
		return store.Device{}, err
	}
	if !ok {
		return store.Device{}, errUnknownDeviceCert
	}
	return device, nil
}

func peerCertFromRequest(r *http.Request, trustProxy bool, header string) *x509.Certificate {
	if r != nil && r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		return r.TLS.PeerCertificates[0]
	}
	if !trustProxy || r == nil {
		return nil
	}
	if header == "" {
		header = "X-Client-Cert"
	}
	val := strings.TrimSpace(r.Header.Get(header))
	if val == "" {
		return nil
	}
	if unesc, err := url.QueryUnescape(val); err == nil {
		val = unesc
	}
	val = strings.ReplaceAll(val, "\\n", "\n")
	val = strings.ReplaceAll(val, "\r", "")
	block, _ := pem.Decode([]byte(val))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return cert
}
