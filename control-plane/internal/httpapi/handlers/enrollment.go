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

func CreateEnrollmentToken(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateEnrollmentTokenRequest
		if r.Body != nil {
			dec := json.NewDecoder(r.Body)
			if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		}

		expires := time.Now().UTC().Add(24 * time.Hour)
		if req.ExpiresInSec > 0 {
			if req.ExpiresInSec > int64(maxEnrollmentTTL.Seconds()) {
				http.Error(w, "expiresInSec too large", http.StatusBadRequest)
				return
			}
			expires = time.Now().UTC().Add(time.Duration(req.ExpiresInSec) * time.Second)
		}
		if req.ExpiresInSec < 0 {
			http.Error(w, "expiresInSec must be positive", http.StatusBadRequest)
			return
		}

		token, tokenHash, err := generateToken()
		if err != nil {
			logger.Printf("generate token error: %v", err)
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}

		if err := st.CreateEnrollmentToken(tokenHash, expires); err != nil {
			logger.Printf("store token error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := CreateEnrollmentTokenResponse{Token: token, ExpiresAt: expires}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeviceEnroll(logger *log.Logger, st store.Store, signer interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error)
	CACertPEM() []byte
}, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if signer == nil {
			http.Error(w, "signer not configured", http.StatusInternalServerError)
			return
		}

		var req DeviceEnrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Token == "" || req.CSR == "" {
			http.Error(w, "token and csr required", http.StatusBadRequest)
			return
		}
		if len(req.CSR) > maxCSRSize {
			http.Error(w, "csr too large", http.StatusBadRequest)
			return
		}
		if err := validateCSR([]byte(req.CSR)); err != nil {
			logger.Printf("csr validation error: %v", err)
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		hash := hashToken(req.Token)
		ok, err := st.ConsumeEnrollmentToken(hash)
		if err != nil {
			logger.Printf("consume token error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		deviceID := uuid.NewString()
		certPEM, fingerprint, err := signer.SignDeviceCert([]byte(req.CSR), deviceID, 365*24*time.Hour)
		if err != nil {
			logger.Printf("sign csr error: %v", err)
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		if err := st.CreateDevice(store.Device{
			DeviceID:        deviceID,
			CertFingerprint: fingerprint,
			Status:          "active",
			LastSeen:        time.Now().UTC(),
		}); err != nil {
			logger.Printf("create device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		logger.Printf("enroll device=%s fingerprint=%s ip=%s", deviceID, fingerprint, clientIP(r, trustProxy))

		resp := DeviceEnrollResponse{
			DeviceID:  deviceID,
			CertPEM:   string(certPEM),
			CACertPEM: string(signer.CACertPEM()),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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
