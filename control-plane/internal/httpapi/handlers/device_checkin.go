package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

type DeviceCheckinRequest struct {
	DeviceID     string          `json:"deviceId"`
	AgentVersion string          `json:"agentVersion"`
	Current      DeviceCurrent   `json:"current"`
	Labels       json.RawMessage `json:"labels,omitempty"`
	Capabilities json.RawMessage `json:"capabilities"`
}

type DeviceCurrent struct {
	SoftwareVersion     string          `json:"softwareVersion"`
	ConfigRev           string          `json:"configRev"`
	Services            json.RawMessage `json:"services"`
	Health              json.RawMessage `json:"health"`
	LastApplyStatus     string          `json:"lastApplyStatus,omitempty"`
	LastApplyError      string          `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time      `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string          `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string          `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string          `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time      `json:"lastPreApplyAt,omitempty"`
}

type DeviceCheckinResponse struct {
	Desired        *DesiredState `json:"desired"`
	PendingActions []Action      `json:"pendingActions"`
	ServerTime     time.Time     `json:"serverTime"`
}

type DesiredState struct {
	ArtifactID      string          `json:"artifactId"`
	SoftwareVersion string          `json:"softwareVersion"`
	ConfigRev       string          `json:"configRev"`
	DownloadURL     string          `json:"downloadUrl"`
	ApplyPolicy     json.RawMessage `json:"applyPolicy"`
	CheckinInterval int             `json:"checkinIntervalSec,omitempty"`
	Source          string          `json:"source,omitempty"`
}

type Action struct {
	ActionID   string          `json:"actionId"`
	Type       string          `json:"type"`
	Params     json.RawMessage `json:"params"`
	TimeoutSec int             `json:"timeoutSec"`
}

func DeviceCheckin(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, clientCertHeader string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, err := deviceFromMTLS(r, st, trustProxy, clientCertHeader)
		if err != nil {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}

		var req DeviceCheckinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.DeviceID != "" && req.DeviceID != device.DeviceID {
			http.Error(w, "deviceId does not match client certificate", http.StatusUnauthorized)
			return
		}
		req.DeviceID = device.DeviceID

		now := time.Now().UTC()
		prevApply := prevApplyStatus(st, req.DeviceID)
		lastApplyStatus := req.Current.LastApplyStatus
		if lastApplyStatus == "" {
			lastApplyStatus = prevApply.Status
		}
		lastApplyError := req.Current.LastApplyError
		if lastApplyError == "" {
			lastApplyError = prevApply.Error
		}
		lastApplyAt := prevApply.At
		if req.Current.LastApplyAt != nil {
			lastApplyAt = *req.Current.LastApplyAt
		}
		lastApplyArtifactID := req.Current.LastApplyArtifactID
		if lastApplyArtifactID == "" {
			lastApplyArtifactID = prevApply.ArtifactID
		}
		lastPreApplyStatus := req.Current.LastPreApplyStatus
		if lastPreApplyStatus == "" {
			lastPreApplyStatus = prevApply.PreStatus
		}
		lastPreApplyError := req.Current.LastPreApplyError
		if lastPreApplyError == "" {
			lastPreApplyError = prevApply.PreError
		}
		lastPreApplyAt := prevApply.PreAt
		if req.Current.LastPreApplyAt != nil {
			lastPreApplyAt = *req.Current.LastPreApplyAt
		}
		status := "active"
		if lastApplyStatus == "error" || lastPreApplyStatus == "error" {
			status = "degraded"
		}

		if err := st.UpsertDevice(store.Device{
			DeviceID:   req.DeviceID,
			Status:     status,
			LastSeen:   now,
			LabelsJSON: req.Labels,
		}); err != nil {
			logger.Printf("upsert device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if err := st.UpsertDeviceState(store.DeviceState{
			DeviceID:            req.DeviceID,
			CurrentVersion:      req.Current.SoftwareVersion,
			CurrentConfigRev:    req.Current.ConfigRev,
			ServicesJSON:        req.Current.Services,
			HealthJSON:          req.Current.Health,
			UpdatedAt:           now,
			LastApplyStatus:     lastApplyStatus,
			LastApplyError:      lastApplyError,
			LastApplyAt:         lastApplyAt,
			LastApplyArtifactID: lastApplyArtifactID,
			LastPreApplyStatus:  lastPreApplyStatus,
			LastPreApplyError:   lastPreApplyError,
			LastPreApplyAt:      lastPreApplyAt,
		}); err != nil {
			logger.Printf("upsert device_state error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		logger.Printf("checkin device=%s agent=%s", req.DeviceID, req.AgentVersion)

		desired, hasDesired, err := st.GetDesiredStateDevice(req.DeviceID)
		if err != nil {
			logger.Printf("get desired_state_device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		var desiredResp *DesiredState
		if hasDesired && desired.Source == "manual" {
			desiredResp = &DesiredState{
				ArtifactID:      desired.ArtifactID,
				SoftwareVersion: desired.DesiredVersion,
				ConfigRev:       desired.DesiredConfigRev,
				ApplyPolicy:     desired.PolicyJSON,
				CheckinInterval: desired.CheckinInterval,
				Source:          "manual",
			}
		} else {
			if req.Current.SoftwareVersion != "" || req.Current.ConfigRev != "" {
				_ = st.UpsertDesiredStateDevice(store.DesiredStateDevice{
					DeviceID:         req.DeviceID,
					DesiredVersion:   req.Current.SoftwareVersion,
					DesiredConfigRev: req.Current.ConfigRev,
					Source:           "agent",
					UpdatedAt:        now,
				})
				if desired, hasDesired, err = st.GetDesiredStateDevice(req.DeviceID); err != nil {
					logger.Printf("get desired_state_device error: %v", err)
					http.Error(w, "storage error", http.StatusInternalServerError)
					return
				}
			}

			groupDesired, hasGroup, err := st.GetDesiredStateGroupForDevice(req.DeviceID)
			if err != nil {
				logger.Printf("get desired_state_group error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}

			if hasGroup {
				desiredResp = &DesiredState{
					ArtifactID:      groupDesired.ArtifactID,
					SoftwareVersion: groupDesired.DesiredVersion,
					ConfigRev:       groupDesired.DesiredConfigRev,
					ApplyPolicy:     groupDesired.PolicyJSON,
					CheckinInterval: groupDesired.CheckinInterval,
					Source:          "group",
				}
			} else if hasDesired {
				desiredResp = &DesiredState{
					ArtifactID:      desired.ArtifactID,
					SoftwareVersion: desired.DesiredVersion,
					ConfigRev:       desired.DesiredConfigRev,
					ApplyPolicy:     desired.PolicyJSON,
					CheckinInterval: desired.CheckinInterval,
					Source:          desired.Source,
				}
			}
		}

		resp := DeviceCheckinResponse{
			Desired:        desiredResp,
			PendingActions: []Action{},
			ServerTime:     now,
		}

		if hub != nil {
			payload, _ := json.Marshal(map[string]any{
				"agentVersion":       req.AgentVersion,
				"currentVersion":     req.Current.SoftwareVersion,
				"currentConfigRev":   req.Current.ConfigRev,
				"artifactId":         desiredRespArtifactID(desiredResp),
				"desiredVersion":     desiredRespSoftwareVersion(desiredResp),
				"desiredConfigRev":   desiredRespConfigRev(desiredResp),
				"checkinIntervalSec": desiredRespCheckinInterval(desiredResp),
			})
			hub.Publish(events.Event{
				Type:     events.TypeDeviceCheckin,
				DeviceID: req.DeviceID,
				At:       now,
				Payload:  payload,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

type applySnapshot struct {
	Status     string
	Error      string
	At         time.Time
	ArtifactID string
	PreStatus  string
	PreError   string
	PreAt      time.Time
}

func prevApplyStatus(st store.Store, deviceID string) applySnapshot {
	if deviceID == "" {
		return applySnapshot{}
	}
	if prev, ok, err := st.GetDeviceState(deviceID); err == nil && ok {
		return applySnapshot{
			Status:     prev.LastApplyStatus,
			Error:      prev.LastApplyError,
			At:         prev.LastApplyAt,
			ArtifactID: prev.LastApplyArtifactID,
			PreStatus:  prev.LastPreApplyStatus,
			PreError:   prev.LastPreApplyError,
			PreAt:      prev.LastPreApplyAt,
		}
	}
	return applySnapshot{}
}

func desiredRespArtifactID(resp *DesiredState) string {
	if resp == nil {
		return ""
	}
	return resp.ArtifactID
}

func desiredRespSoftwareVersion(resp *DesiredState) string {
	if resp == nil {
		return ""
	}
	return resp.SoftwareVersion
}

func desiredRespConfigRev(resp *DesiredState) string {
	if resp == nil {
		return ""
	}
	return resp.ConfigRev
}

func desiredRespCheckinInterval(resp *DesiredState) int {
	if resp == nil {
		return 0
	}
	return resp.CheckinInterval
}
