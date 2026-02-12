package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

type DeviceReenrollRequest struct {
	CSR string `json:"csr"`
}

type DeviceReenrollResponse struct {
	DeviceID  string `json:"deviceId"`
	CertPEM   string `json:"certPem"`
	CACertPEM string `json:"caCertPem"`
}

func DeviceReenroll(logger *log.Logger, st store.Store, signer interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error)
	CACertPEM() []byte
}, trustProxy bool, clientCertHeader string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}
		if signer == nil {
			http.Error(w, "signer not configured", http.StatusInternalServerError)
			return
		}

		var req DeviceReenrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CSR == "" {
			http.Error(w, "csr required", http.StatusBadRequest)
			return
		}
		if len(req.CSR) > maxCSRSize {
			http.Error(w, "csr too large", http.StatusBadRequest)
			return
		}
		if err := validateCSR([]byte(req.CSR)); err != nil {
			logger.Printf("reenroll csr validation error: %v", err)
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		certPEM, fingerprint, err := signer.SignDeviceCert([]byte(req.CSR), device.DeviceID, 365*24*time.Hour)
		if err != nil {
			logger.Printf("reenroll sign error: %v", err)
			http.Error(w, "csr invalid", http.StatusBadRequest)
			return
		}

		meta := []byte(nil)
		if caFingerprint, _, err := caFingerprintFromPEM(signer.CACertPEM()); err == nil {
			meta = updateCertMeta(device.MetadataJSON, map[string]any{
				"active":        true,
				"caFingerprint": caFingerprint,
				"checkedAt":     time.Now().UTC().Format(time.RFC3339),
			})
		}
		if err := st.UpsertDevice(store.Device{
			DeviceID:        device.DeviceID,
			CertFingerprint: fingerprint,
			Status:          device.Status,
			LastSeen:        time.Now().UTC(),
			MetadataJSON:    meta,
		}); err != nil {
			logger.Printf("reenroll update device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, AuditActor{Type: "device", ID: device.DeviceID, AuthMethod: "mtls"}, "device.reenroll", "device", device.DeviceID)
		event.MetadataJSON = auditJSON(map[string]any{"fingerprint": fingerprint})
		writeAudit(logger, st, event, nil)

		resp := DeviceReenrollResponse{
			DeviceID:  device.DeviceID,
			CertPEM:   string(certPEM),
			CACertPEM: string(signer.CACertPEM()),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
