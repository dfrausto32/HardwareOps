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
	Cleanup      CleanupStatus        `json:"cleanup"`
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

type CleanupStatus struct {
	Eligible            bool      `json:"eligible"`
	Reason              string    `json:"reason,omitempty"`
	ActiveFingerprint   string    `json:"activeFingerprint,omitempty"`
	PreviousFingerprint string    `json:"previousFingerprint,omitempty"`
	RotatedAt           time.Time `json:"rotatedAt,omitempty"`
	GracePeriodSeconds  int64     `json:"gracePeriodSec,omitempty"`
	GraceDeadline       time.Time `json:"graceDeadline,omitempty"`
	GraceRemainingSec   int64     `json:"graceRemainingSec,omitempty"`
	CleanedAt           time.Time `json:"cleanedAt,omitempty"`
	CleanedReason       string    `json:"cleanedReason,omitempty"`
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
		var req BreakglassRequest
		if err := decodeBreakglassRequest(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "certs.reload", "certs", "")
		if mgr == nil {
			auditBreakglassFailure(logger, st, event, "error", "cert_manager_not_configured", req.Reason, nil)
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}
		if _, err := mgr.Reload(); err != nil {
			logger.Printf("cert reload error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "reload_failed", req.Reason, nil)
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}
		status := buildRotationStatus(logger, st, mgr)
		event.MetadataJSON = breakglassMetadata(req.Reason, map[string]any{
			"activeFingerprint":  status.ActiveCA.Fingerprint,
			"bundleContainsCA":   status.ClientCA.ContainsActive,
			"clientBundleCerts":  status.ClientCA.CertCount,
			"cleanupEligibility": status.Cleanup.Reason,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"activeFingerprint": status.ActiveCA.Fingerprint,
			"clientCaPath":      status.ClientCA.Path,
			"clientCaCertCount": status.ClientCA.CertCount,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func RotateCertRotation(logger *log.Logger, st store.Store, mgr *certs.Manager, trustProxy bool, gracePeriod time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BreakglassRequest
		if err := decodeBreakglassRequest(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "certs.rotate", "certs", "")
		if mgr == nil {
			auditBreakglassFailure(logger, st, event, "error", "cert_manager_not_configured", req.Reason, nil)
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}
		state := mgr.State()
		if state.ActiveCertPath == "" || state.ActiveKeyPath == "" {
			auditBreakglassFailure(logger, st, event, "error", "active_ca_paths_not_set", req.Reason, nil)
			http.Error(w, "ACTIVE_CA_CERT_PATH/ACTIVE_CA_KEY_PATH not set", http.StatusBadRequest)
			return
		}
		if state.ClientCAPath == "" {
			auditBreakglassFailure(logger, st, event, "error", "ca_bundle_path_not_set", req.Reason, nil)
			http.Error(w, "CA_BUNDLE_PATH not set", http.StatusBadRequest)
			return
		}
		if state.ActiveCertPEM == nil || len(state.ActiveCertPEM) == 0 {
			auditBreakglassFailure(logger, st, event, "error", "active_ca_not_loaded", req.Reason, nil)
			http.Error(w, "active CA cert not loaded", http.StatusBadRequest)
			return
		}
		if samePath(state.ActiveCertPath, state.ClientCAPath) {
			auditBreakglassFailure(logger, st, event, "error", "ca_bundle_path_matches_active_ca", req.Reason, nil)
			http.Error(w, "CA_BUNDLE_PATH must be different from ACTIVE_CA_CERT_PATH", http.StatusBadRequest)
			return
		}

		oldFingerprint := state.ActiveFingerprint
		newCertPEM, newKeyPEM, err := generateRotationCA(state.ActiveCert)
		if err != nil {
			logger.Printf("cert rotate generate error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "generate_ca_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
			})
			http.Error(w, fmt.Sprintf("generate CA failed: %v", err), http.StatusInternalServerError)
			return
		}
		newFingerprint, _, err := caFingerprintFromPEM(newCertPEM)
		if err != nil {
			logger.Printf("cert rotate fingerprint error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "fingerprint_ca_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
			})
			http.Error(w, fmt.Sprintf("fingerprint CA failed: %v", err), http.StatusInternalServerError)
			return
		}

		if err := writeFileSecure(state.ActiveCertPath, newCertPEM, 0644); err != nil {
			logger.Printf("cert rotate write cert error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "write_active_ca_cert_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
				"nextFingerprint":     newFingerprint,
			})
			http.Error(w, fmt.Sprintf("write active CA cert failed: %v", err), http.StatusInternalServerError)
			return
		}
		if err := writeFileSecure(state.ActiveKeyPath, newKeyPEM, 0600); err != nil {
			logger.Printf("cert rotate write key error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "write_active_ca_key_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
				"nextFingerprint":     newFingerprint,
			})
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
			auditBreakglassFailure(logger, st, event, "error", "write_ca_bundle_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
				"nextFingerprint":     newFingerprint,
			})
			http.Error(w, fmt.Sprintf("write CA bundle failed: %v", err), http.StatusInternalServerError)
			return
		}

		if _, err := mgr.Reload(); err != nil {
			logger.Printf("cert rotate reload error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "reload_failed", req.Reason, map[string]any{
				"previousFingerprint": oldFingerprint,
				"nextFingerprint":     newFingerprint,
			})
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}

		if st != nil {
			graceSeconds := int64(gracePeriod.Seconds())
			if graceSeconds < 0 {
				graceSeconds = 0
			}
			if err := st.SetCertRotationState(store.CertRotationState{
				ActiveFingerprint:   newFingerprint,
				PreviousFingerprint: oldFingerprint,
				RotatedAt:           time.Now().UTC(),
				GracePeriodSeconds:  graceSeconds,
			}); err != nil {
				logger.Printf("cert rotate state error: %v", err)
			}
		}

		status := buildRotationStatus(logger, st, mgr)
		event.MetadataJSON = breakglassMetadata(req.Reason, map[string]any{
			"previousFingerprint": oldFingerprint,
			"nextFingerprint":     newFingerprint,
			"gracePeriodSec":      int64(gracePeriod.Seconds()),
		})
		event.AfterJSON = auditJSON(map[string]any{
			"activeFingerprint": status.ActiveCA.Fingerprint,
			"clientCaCertCount": status.ClientCA.CertCount,
			"cleanupReason":     status.Cleanup.Reason,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func CleanupCertRotation(logger *log.Logger, st store.Store, mgr *certs.Manager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BreakglassRequest
		if err := decodeBreakglassRequest(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "certs.cleanup", "certs", "")
		if mgr == nil {
			auditBreakglassFailure(logger, st, event, "error", "cert_manager_not_configured", req.Reason, nil)
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}
		state := mgr.State()
		if state.ClientCAPath == "" {
			auditBreakglassFailure(logger, st, event, "error", "ca_bundle_path_not_set", req.Reason, nil)
			http.Error(w, "CA_BUNDLE_PATH not set", http.StatusBadRequest)
			return
		}
		if state.ActiveCertPath == "" || state.ActiveCertPEM == nil {
			auditBreakglassFailure(logger, st, event, "error", "active_ca_not_loaded", req.Reason, nil)
			http.Error(w, "active CA cert not loaded", http.StatusBadRequest)
			return
		}
		if samePath(state.ActiveCertPath, state.ClientCAPath) {
			auditBreakglassFailure(logger, st, event, "error", "ca_bundle_path_matches_active_ca", req.Reason, nil)
			http.Error(w, "CA_BUNDLE_PATH must be different from ACTIVE_CA_CERT_PATH", http.StatusBadRequest)
			return
		}
		if st == nil {
			auditBreakglassFailure(logger, st, event, "error", "store_not_configured", req.Reason, nil)
			http.Error(w, "store not configured", http.StatusServiceUnavailable)
			return
		}
		rotationState, ok, err := st.GetCertRotationState()
		if err != nil {
			logger.Printf("cert cleanup state error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "rotation_state_error", req.Reason, nil)
			http.Error(w, "rotation state error", http.StatusInternalServerError)
			return
		}
		if !ok || rotationState.PreviousFingerprint == "" {
			auditBreakglassFailure(logger, st, event, "denied", "rotation_state_not_found", req.Reason, nil)
			http.Error(w, "rotation state not found", http.StatusBadRequest)
			return
		}
		counts, err := countRotationDevices(st)
		if err != nil {
			logger.Printf("cert cleanup count error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "device_count_error", req.Reason, nil)
			http.Error(w, "device count error", http.StatusInternalServerError)
			return
		}

		cleanup := evaluateCleanup(rotationState, counts, state.ActiveFingerprint)
		if !cleanup.Eligible {
			auditBreakglassFailure(logger, st, event, "denied", cleanup.Reason, req.Reason, map[string]any{
				"cleanupReason":       cleanup.Reason,
				"activeFingerprint":   cleanup.ActiveFingerprint,
				"previousFingerprint": cleanup.PreviousFingerprint,
				"graceRemainingSec":   cleanup.GraceRemainingSec,
			})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(CertRotationStatus{
				ActiveCA:     CAInfo{Path: state.ActiveCertPath},
				ClientCA:     CAInfo{Path: state.ClientCAPath, CertCount: len(state.ClientCerts), ContainsActive: state.ClientContainsActive},
				DeviceCounts: counts,
				Cleanup:      cleanup,
				UpdatedAt:    time.Now().UTC(),
			})
			return
		}

		if err := writeFileSecure(state.ClientCAPath, state.ActiveCertPEM, 0644); err != nil {
			logger.Printf("cert cleanup write error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "write_ca_bundle_failed", req.Reason, map[string]any{
				"cleanupReason": cleanup.Reason,
			})
			http.Error(w, fmt.Sprintf("write CA bundle failed: %v", err), http.StatusInternalServerError)
			return
		}
		if _, err := mgr.Reload(); err != nil {
			logger.Printf("cert cleanup reload error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "reload_failed", req.Reason, map[string]any{
				"cleanupReason": cleanup.Reason,
			})
			http.Error(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
			return
		}

		rotationState.CleanedAt = time.Now().UTC()
		rotationState.CleanedReason = cleanup.Reason
		rotationState.ActiveFingerprint = state.ActiveFingerprint
		if err := st.SetCertRotationState(rotationState); err != nil {
			logger.Printf("cert cleanup state update error: %v", err)
		}

		status := buildRotationStatus(logger, st, mgr)
		event.MetadataJSON = breakglassMetadata(req.Reason, map[string]any{
			"cleanupReason":       cleanup.Reason,
			"activeFingerprint":   cleanup.ActiveFingerprint,
			"previousFingerprint": cleanup.PreviousFingerprint,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"cleanedAt":           status.Cleanup.CleanedAt,
			"cleanedReason":       status.Cleanup.CleanedReason,
			"activeFingerprint":   status.Cleanup.ActiveFingerprint,
			"previousFingerprint": status.Cleanup.PreviousFingerprint,
		})
		writeAudit(logger, st, event, nil)
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
	if st != nil {
		if rotationState, ok, err := st.GetCertRotationState(); err != nil {
			logger.Printf("rotation state fetch error: %v", err)
			errs = append(errs, "rotation state error")
		} else if ok {
			status.Cleanup = evaluateCleanup(rotationState, counts, state.ActiveFingerprint)
		} else {
			status.Cleanup = CleanupStatus{Reason: "no-rotation"}
		}
	}

	if len(errs) > 0 {
		status.Errors = errs
	}
	return status
}

func evaluateCleanup(rotation store.CertRotationState, counts RotationDeviceCounts, activeFingerprint string) CleanupStatus {
	now := time.Now().UTC()
	out := CleanupStatus{
		ActiveFingerprint:   rotation.ActiveFingerprint,
		PreviousFingerprint: rotation.PreviousFingerprint,
		RotatedAt:           rotation.RotatedAt,
		GracePeriodSeconds:  rotation.GracePeriodSeconds,
		CleanedAt:           rotation.CleanedAt,
		CleanedReason:       rotation.CleanedReason,
	}
	if rotation.PreviousFingerprint == "" || rotation.RotatedAt.IsZero() {
		out.Reason = "no-rotation"
		return out
	}
	if rotation.ActiveFingerprint != "" && activeFingerprint != "" && rotation.ActiveFingerprint != activeFingerprint {
		out.Reason = "active-mismatch"
		return out
	}
	if !rotation.CleanedAt.IsZero() {
		out.Reason = "cleaned"
		return out
	}
	grace := time.Duration(rotation.GracePeriodSeconds) * time.Second
	if grace > 0 {
		out.GraceDeadline = rotation.RotatedAt.Add(grace)
		if now.Before(out.GraceDeadline) {
			out.GraceRemainingSec = int64(out.GraceDeadline.Sub(now).Seconds())
		}
	}
	coverageComplete := counts.Total == 0 || counts.Active == counts.Total
	if coverageComplete {
		out.Eligible = true
		out.Reason = "coverage"
		return out
	}
	if grace > 0 && now.After(rotation.RotatedAt.Add(grace)) {
		out.Eligible = true
		out.Reason = "grace"
		return out
	}
	out.Reason = "waiting"
	return out
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
