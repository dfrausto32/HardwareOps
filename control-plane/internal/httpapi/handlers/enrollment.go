package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/license"
	"github.com/hardwareops/control-plane/internal/metrics"
	"github.com/hardwareops/control-plane/internal/store"
)

type CreateEnrollmentTokenRequest struct {
	ExpiresInSec int64 `json:"expiresInSec"`
}

type CreateEnrollmentTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type DeviceEnrollRequest struct {
	Token string `json:"token"`
	CSR   string `json:"csr"`
}

type DeviceEnrollResponse struct {
	DeviceID  string `json:"deviceId"`
	CertPEM   string `json:"certPem"`
	CACertPEM string `json:"caCertPem"`
}

const (
	maxEnrollmentTTL = 7 * 24 * time.Hour
	maxCSRSize       = 16 * 1024
	maxCSRAltNames   = 10
	maxCSRCommonName = 128
)

func CreateEnrollmentToken(logger *log.Logger, st store.Store, lic *license.Manager, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status, reason string) {
			if metricsCollector != nil {
				metricsCollector.IncEnrollmentToken(status, reason)
			}
		}
		if err := enforceLicense(st, lic); err != nil {
			status := http.StatusForbidden
			if errors.Is(err, errLicenseLimitExceeded) {
				status = http.StatusForbidden
				record("error", "license_limit")
				http.Error(w, "device limit reached", status)
				return
			}
			if errors.Is(err, errLicenseInvalid) {
				record("error", "license_invalid")
				http.Error(w, "license invalid", status)
				return
			}
			logger.Printf("license enforcement error: %v", err)
			record("error", "license_error")
			http.Error(w, "license enforcement error", http.StatusInternalServerError)
			return
		}
		var req CreateEnrollmentTokenRequest
		if r.Body != nil {
			dec := json.NewDecoder(r.Body)
			if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
				record("error", "bad_request")
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}

		expires := time.Now().UTC().Add(24 * time.Hour)
		if req.ExpiresInSec > 0 {
			if req.ExpiresInSec > int64(maxEnrollmentTTL.Seconds()) {
				record("error", "expires_too_large")
				http.Error(w, "expiresInSec too large", http.StatusBadRequest)
				return
			}
			expires = time.Now().UTC().Add(time.Duration(req.ExpiresInSec) * time.Second)
		}
		if req.ExpiresInSec < 0 {
			record("error", "expires_negative")
			http.Error(w, "expiresInSec must be positive", http.StatusBadRequest)
			return
		}

		token, tokenHash, err := generateToken()
		if err != nil {
			logger.Printf("generate token error: %v", err)
			record("error", "token_error")
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}

		if err := st.CreateEnrollmentToken(tokenHash, expires); err != nil {
			logger.Printf("store token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("token"), "enrollment_token.create", "enrollment_token", ""), err)
			record("error", "storage_error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("token"), "enrollment_token.create", "enrollment_token", "")
		event.MetadataJSON = auditJSON(map[string]any{"expiresAt": expires})
		writeAudit(logger, st, event, nil)

		resp := CreateEnrollmentTokenResponse{Token: token, ExpiresAt: expires}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		record("success", "ok")
	}
}

func DeviceEnroll(logger *log.Logger, st store.Store, lic *license.Manager, signer interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error)
	CACertPEM() []byte
}, trustProxy bool, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := func(status, reason string) {
			if metricsCollector != nil {
				metricsCollector.IncEnroll(status, reason)
			}
		}
		maxDevices, err := enrollmentMaxDevices(lic)
		if err != nil {
			record("error", "license_invalid")
			http.Error(w, "license invalid", http.StatusForbidden)
			return
		}
		if signer == nil {
			record("error", "signer_missing")
			http.Error(w, "signer not configured", http.StatusInternalServerError)
			return
		}

		var req DeviceEnrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			record("error", "bad_request")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Token == "" || req.CSR == "" {
			record("error", "bad_request")
			http.Error(w, "token and csr required", http.StatusBadRequest)
			return
		}
		if len(req.CSR) > maxCSRSize {
			record("error", "csr_too_large")
			http.Error(w, "csr too large", http.StatusBadRequest)
			return
		}
		if err := validateCSR([]byte(req.CSR)); err != nil {
			logger.Printf("csr validation error: %v", err)
			record("error", "csr_invalid")
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		deviceID := uuid.NewString()
		certPEM, fingerprint, err := signer.SignDeviceCert([]byte(req.CSR), deviceID, 365*24*time.Hour)
		if err != nil {
			logger.Printf("sign csr error: %v", err)
			record("error", "sign_error")
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		meta := []byte(nil)
		if caFingerprint, _, err := caFingerprintFromPEM(signer.CACertPEM()); err == nil {
			meta = updateCertMeta(nil, map[string]any{
				"active":        true,
				"caFingerprint": caFingerprint,
				"checkedAt":     time.Now().UTC().Format(time.RFC3339),
			})
		}
		hash := hashToken(req.Token)
		if err := st.EnrollDeviceWithToken(hash, store.Device{
			DeviceID:        deviceID,
			CertFingerprint: fingerprint,
			Status:          "active",
			LastSeen:        time.Now().UTC(),
			MetadataJSON:    meta,
		}, maxDevices); err != nil {
			if errors.Is(err, store.ErrEnrollmentTokenInvalid) {
				record("error", "token_invalid")
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			if errors.Is(err, store.ErrDeviceLimitExceeded) {
				record("error", "license_limit")
				http.Error(w, "device limit reached", http.StatusForbidden)
				return
			}
			logger.Printf("enroll device store error: %v", err)
			record("error", "storage_error")
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		logger.Printf("enroll device=%s fingerprint=%s ip=%s", deviceID, fingerprint, clientIP(r, trustProxy))

		resp := DeviceEnrollResponse{
			DeviceID:  deviceID,
			CertPEM:   string(certPEM),
			CACertPEM: string(signer.CACertPEM()),
		}
		event := buildAuditEvent(r, trustProxy, AuditActor{Type: "device", ID: deviceID, AuthMethod: "enrollment_token"}, "device.enroll", "device", deviceID)
		event.MetadataJSON = auditJSON(map[string]any{"fingerprint": fingerprint})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		record("success", "ok")
	}
}

func generateToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmtHex(h[:])
}

func fmtHex(b []byte) string {
	const hextable = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hextable[v>>4]
		out[i*2+1] = hextable[v&0x0f]
	}
	return string(out)
}

