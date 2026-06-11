package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/events"
	"github.com/parcel/control-plane/internal/metrics"
	"github.com/parcel/control-plane/internal/store"
)

type ApplyResultRequest struct {
	Status           string `json:"status"`
	ArtifactID       string `json:"artifactId"`
	Component        string `json:"component,omitempty"`
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

func PostApplyResult(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, clientCertHeader string, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recordApply := func(status, component string) {
			if metricsCollector != nil {
				metricsCollector.IncApply(status, component)
			}
		}
		recordPreApply := func(status, component string) {
			if metricsCollector != nil {
				metricsCollector.IncPreApply(status, component)
			}
		}

		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			recordApply("error", "unknown")
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}

		deviceID := chi.URLParam(r, "deviceId")
		if deviceID == "" {
			recordApply("error", "unknown")
			http.Error(w, "deviceId required", http.StatusBadRequest)
			return
		}
		if deviceID != device.DeviceID {
			recordApply("error", normalizeComponentKey(""))
			http.Error(w, "deviceId does not match client certificate", http.StatusUnauthorized)
			return
		}

		var req ApplyResultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			recordApply("error", "unknown")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		component := normalizeComponentKey(req.Component)
		if component == "" {
			component = "unknown"
		}
		if req.Status != "success" && req.Status != "error" {
			recordApply("error", component)
			http.Error(w, "status must be success or error", http.StatusBadRequest)
			return
		}
		if req.PreApplyStatus != "" && req.PreApplyStatus != "success" && req.PreApplyStatus != "error" && req.PreApplyStatus != "skipped" {
			recordApply("error", component)
			http.Error(w, "preApplyStatus must be success, error, or skipped", http.StatusBadRequest)
			return
		}
		if req.ArtifactID != "" {
			if _, err := uuid.Parse(req.ArtifactID); err != nil {
				recordApply("error", component)
				http.Error(w, "artifactId must be uuid", http.StatusBadRequest)
				return
			}
		}
		if req.Status == "success" && req.AppliedVersion == "" {
			recordApply("error", component)
			http.Error(w, "appliedVersion required on success", http.StatusBadRequest)
			return
		}

		res := store.ApplyResult{
			ApplyID:          uuid.NewString(),
			DeviceID:         device.DeviceID,
			ArtifactID:       req.ArtifactID,
			Component:        normalizeComponentKey(req.Component),
			Status:           req.Status,
			AppliedVersion:   req.AppliedVersion,
			AppliedConfigRev: req.AppliedConfigRev,
			Error:            req.Error,
			PreApplyStatus:   req.PreApplyStatus,
			PreApplyError:    req.PreApplyError,
			CreatedAt:        time.Now().UTC(),
		}

		event := withPHI(buildAuditEvent(r, trustProxy, actorDevice(device.DeviceID), "device.apply_result", "device", device.DeviceID), "device_apply_logs")
		event.MetadataJSON = auditJSON(map[string]any{
			"applyId":          res.ApplyID,
			"artifactId":       res.ArtifactID,
			"component":        res.Component,
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
			recordApply("error", component)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		recordApply(res.Status, component)
		if res.PreApplyStatus != "" {
			recordPreApply(res.PreApplyStatus, component)
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
			component := normalizeComponentKey(req.Component)
			components := decodeDeviceComponents(prev.ComponentsJSON)
			compState := components[component]
			if res.Status != "" {
				compState.LastApplyStatus = res.Status
				compState.LastApplyError = res.Error
				compState.LastApplyAt = timePtr(res.CreatedAt)
				if res.ArtifactID != "" {
					compState.LastApplyArtifactID = res.ArtifactID
				}
				if res.PreApplyStatus != "" {
					compState.LastPreApplyStatus = res.PreApplyStatus
					compState.LastPreApplyError = res.PreApplyError
					compState.LastPreApplyAt = timePtr(res.CreatedAt)
				}
				if res.Status == "success" {
					if res.AppliedVersion != "" {
						compState.CurrentVersion = res.AppliedVersion
					}
					if res.AppliedConfigRev != "" {
						compState.CurrentConfigRev = res.AppliedConfigRev
					}
				}
				components[component] = compState
				state.ComponentsJSON = encodeDeviceComponents(components)
			}
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
			if res.Component != "" && res.Component != "app_bundle" {
				state.LastApplyStatus = prev.LastApplyStatus
				state.LastApplyError = prev.LastApplyError
				state.LastApplyAt = prev.LastApplyAt
				state.LastApplyArtifactID = prev.LastApplyArtifactID
				state.LastPreApplyStatus = prev.LastPreApplyStatus
				state.LastPreApplyError = prev.LastPreApplyError
				state.LastPreApplyAt = prev.LastPreApplyAt
			} else if res.Status == "success" {
				if res.AppliedVersion != "" {
					state.CurrentVersion = res.AppliedVersion
				}
				if res.AppliedConfigRev != "" {
					state.CurrentConfigRev = res.AppliedConfigRev
				}
			}
		}
		if err := st.UpsertDeviceState(state); err != nil {
			logger.Printf("upsert device_state error: %v", err)
		}

		resp := ApplyResultResponse{ApplyID: res.ApplyID, DeviceID: res.DeviceID, Status: res.Status, At: res.CreatedAt}
		writeAudit(logger, st, event, nil)

		payload, _ := json.Marshal(map[string]any{
			"deviceId":         res.DeviceID,
			"status":           res.Status,
			"artifactId":       res.ArtifactID,
			"component":        res.Component,
			"appliedVersion":   res.AppliedVersion,
			"appliedConfigRev": res.AppliedConfigRev,
			"error":            res.Error,
			"preApplyStatus":   res.PreApplyStatus,
			"preApplyError":    res.PreApplyError,
		})
		emitRuntimeEvent(logger, st, hub, events.Event{
			Type:     events.TypeDeviceApplyResult,
			DeviceID: res.DeviceID,
			At:       res.CreatedAt,
			Payload:  payload,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
