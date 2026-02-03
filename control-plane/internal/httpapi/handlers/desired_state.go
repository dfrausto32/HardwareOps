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

type DesiredStateRequest struct {
	ArtifactID       string          `json:"artifactId"`
	DesiredVersion   string          `json:"desiredVersion"`
	DesiredConfigRev string          `json:"desiredConfigRev"`
	Policy           json.RawMessage `json:"policy"`
	CheckinInterval  int             `json:"checkinIntervalSec"`
}

type DesiredStateGroupResponse struct {
	GroupID          string          `json:"groupId"`
	ArtifactID       string          `json:"artifactId,omitempty"`
	DesiredVersion   string          `json:"desiredVersion,omitempty"`
	DesiredConfigRev string          `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage `json:"policy,omitempty"`
	CheckinInterval  int             `json:"checkinIntervalSec,omitempty"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type DesiredStateDeviceResponse struct {
	DeviceID         string          `json:"deviceId"`
	ArtifactID       string          `json:"artifactId,omitempty"`
	DesiredVersion   string          `json:"desiredVersion,omitempty"`
	DesiredConfigRev string          `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage `json:"policy,omitempty"`
	CheckinInterval  int             `json:"checkinIntervalSec,omitempty"`
	Source           string          `json:"source"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type DesiredStateListResponse struct {
	Groups  []DesiredStateGroupResponse  `json:"groups,omitempty"`
	Devices []DesiredStateDeviceResponse `json:"devices,omitempty"`
}

func PutDesiredStateGroup(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		if groupID == "" {
			http.Error(w, "groupId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(groupID); err != nil {
			http.Error(w, "groupId must be uuid", http.StatusBadRequest)
			return
		}

		var req DesiredStateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CheckinInterval < 0 {
			http.Error(w, "checkinIntervalSec must be >= 0", http.StatusBadRequest)
			return
		}
		if req.DesiredVersion == "" && req.ArtifactID == "" && req.CheckinInterval == 0 && len(req.Policy) == 0 {
			http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
			return
		}

		state := store.DesiredStateGroup{
			GroupID:          groupID,
			ArtifactID:       req.ArtifactID,
			DesiredVersion:   req.DesiredVersion,
			DesiredConfigRev: req.DesiredConfigRev,
			PolicyJSON:       req.Policy,
			CheckinInterval:  req.CheckinInterval,
			UpdatedAt:        time.Now().UTC(),
		}
		if err := st.UpsertDesiredStateGroup(state); err != nil {
			logger.Printf("upsert desired_state_group error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := DesiredStateGroupResponse{
			GroupID:          state.GroupID,
			ArtifactID:       state.ArtifactID,
			DesiredVersion:   state.DesiredVersion,
			DesiredConfigRev: state.DesiredConfigRev,
			Policy:           state.PolicyJSON,
			CheckinInterval:  state.CheckinInterval,
			UpdatedAt:        state.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PutDesiredStateDevice(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		if deviceID == "" {
			http.Error(w, "deviceId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(deviceID); err != nil {
			http.Error(w, "deviceId must be uuid", http.StatusBadRequest)
			return
		}

		var req DesiredStateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.CheckinInterval < 0 {
			http.Error(w, "checkinIntervalSec must be >= 0", http.StatusBadRequest)
			return
		}
		if req.DesiredVersion == "" && req.ArtifactID == "" && req.CheckinInterval == 0 && len(req.Policy) == 0 {
			http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
			return
		}

		state := store.DesiredStateDevice{
			DeviceID:         deviceID,
			ArtifactID:       req.ArtifactID,
			DesiredVersion:   req.DesiredVersion,
			DesiredConfigRev: req.DesiredConfigRev,
			PolicyJSON:       req.Policy,
			CheckinInterval:  req.CheckinInterval,
			Source:           "manual",
			UpdatedAt:        time.Now().UTC(),
		}
		if err := st.UpsertDesiredStateDevice(state); err != nil {
			logger.Printf("upsert desired_state_device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := DesiredStateDeviceResponse{
			DeviceID:         state.DeviceID,
			ArtifactID:       state.ArtifactID,
			DesiredVersion:   state.DesiredVersion,
			DesiredConfigRev: state.DesiredConfigRev,
			Policy:           state.PolicyJSON,
			CheckinInterval:  state.CheckinInterval,
			Source:           state.Source,
			UpdatedAt:        state.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListDesiredState(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		resp := DesiredStateListResponse{}

		if scope == "" || scope == "group" {
			groups, err := st.ListDesiredStateGroups()
			if err != nil {
				logger.Printf("list desired_state_group error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			resp.Groups = make([]DesiredStateGroupResponse, 0, len(groups))
			for _, g := range groups {
				resp.Groups = append(resp.Groups, DesiredStateGroupResponse{
					GroupID:          g.GroupID,
					ArtifactID:       g.ArtifactID,
					DesiredVersion:   g.DesiredVersion,
					DesiredConfigRev: g.DesiredConfigRev,
					Policy:           g.PolicyJSON,
					CheckinInterval:  g.CheckinInterval,
					UpdatedAt:        g.UpdatedAt,
				})
			}
		}

		if scope == "" || scope == "device" {
			devices, err := st.ListDesiredStateDevices()
			if err != nil {
				logger.Printf("list desired_state_device error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			resp.Devices = make([]DesiredStateDeviceResponse, 0, len(devices))
			for _, d := range devices {
				resp.Devices = append(resp.Devices, DesiredStateDeviceResponse{
					DeviceID:         d.DeviceID,
					ArtifactID:       d.ArtifactID,
					DesiredVersion:   d.DesiredVersion,
					DesiredConfigRev: d.DesiredConfigRev,
					Policy:           d.PolicyJSON,
					CheckinInterval:  d.CheckinInterval,
					Source:           d.Source,
					UpdatedAt:        d.UpdatedAt,
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
