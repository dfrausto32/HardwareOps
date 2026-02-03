package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

type ApplyResultRequest struct {
	Status           string `json:"status"`
	AppliedVersion   string `json:"appliedVersion"`
	AppliedConfigRev string `json:"appliedConfigRev"`
	Error            string `json:"error"`
}

type ApplyResultResponse struct {
	ApplyID  string    `json:"applyId"`
	DeviceID string    `json:"deviceId"`
	Status   string    `json:"status"`
	At       time.Time `json:"at"`
}

func PostApplyResult(logger *log.Logger, st store.Store, trustProxy bool, clientCertHeader string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}

		deviceID := chi.URLParam(r, "deviceId")
		if deviceID == "" {
			http.Error(w, "deviceId required", http.StatusBadRequest)
			return
		}
		if deviceID != device.DeviceID {
			http.Error(w, "deviceId does not match client certificate", http.StatusUnauthorized)
			return
		}

		var req ApplyResultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Status != "success" && req.Status != "error" {
			http.Error(w, "status must be success or error", http.StatusBadRequest)
			return
		}
		if req.Status == "success" && req.AppliedVersion == "" {
			http.Error(w, "appliedVersion required on success", http.StatusBadRequest)
			return
		}

		res := store.ApplyResult{
			ApplyID:          uuid.NewString(),
			DeviceID:         device.DeviceID,
			Status:           req.Status,
			AppliedVersion:   req.AppliedVersion,
			AppliedConfigRev: req.AppliedConfigRev,
			Error:            req.Error,
			CreatedAt:        time.Now().UTC(),
		}

		if err := st.CreateApplyResult(res); err != nil {
			logger.Printf("create apply result error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := ApplyResultResponse{ApplyID: res.ApplyID, DeviceID: res.DeviceID, Status: res.Status, At: res.CreatedAt}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
