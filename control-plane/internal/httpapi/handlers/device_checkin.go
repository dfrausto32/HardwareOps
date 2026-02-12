package handlers

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

type DeviceCheckinRequest struct {
	DeviceID          string                            `json:"deviceId"`
	AgentVersion      string                            `json:"agentVersion"`
	Current           DeviceCurrent                     `json:"current"`
	CurrentComponents map[string]DeviceCurrentComponent `json:"currentComponents,omitempty"`
	Labels            json.RawMessage                   `json:"labels,omitempty"`
	Capabilities      json.RawMessage                   `json:"capabilities"`
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

type DeviceCurrentComponent struct {
	SoftwareVersion     string     `json:"softwareVersion,omitempty"`
	ConfigRev           string     `json:"configRev,omitempty"`
	LastApplyStatus     string     `json:"lastApplyStatus,omitempty"`
	LastApplyError      string     `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string     `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string     `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string     `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time `json:"lastPreApplyAt,omitempty"`
}

type DeviceComponentState struct {
	CurrentVersion      string     `json:"currentVersion,omitempty"`
	CurrentConfigRev    string     `json:"currentConfigRev,omitempty"`
	LastApplyStatus     string     `json:"lastApplyStatus,omitempty"`
	LastApplyError      string     `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string     `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string     `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string     `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time `json:"lastPreApplyAt,omitempty"`
}

type DeviceCheckinResponse struct {
	Desired        *DesiredState `json:"desired"`
	PendingActions []Action      `json:"pendingActions"`
	ServerTime     time.Time     `json:"serverTime"`
}

type DesiredState struct {
	ArtifactID      string                      `json:"artifactId"`
	SoftwareVersion string                      `json:"softwareVersion"`
	ConfigRev       string                      `json:"configRev"`
	DownloadURL     string                      `json:"downloadUrl"`
	ApplyPolicy     json.RawMessage             `json:"applyPolicy"`
	CheckinInterval int                         `json:"checkinIntervalSec,omitempty"`
	Source          string                      `json:"source,omitempty"`
	Components      map[string]DesiredComponent `json:"components,omitempty"`
}

type DesiredComponent struct {
	ArtifactID      string          `json:"artifactId"`
	ArtifactType    string          `json:"artifactType,omitempty"`
	SoftwareVersion string          `json:"softwareVersion"`
	ConfigRev       string          `json:"configRev"`
	DownloadURL     string          `json:"downloadUrl"`
	ApplyPolicy     json.RawMessage `json:"applyPolicy"`
	Source          string          `json:"source,omitempty"`
	Locked          bool            `json:"locked,omitempty"`
}

type Action struct {
	ActionID   string          `json:"actionId"`
	Type       string          `json:"type"`
	Params     json.RawMessage `json:"params"`
	TimeoutSec int             `json:"timeoutSec"`
}

func DeviceCheckin(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, clientCertHeader string, activeCAPool func() *x509.CertPool) http.HandlerFunc {
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
		var certNeedsReenroll bool
		var certMetaUpdate []byte
		if activeCAPool != nil {
			pool := activeCAPool()
			if pool != nil {
				cert := peerCertFromRequest(r, trustProxy, clientCertHeader)
				if cert != nil {
					certNeedsReenroll = needsReenroll(cert, pool)
					certMetaUpdate = updateCertMetaIfNeeded(device.MetadataJSON, !certNeedsReenroll, now)
				}
			}
		}
		prevState, _, _ := st.GetDeviceState(req.DeviceID)
		prevComponents := decodeDeviceComponents(prevState.ComponentsJSON)
		currentComponents := map[string]DeviceComponentState{}
		for key, comp := range req.CurrentComponents {
			component := normalizeComponentKey(key)
			if component == "" {
				continue
			}
			currentComponents[component] = mergeComponentCurrent(comp, prevComponents[component])
		}
		if req.Current.SoftwareVersion == "" {
			if comp, ok := currentComponents["app_bundle"]; ok {
				req.Current.SoftwareVersion = comp.CurrentVersion
				req.Current.ConfigRev = comp.CurrentConfigRev
			}
		}
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
		if componentHasError(currentComponents) || lastApplyStatus == "error" || lastPreApplyStatus == "error" {
			status = "degraded"
		}

		if err := st.UpsertDevice(store.Device{
			DeviceID:     req.DeviceID,
			Status:       status,
			LastSeen:     now,
			LabelsJSON:   req.Labels,
			MetadataJSON: certMetaUpdate,
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
			ComponentsJSON:      encodeDeviceComponents(currentComponents),
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
		desiredComponents := mergeLegacyDesiredComponents(decodeDesiredComponents(desired.ComponentsJSON), desired.ArtifactID, desired.DesiredVersion, desired.DesiredConfigRev, desired.PolicyJSON, desired.Source)
		agentUpdate := false
		if len(currentComponents) > 0 {
			for key, comp := range currentComponents {
				existing := desiredComponents[key]
				if existing.Source == "manual" {
					continue
				}
				desiredComponents[key] = DesiredComponentResponse{
					ArtifactID:       existing.ArtifactID,
					DesiredVersion:   comp.CurrentVersion,
					DesiredConfigRev: comp.CurrentConfigRev,
					Policy:           existing.Policy,
					Source:           "agent",
				}
				agentUpdate = true
			}
		}
		if agentUpdate {
			legacy := legacyFromComponents(desiredComponents)
			_ = st.UpsertDesiredStateDevice(store.DesiredStateDevice{
				DeviceID:         req.DeviceID,
				ArtifactID:       legacy.ArtifactID,
				DesiredVersion:   legacy.DesiredVersion,
				DesiredConfigRev: legacy.DesiredConfigRev,
				PolicyJSON:       legacy.Policy,
				ComponentsJSON:   encodeDesiredComponents(desiredComponents),
				CheckinInterval:  desired.CheckinInterval,
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
		groupComponents := mergeLegacyDesiredComponents(decodeDesiredComponents(groupDesired.ComponentsJSON), groupDesired.ArtifactID, groupDesired.DesiredVersion, groupDesired.DesiredConfigRev, groupDesired.PolicyJSON, "")

		respComponents := map[string]DesiredComponent{}
		componentKeys := map[string]struct{}{}
		for key := range desiredComponents {
			componentKeys[key] = struct{}{}
		}
		for key := range groupComponents {
			componentKeys[key] = struct{}{}
		}
		for key := range componentKeys {
			if comp, ok := desiredComponents[key]; ok && comp.Source == "manual" {
				out := toCheckinComponent(comp)
				out.Source = "manual"
				respComponents[key] = out
				continue
			}
			if comp, ok := groupComponents[key]; ok && (comp.ArtifactID != "" || comp.DesiredVersion != "" || comp.DesiredConfigRev != "" || len(comp.Policy) != 0) {
				out := toCheckinComponent(comp)
				out.Source = "group"
				respComponents[key] = out
				continue
			}
			if comp, ok := desiredComponents[key]; ok {
				respComponents[key] = toCheckinComponent(comp)
			}
		}

		var desiredResp *DesiredState
		if len(respComponents) > 0 {
			desiredResp = &DesiredState{
				Components: respComponents,
			}
			if comp, ok := respComponents["app_bundle"]; ok {
				desiredResp.ArtifactID = comp.ArtifactID
				desiredResp.SoftwareVersion = comp.SoftwareVersion
				desiredResp.ConfigRev = comp.ConfigRev
				desiredResp.ApplyPolicy = comp.ApplyPolicy
				desiredResp.Source = comp.Source
			}
			if hasGroup && groupDesired.CheckinInterval > 0 {
				desiredResp.CheckinInterval = groupDesired.CheckinInterval
			} else if hasDesired {
				desiredResp.CheckinInterval = desired.CheckinInterval
			}
		}

		pending := []Action{}
		if certNeedsReenroll {
			pending = append(pending, Action{
				ActionID:   newActionID(),
				Type:       "device.reenroll",
				Params:     auditJSON(map[string]any{"reason": "ca-rotation"}),
				TimeoutSec: 0,
			})
		}
		resp := DeviceCheckinResponse{
			Desired:        desiredResp,
			PendingActions: pending,
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

func normalizeComponentKey(val string) string {
	key := strings.TrimSpace(strings.ToLower(val))
	return key
}

func needsReenroll(cert *x509.Certificate, pool *x509.CertPool) bool {
	if cert == nil || pool == nil {
		return false
	}
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:       pool,
		CurrentTime: time.Now(),
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err != nil
}

func newActionID() string {
	return fmt.Sprintf("action-%d", time.Now().UnixNano())
}

func mergeComponentCurrent(cur DeviceCurrentComponent, prev DeviceComponentState) DeviceComponentState {
	state := DeviceComponentState{
		CurrentVersion:      cur.SoftwareVersion,
		CurrentConfigRev:    cur.ConfigRev,
		LastApplyStatus:     cur.LastApplyStatus,
		LastApplyError:      cur.LastApplyError,
		LastApplyAt:         cur.LastApplyAt,
		LastApplyArtifactID: cur.LastApplyArtifactID,
		LastPreApplyStatus:  cur.LastPreApplyStatus,
		LastPreApplyError:   cur.LastPreApplyError,
		LastPreApplyAt:      cur.LastPreApplyAt,
	}
	if state.CurrentVersion == "" {
		state.CurrentVersion = prev.CurrentVersion
	}
	if state.CurrentConfigRev == "" {
		state.CurrentConfigRev = prev.CurrentConfigRev
	}
	if state.LastApplyStatus == "" {
		state.LastApplyStatus = prev.LastApplyStatus
	}
	if state.LastApplyError == "" {
		state.LastApplyError = prev.LastApplyError
	}
	if state.LastApplyAt == nil {
		state.LastApplyAt = prev.LastApplyAt
	}
	if state.LastApplyArtifactID == "" {
		state.LastApplyArtifactID = prev.LastApplyArtifactID
	}
	if state.LastPreApplyStatus == "" {
		state.LastPreApplyStatus = prev.LastPreApplyStatus
	}
	if state.LastPreApplyError == "" {
		state.LastPreApplyError = prev.LastPreApplyError
	}
	if state.LastPreApplyAt == nil {
		state.LastPreApplyAt = prev.LastPreApplyAt
	}
	return state
}

func decodeDeviceComponents(raw []byte) map[string]DeviceComponentState {
	if len(raw) == 0 {
		return map[string]DeviceComponentState{}
	}
	var comps map[string]DeviceComponentState
	if err := json.Unmarshal(raw, &comps); err != nil {
		return map[string]DeviceComponentState{}
	}
	return comps
}

func encodeDeviceComponents(comps map[string]DeviceComponentState) []byte {
	if len(comps) == 0 {
		return nil
	}
	out, err := json.Marshal(comps)
	if err != nil {
		return nil
	}
	return out
}

func componentHasError(comps map[string]DeviceComponentState) bool {
	for _, comp := range comps {
		if comp.LastApplyStatus == "error" || comp.LastPreApplyStatus == "error" {
			return true
		}
	}
	return false
}

func toCheckinComponent(comp DesiredComponentResponse) DesiredComponent {
	return DesiredComponent{
		ArtifactID:      comp.ArtifactID,
		ArtifactType:    comp.ArtifactType,
		SoftwareVersion: comp.DesiredVersion,
		ConfigRev:       comp.DesiredConfigRev,
		ApplyPolicy:     comp.Policy,
		Source:          comp.Source,
		Locked:          comp.Locked,
	}
}
