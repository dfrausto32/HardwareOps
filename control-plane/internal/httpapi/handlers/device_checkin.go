package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

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
	SoftwareVersion string          `json:"softwareVersion"`
	ConfigRev       string          `json:"configRev"`
	Services        json.RawMessage `json:"services"`
	Health          json.RawMessage `json:"health"`
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
}

type Action struct {
	ActionID   string          `json:"actionId"`
	Type       string          `json:"type"`
	Params     json.RawMessage `json:"params"`
	TimeoutSec int             `json:"timeoutSec"`
}

func DeviceCheckin(logger *log.Logger, st store.Store, trustProxy bool, clientCertHeader string) http.HandlerFunc {
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
		if err := st.UpsertDevice(store.Device{
			DeviceID:   req.DeviceID,
			Status:     "active",
			LastSeen:   now,
			LabelsJSON: req.Labels,
		}); err != nil {
			logger.Printf("upsert device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		if err := st.UpsertDeviceState(store.DeviceState{
			DeviceID:         req.DeviceID,
			CurrentVersion:   req.Current.SoftwareVersion,
			CurrentConfigRev: req.Current.ConfigRev,
			ServicesJSON:     req.Current.Services,
			HealthJSON:       req.Current.Health,
			UpdatedAt:        now,
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
		if hasDesired && desired.Source == "manual" {
			resp := DeviceCheckinResponse{
				Desired: &DesiredState{
					ArtifactID:      desired.ArtifactID,
					SoftwareVersion: desired.DesiredVersion,
					ConfigRev:       desired.DesiredConfigRev,
					ApplyPolicy:     desired.PolicyJSON,
					CheckinInterval: desired.CheckinInterval,
				},
				PendingActions: []Action{},
				ServerTime:     now,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

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

		var desiredResp *DesiredState
		if hasGroup {
			desiredResp = &DesiredState{
				ArtifactID:      groupDesired.ArtifactID,
				SoftwareVersion: groupDesired.DesiredVersion,
				ConfigRev:       groupDesired.DesiredConfigRev,
				ApplyPolicy:     groupDesired.PolicyJSON,
				CheckinInterval: groupDesired.CheckinInterval,
			}
		} else if hasDesired {
			desiredResp = &DesiredState{
				ArtifactID:      desired.ArtifactID,
				SoftwareVersion: desired.DesiredVersion,
				ConfigRev:       desired.DesiredConfigRev,
				ApplyPolicy:     desired.PolicyJSON,
				CheckinInterval: desired.CheckinInterval,
			}
		}

		resp := DeviceCheckinResponse{
			Desired:        desiredResp,
			PendingActions: []Action{},
			ServerTime:     now,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
