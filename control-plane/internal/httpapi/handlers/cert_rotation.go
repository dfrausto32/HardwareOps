package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/store"
)

type CertRotationStatus struct {
	ActiveCA     CAInfo               `json:"activeCa"`
	ClientCA     CAInfo               `json:"clientCa"`
	DeviceCounts RotationDeviceCounts `json:"deviceCounts"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	Errors       []string             `json:"errors,omitempty"`
}

type CAInfo struct {
	Path           string    `json:"path,omitempty"`
	Subject        string    `json:"subject,omitempty"`
	Fingerprint    string    `json:"fingerprint,omitempty"`
	NotBefore      time.Time `json:"notBefore,omitempty"`
	NotAfter       time.Time `json:"notAfter,omitempty"`
	CertCount      int       `json:"certCount,omitempty"`
	ContainsActive bool      `json:"containsActive,omitempty"`
}

type RotationDeviceCounts struct {
	Total         int `json:"total"`
	Active        int `json:"active"`
	NeedsReenroll int `json:"needsReenroll"`
	Unknown       int `json:"unknown"`
}

func GetCertRotationStatus(logger *log.Logger, st store.Store, mgr *certs.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := buildRotationStatus(logger, st, mgr)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func ReloadCertRotation(logger *log.Logger, st store.Store, mgr *certs.Manager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}
		if _, err := mgr.Reload(); err != nil {
			logger.Printf("cert reload error: %v", err)
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "certs.reload", "certs", "")
		writeAudit(logger, st, event, nil)
		status := buildRotationStatus(logger, st, mgr)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func RotateCertRotation(logger *log.Logger, st store.Store, mgr *certs.Manager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}
		state := mgr.State()
		if state.ActiveCertPath == "" || state.ActiveKeyPath == "" {
			http.Error(w, "ACTIVE_CA_CERT_PATH/ACTIVE_CA_KEY_PATH not set", http.StatusBadRequest)
			return
		}
		if state.ClientCAPath == "" {
			http.Error(w, "CA_BUNDLE_PATH not set", http.StatusBadRequest)
			return
		}
		if state.ActiveCertPEM == nil || len(state.ActiveCertPEM) == 0 {
			http.Error(w, "active CA cert not loaded", http.StatusBadRequest)
			return
		}
		if samePath(state.ActiveCertPath, state.ClientCAPath) {
			http.Error(w, "CA_BUNDLE_PATH must be different from ACTIVE_CA_CERT_PATH", http.StatusBadRequest)
			return
		}

		newCertPEM, newKeyPEM, err := generateRotationCA(state.ActiveCert)
		if err != nil {
			logger.Printf("cert rotate generate error: %v", err)
			http.Error(w, fmt.Sprintf("generate CA failed: %v", err), http.StatusInternalServerError)
			return
		}

		if err := writeFileSecure(state.ActiveCertPath, newCertPEM, 0644); err != nil {
			logger.Printf("cert rotate write cert error: %v", err)
			http.Error(w, fmt.Sprintf("write active CA cert failed: %v", err), http.StatusInternalServerError)
			return
		}
		if err := writeFileSecure(state.ActiveKeyPath, newKeyPEM, 0600); err != nil {
			logger.Printf("cert rotate write key error: %v", err)
			http.Error(w, fmt.Sprintf("write active CA key failed: %v", err), http.StatusInternalServerError)
			return
		}

		bundle := bytes.NewBuffer(nil)
		bundle.Write(state.ActiveCertPEM)
		if !bytes.HasSuffix(state.ActiveCertPEM, []byte("\n")) {
			bundle.WriteByte('\n')
		}
		bundle.Write(newCertPEM)
		if err := writeFileSecure(state.ClientCAPath, bundle.Bytes(), 0644); err != nil {
			logger.Printf("cert rotate write bundle error: %v", err)
			http.Error(w, fmt.Sprintf("write CA bundle failed: %v", err), http.StatusInternalServerError)
			return
		}

		if _, err := mgr.Reload(); err != nil {
			logger.Printf("cert rotate reload error: %v", err)
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "certs.rotate", "certs", "")
		writeAudit(logger, st, event, nil)

		status := buildRotationStatus(logger, st, mgr)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func buildRotationStatus(logger *log.Logger, st store.Store, mgr *certs.Manager) CertRotationStatus {
	status := CertRotationStatus{UpdatedAt: time.Now().UTC()}
	var errs []string
	if mgr == nil {
		status.Errors = []string{"cert manager not configured"}
		return status
	}
	state := mgr.State()
	status.ActiveCA = CAInfo{Path: state.ActiveCertPath}
	if state.ActiveCert != nil {
		status.ActiveCA.Subject = state.ActiveCert.Subject.String()
		status.ActiveCA.Fingerprint = state.ActiveFingerprint
		status.ActiveCA.NotBefore = state.ActiveCert.NotBefore
		status.ActiveCA.NotAfter = state.ActiveCert.NotAfter
		status.ActiveCA.CertCount = 1
	} else if state.ActiveCertPath == "" {
		errs = append(errs, "ACTIVE_CA_CERT_PATH not set")
	}
	status.ClientCA = CAInfo{Path: state.ClientCAPath}
	if len(state.ClientCerts) > 0 {
		status.ClientCA.CertCount = len(state.ClientCerts)
		if len(state.ClientCerts) == 1 {
			cert := state.ClientCerts[0]
			status.ClientCA.Subject = cert.Subject.String()
			status.ClientCA.Fingerprint = fingerprintCert(cert)
			status.ClientCA.NotBefore = cert.NotBefore
			status.ClientCA.NotAfter = cert.NotAfter
		}
		status.ClientCA.ContainsActive = state.ClientContainsActive
	} else if state.ClientCAPath == "" {
		errs = append(errs, "CA_BUNDLE_PATH/TLS_CLIENT_CA_PATH/CA_CERT_PATH not set")
	}
	if state.ActiveCert != nil && state.Signer == nil {
		errs = append(errs, "active CA key not configured; reenroll disabled")
	}

	counts, err := countRotationDevices(st)
	if err != nil {
		logger.Printf("rotation device count error: %v", err)
		errs = append(errs, "device count error")
	}
	status.DeviceCounts = counts

	if len(errs) > 0 {
		status.Errors = errs
	}
	return status
}

func fingerprintCert(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func generateRotationCA(prev *x509.Certificate) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	subject := pkix.Name{CommonName: "HardwareOps Dev CA"}
	if prev != nil {
		subject = prev.Subject
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	ski := sha1.Sum(pubBytes)
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		SubjectKeyId:          ski[:],
		AuthorityKeyId:        ski[:],
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func writeFileSecure(path string, data []byte, perm os.FileMode) error {
	if path == "" {
		return errors.New("path empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ap := filepath.Clean(a)
	bp := filepath.Clean(b)
	return ap == bp
}

func countRotationDevices(st store.Store) (RotationDeviceCounts, error) {
	counts := RotationDeviceCounts{}
	const batch = 500
	for offset := 0; ; offset += batch {
		items, err := st.ListDevices(store.ListDevicesFilter{Limit: batch, Offset: offset})
		if err != nil {
			return counts, err
		}
		if len(items) == 0 {
			break
		}
		for _, d := range items {
			counts.Total++
			active, _ := certMetaFromJSON(d.MetadataJSON)
			if active == nil {
				counts.Unknown++
			} else if *active {
				counts.Active++
			} else {
				counts.NeedsReenroll++
			}
		}
		if len(items) < batch {
			break
		}
	}
	return counts, nil
}
