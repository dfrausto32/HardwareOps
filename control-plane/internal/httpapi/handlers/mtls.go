package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
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
	// Check cert serial revocation before fingerprint lookup.
	if cert.SerialNumber != nil {
		serial := cert.SerialNumber.Text(16)
		if serial != "" {
			if revoked, err := st.IsCertSerialRevoked(serial); err == nil && revoked {
				return store.Device{}, errUnknownDeviceCert
			}
		}
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
	if !trustProxy || r == nil || !proxyHeadersAllowed(r) {
		return nil
	}
	if header == "" {
		header = "X-Client-Cert"
	}
	val := strings.TrimSpace(r.Header.Get(header))
	if val == "" {
		return nil
	}
	if cert := parseCertPEM(val); cert != nil {
		return cert
	}
	if unesc, err := url.PathUnescape(val); err == nil {
		if cert := parseCertPEM(unesc); cert != nil {
			return cert
		}
	}
	if unesc, err := url.QueryUnescape(val); err == nil {
		if cert := parseCertPEM(unesc); cert != nil {
			return cert
		}
	}
	return nil
}

func parseCertPEM(raw string) *x509.Certificate {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "")
	data := []byte(raw)
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				certs = append(certs, cert)
			}
		}
		data = rest
	}
	if len(certs) > 0 {
		for _, cert := range certs {
			if cert != nil && !cert.IsCA {
				return cert
			}
		}
		return certs[0]
	}
	compacted := strings.ReplaceAll(raw, "\n", "")
	compacted = strings.ReplaceAll(compacted, " ", "+")
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		der, err := enc.DecodeString(compacted)
		if err != nil {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err == nil {
			return cert
		}
	}
	return nil
}
