package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

type ApplyResultRequest struct {
	Status           string `json:"status"`
	ArtifactID       string `json:"artifactId"`
	AppliedVersion   string `json:"appliedVersion"`
	AppliedConfigRev string `json:"appliedConfigRev"`
	Error            string `json:"error"`
	PreApplyStatus   string `json:"preApplyStatus"`
	PreApplyError    string `json:"preApplyError"`
}

type ApplyResultResponse struct {
	ApplyID  string    `json:"applyId"`
	DeviceID string    `json:"deviceId"`
	Status   string    `json:"status"`
	At       time.Time `json:"at"`
}

func PostApplyResult(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, clientCertHeader string) http.HandlerFunc {
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
		if req.PreApplyStatus != "" && req.PreApplyStatus != "success" && req.PreApplyStatus != "error" && req.PreApplyStatus != "skipped" {
			http.Error(w, "preApplyStatus must be success, error, or skipped", http.StatusBadRequest)
			return
		}
		if req.ArtifactID != "" {
			if _, err := uuid.Parse(req.ArtifactID); err != nil {
				http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
				return
			}
		}
		if req.Status == "success" && req.AppliedVersion == "" {
			http.Error(w, "appliedVersion required on success", http.StatusBadRequest)
			return
		}

		res := store.ApplyResult{
			ApplyID:          uuid.NewString(),
			DeviceID:         device.DeviceID,
			ArtifactID:       req.ArtifactID,
			Status:           req.Status,
			AppliedVersion:   req.AppliedVersion,
			AppliedConfigRev: req.AppliedConfigRev,
			Error:            req.Error,
			PreApplyStatus:   req.PreApplyStatus,
			PreApplyError:    req.PreApplyError,
			CreatedAt:        time.Now().UTC(),
		}

		event := buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "device.apply_result", "device", device.DeviceID)
		event.MetadataJSON = auditJSON(map[string]any{
			"applyId":          res.ApplyID,
			"artifactId":       res.ArtifactID,
			"status":           res.Status,
			"appliedVersion":   res.AppliedVersion,
			"appliedConfigRev": res.AppliedConfigRev,
			"error":            res.Error,
			"preApplyStatus":   res.PreApplyStatus,
			"preApplyError":    res.PreApplyError,
		})
		if err := st.CreateApplyResult(res); err != nil {
			logger.Printf("create apply result error: %v", err)
			writeAudit(logger, st, event, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		state := store.DeviceState{
			DeviceID:            device.DeviceID,
			LastApplyStatus:     res.Status,
			LastApplyError:      res.Error,
			LastApplyAt:         res.CreatedAt,
			LastApplyArtifactID: res.ArtifactID,
		}
		if res.PreApplyStatus != "" {
			state.LastPreApplyStatus = res.PreApplyStatus
			state.LastPreApplyError = res.PreApplyError
			state.LastPreApplyAt = res.CreatedAt
		}
		if prev, ok, err := st.GetDeviceState(device.DeviceID); err != nil {
			logger.Printf("get device_state error: %v", err)
		} else if ok {
			state.CurrentVersion = prev.CurrentVersion
			state.CurrentConfigRev = prev.CurrentConfigRev
			state.ServicesJSON = prev.ServicesJSON
			state.HealthJSON = prev.HealthJSON
			state.UpdatedAt = prev.UpdatedAt
			if res.ArtifactID == "" {
				state.LastApplyArtifactID = prev.LastApplyArtifactID
			}
			if res.PreApplyStatus == "" {
				state.LastPreApplyStatus = prev.LastPreApplyStatus
				state.LastPreApplyError = prev.LastPreApplyError
				state.LastPreApplyAt = prev.LastPreApplyAt
			}
		}
		if err := st.UpsertDeviceState(state); err != nil {
			logger.Printf("upsert device_state error: %v", err)
		}

		resp := ApplyResultResponse{ApplyID: res.ApplyID, DeviceID: res.DeviceID, Status: res.Status, At: res.CreatedAt}
		writeAudit(logger, st, event, nil)

		if hub != nil {
			payload, _ := json.Marshal(map[string]any{
				"status":           res.Status,
				"artifactId":       res.ArtifactID,
				"appliedVersion":   res.AppliedVersion,
				"appliedConfigRev": res.AppliedConfigRev,
				"error":            res.Error,
				"preApplyStatus":   res.PreApplyStatus,
				"preApplyError":    res.PreApplyError,
			})
			hub.Publish(events.Event{
				Type:     events.TypeDeviceApplyResult,
				DeviceID: res.DeviceID,
				At:       res.CreatedAt,
				Payload:  payload,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