func validateCSR(csrPEM []byte) error {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return errors.New("invalid csr pem")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return err
	}
	if err := csr.CheckSignature(); err != nil {
		return err
	}

	cn := strings.TrimSpace(csr.Subject.CommonName)
	if cn == "" {
		return errors.New("csr subject common name required")
	}
	if len(cn) > maxCSRCommonName {
		return errors.New("csr subject common name too long")
	}
	if strings.Contains(cn, "*") {
		return errors.New("csr subject common name must not contain wildcard")
	}

	if len(csr.URIs) > 0 {
		return errors.New("csr uri SANs not allowed")
	}
	if len(csr.EmailAddresses) > 0 {
		return errors.New("csr email SANs not allowed")
	}
	if len(csr.DNSNames) > maxCSRAltNames {
		return errors.New("too many dns SANs")
	}
	for _, name := range csr.DNSNames {
		if name == "" {
			return errors.New("dns SAN empty")
		}
		if strings.Contains(name, "*") {
			return errors.New("dns SAN wildcard not allowed")
		}
		if !isDNSName(name) {
			return errors.New("dns SAN invalid")
		}
	}
	if len(csr.IPAddresses) > maxCSRAltNames {
		return errors.New("too many ip SANs")
	}
	return nil
}

func isDNSName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return false
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func clientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); ip != "" {
					return ip
				}
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
