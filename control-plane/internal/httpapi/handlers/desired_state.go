package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

type DesiredStateRequest struct {
	ArtifactID       string                             `json:"artifactId"`
	DesiredVersion   string                             `json:"desiredVersion"`
	DesiredConfigRev string                             `json:"desiredConfigRev"`
	Policy           json.RawMessage                    `json:"policy"`
	Components       map[string]DesiredComponentRequest `json:"components,omitempty"`
	CheckinInterval  int                                `json:"checkinIntervalSec"`
}

type DesiredComponentRequest struct {
	ArtifactID       string          `json:"artifactId,omitempty"`
	ArtifactType     string          `json:"artifactType,omitempty"`
	DesiredVersion   string          `json:"desiredVersion,omitempty"`
	DesiredConfigRev string          `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage `json:"policy,omitempty"`
	Locked           bool            `json:"locked,omitempty"`
}

type DesiredComponentResponse struct {
	ArtifactID       string          `json:"artifactId,omitempty"`
	ArtifactType     string          `json:"artifactType,omitempty"`
	DesiredVersion   string          `json:"desiredVersion,omitempty"`
	DesiredConfigRev string          `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage `json:"policy,omitempty"`
	Source           string          `json:"source,omitempty"`
	Locked           bool            `json:"locked,omitempty"`
}

type DesiredStateGroupResponse struct {
	GroupID          string                              `json:"groupId"`
	ArtifactID       string                              `json:"artifactId,omitempty"`
	DesiredVersion   string                              `json:"desiredVersion,omitempty"`
	DesiredConfigRev string                              `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage                     `json:"policy,omitempty"`
	Components       map[string]DesiredComponentResponse `json:"components,omitempty"`
	CheckinInterval  int                                 `json:"checkinIntervalSec,omitempty"`
	UpdatedAt        time.Time                           `json:"updatedAt"`
}

type DesiredStateDeviceResponse struct {
	DeviceID         string                              `json:"deviceId"`
	ArtifactID       string                              `json:"artifactId,omitempty"`
	DesiredVersion   string                              `json:"desiredVersion,omitempty"`
	DesiredConfigRev string                              `json:"desiredConfigRev,omitempty"`
	Policy           json.RawMessage                     `json:"policy,omitempty"`
	Components       map[string]DesiredComponentResponse `json:"components,omitempty"`
	CheckinInterval  int                                 `json:"checkinIntervalSec,omitempty"`
	Source           string                              `json:"source"`
	UpdatedAt        time.Time                           `json:"updatedAt"`
}

type DesiredStateListResponse struct {
	Groups      []DesiredStateGroupResponse  `json:"groups,omitempty"`
	Devices     []DesiredStateDeviceResponse `json:"devices,omitempty"`
	GroupTotal  int                          `json:"groupTotal"`
	DeviceTotal int                          `json:"deviceTotal"`
}

type desiredStateReleaseAutoTrigger interface {
	Trigger(reason string)
}

func PutDesiredStateGroup(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return putDesiredStateGroup(logger, st, trustProxy, ArtifactSignaturePolicy{}, nil)
}

func PutDesiredStateGroupWithPolicy(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto desiredStateReleaseAutoTrigger) http.HandlerFunc {
	return putDesiredStateGroup(logger, st, trustProxy, sigPolicy, releaseAuto)
}

func putDesiredStateGroup(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto desiredStateReleaseAutoTrigger) http.HandlerFunc {
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
		if len(req.Components) == 0 && req.DesiredVersion == "" && req.ArtifactID == "" {
			if req.DesiredConfigRev != "" || len(req.Policy) > 0 {
				http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
				return
			}
			if req.DesiredConfigRev == "" && len(req.Policy) == 0 && req.CheckinInterval == 0 {
				http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
				return
			}
		}
		components, err := normalizeDesiredComponents(req, "", st, sigPolicy)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := enforceComponentLocks(existingGroupComponents(st, groupID), components); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		legacy := legacyFromComponents(components)

		state := store.DesiredStateGroup{
			GroupID:          groupID,
			ArtifactID:       legacy.ArtifactID,
			DesiredVersion:   legacy.DesiredVersion,
			DesiredConfigRev: legacy.DesiredConfigRev,
			PolicyJSON:       legacy.Policy,
			ComponentsJSON:   encodeDesiredComponents(components),
			CheckinInterval:  req.CheckinInterval,
			UpdatedAt:        time.Now().UTC(),
		}
		if err := st.UpsertDesiredStateGroup(state); err != nil {
			logger.Printf("upsert desired_state_group error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_group.upsert", "group", groupID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_group.upsert", "group", groupID)
		event.AfterJSON = auditJSON(map[string]any{
			"groupId":          state.GroupID,
			"artifactId":       state.ArtifactID,
			"desiredVersion":   state.DesiredVersion,
			"desiredConfigRev": state.DesiredConfigRev,
			"checkinInterval":  state.CheckinInterval,
			"policy":           json.RawMessage(state.PolicyJSON),
			"components":       components,
		})
		writeAudit(logger, st, event, nil)
		if releaseAuto != nil {
			releaseAuto.Trigger("desired_state_group_upsert")
		}

		resp := DesiredStateGroupResponse{
			GroupID:          state.GroupID,
			ArtifactID:       state.ArtifactID,
			DesiredVersion:   state.DesiredVersion,
			DesiredConfigRev: state.DesiredConfigRev,
			Policy:           state.PolicyJSON,
			Components:       components,
			CheckinInterval:  state.CheckinInterval,
			UpdatedAt:        state.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PutDesiredStateDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return putDesiredStateDevice(logger, st, trustProxy, ArtifactSignaturePolicy{}, nil)
}

func PutDesiredStateDeviceWithPolicy(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto desiredStateReleaseAutoTrigger) http.HandlerFunc {
	return putDesiredStateDevice(logger, st, trustProxy, sigPolicy, releaseAuto)
}

func putDesiredStateDevice(logger *log.Logger, st store.Store, trustProxy bool, sigPolicy ArtifactSignaturePolicy, releaseAuto desiredStateReleaseAutoTrigger) http.HandlerFunc {
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
		if len(req.Components) == 0 && req.DesiredVersion == "" && req.ArtifactID == "" {
			if req.DesiredConfigRev != "" || len(req.Policy) > 0 {
				http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
				return
			}
			if req.DesiredConfigRev == "" && len(req.Policy) == 0 && req.CheckinInterval == 0 {
				http.Error(w, "artifactId required when desiredVersion is empty", http.StatusBadRequest)
				return
			}
		}
		components, err := normalizeDesiredComponents(req, "manual", st, sigPolicy)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := st.GetDesiredStateDevice(deviceID)
		if err != nil {
			logger.Printf("get desired_state_device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if ok {
			existingComponents := decodeDesiredComponents(existing.ComponentsJSON)
			if err := enforceComponentLocks(existingComponents, components); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		legacy := legacyFromComponents(components)

		state := store.DesiredStateDevice{
			DeviceID:         deviceID,
			ArtifactID:       legacy.ArtifactID,
			DesiredVersion:   legacy.DesiredVersion,
			DesiredConfigRev: legacy.DesiredConfigRev,
			PolicyJSON:       legacy.Policy,
			ComponentsJSON:   encodeDesiredComponents(components),
			CheckinInterval:  req.CheckinInterval,
			Source:           "manual",
			UpdatedAt:        time.Now().UTC(),
		}
		if err := st.UpsertDesiredStateDevice(state); err != nil {
			logger.Printf("upsert desired_state_device error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_device.upsert", "device", deviceID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_device.upsert", "device", deviceID)
		event.AfterJSON = auditJSON(map[string]any{
			"deviceId":         state.DeviceID,
			"artifactId":       state.ArtifactID,
			"desiredVersion":   state.DesiredVersion,
			"desiredConfigRev": state.DesiredConfigRev,
			"checkinInterval":  state.CheckinInterval,
			"policy":           json.RawMessage(state.PolicyJSON),
			"components":       components,
			"source":           state.Source,
		})
		writeAudit(logger, st, event, nil)
		if releaseAuto != nil {
			releaseAuto.Trigger("desired_state_device_upsert")
		}

		resp := DesiredStateDeviceResponse{
			DeviceID:         state.DeviceID,
			ArtifactID:       state.ArtifactID,
			DesiredVersion:   state.DesiredVersion,
			DesiredConfigRev: state.DesiredConfigRev,
			Policy:           state.PolicyJSON,
			Components:       components,
			CheckinInterval:  state.CheckinInterval,
			Source:           state.Source,
			UpdatedAt:        state.UpdatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeleteDesiredStateDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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
		if err := st.DeleteDesiredStateDevice(deviceID); err != nil {
			logger.Printf("delete desired_state_device error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_device.delete", "device", deviceID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_device.delete", "device", deviceID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func DeleteDesiredStateGroup(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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
		if err := st.DeleteDesiredStateGroup(groupID); err != nil {
			logger.Printf("delete desired_state_group error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_group.delete", "group", groupID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state_group.delete", "group", groupID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func ListDesiredState(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		resp := DesiredStateListResponse{}

		// Apply pagination to avoid returning unbounded results.
		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")
		pageLimit := 500
		pageOffset := 0
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 1000 {
			pageLimit = v
		}
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			pageOffset = v
		}

		if scope == "" || scope == "group" {
			groups, err := st.ListDesiredStateGroups()
			if err != nil {
				logger.Printf("list desired_state_group error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			resp.GroupTotal = len(groups)
			start := pageOffset
			if start > len(groups) {
				start = len(groups)
			}
			end := start + pageLimit
			if end > len(groups) {
				end = len(groups)
			}
			groups = groups[start:end]
			resp.Groups = make([]DesiredStateGroupResponse, 0, len(groups))
			for _, g := range groups {
				components := decodeDesiredComponents(g.ComponentsJSON)
				components = mergeLegacyDesiredComponents(components, g.ArtifactID, g.DesiredVersion, g.DesiredConfigRev, g.PolicyJSON, "")
				resp.Groups = append(resp.Groups, DesiredStateGroupResponse{
					GroupID:          g.GroupID,
					ArtifactID:       g.ArtifactID,
					DesiredVersion:   g.DesiredVersion,
					DesiredConfigRev: g.DesiredConfigRev,
					Policy:           g.PolicyJSON,
					Components:       components,
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
			resp.DeviceTotal = len(devices)
			start := pageOffset
			if start > len(devices) {
				start = len(devices)
			}
			end := start + pageLimit
			if end > len(devices) {
				end = len(devices)
			}
			devices = devices[start:end]
			resp.Devices = make([]DesiredStateDeviceResponse, 0, len(devices))
			for _, d := range devices {
				components := decodeDesiredComponents(d.ComponentsJSON)
				components = mergeLegacyDesiredComponents(components, d.ArtifactID, d.DesiredVersion, d.DesiredConfigRev, d.PolicyJSON, d.Source)
				resp.Devices = append(resp.Devices, DesiredStateDeviceResponse{
					DeviceID:         d.DeviceID,
					ArtifactID:       d.ArtifactID,
					DesiredVersion:   d.DesiredVersion,
					DesiredConfigRev: d.DesiredConfigRev,
					Policy:           d.PolicyJSON,
					Components:       components,
					CheckinInterval:  d.CheckinInterval,
					Source:           d.Source,
					UpdatedAt:        d.UpdatedAt,
				})
			}
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "desired_state.list", "desired_state", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"scope":       scope,
			"groupCount":  len(resp.Groups),
			"deviceCount": len(resp.Devices),
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

type legacyDesired struct {
	ArtifactID       string
	DesiredVersion   string
	DesiredConfigRev string
	Policy           []byte
}

func normalizeDesiredComponents(req DesiredStateRequest, source string, st store.Store, sigPolicy ArtifactSignaturePolicy) (map[string]DesiredComponentResponse, error) {
	components := map[string]DesiredComponentResponse{}
	for rawKey, comp := range req.Components {
		key := normalizeDesiredComponentKey(rawKey)
		if key == "" {
			return nil, fmt.Errorf("component name is required")
		}
		if _, exists := components[key]; exists {
			return nil, fmt.Errorf("component %s is duplicated", key)
		}
		if comp.ArtifactID == "" && comp.DesiredVersion == "" && comp.DesiredConfigRev == "" && len(comp.Policy) == 0 {
			return nil, fmt.Errorf("component %s requires artifactId, desiredVersion, desiredConfigRev, or policy", key)
		}
		artifactType, err := resolveArtifactType(comp.ArtifactID, comp.ArtifactType, st)
		if err != nil {
			return nil, fmt.Errorf("component %s: %w", key, err)
		}
		if comp.ArtifactID != "" && st != nil {
			artifact, ok, err := st.GetArtifact(comp.ArtifactID)
			if err != nil {
				return nil, fmt.Errorf("component %s: artifact lookup failed: %w", key, err)
			}
			if !ok {
				return nil, fmt.Errorf("component %s: artifact not found", key)
			}
			attestations, _ := st.ListAttestations(comp.ArtifactID)
			if err := sigPolicy.ValidateDesiredArtifactWithAttestations(artifact, attestations, comp.Policy); err != nil {
				return nil, fmt.Errorf("component %s: %w", key, err)
			}
		}
		if artifactType == "" {
			return nil, fmt.Errorf("component %s requires artifactType", key)
		}
		components[key] = DesiredComponentResponse{
			ArtifactID:       comp.ArtifactID,
			ArtifactType:     artifactType,
			DesiredVersion:   comp.DesiredVersion,
			DesiredConfigRev: comp.DesiredConfigRev,
			Policy:           comp.Policy,
			Source:           source,
			Locked:           comp.Locked,
		}
	}

	if len(components) == 0 && (req.ArtifactID != "" || req.DesiredVersion != "" || req.DesiredConfigRev != "" || len(req.Policy) != 0) {
		if req.ArtifactID != "" && st != nil {
			artifact, ok, err := st.GetArtifact(req.ArtifactID)
			if err != nil {
				return nil, fmt.Errorf("artifact lookup failed: %w", err)
			}
			if !ok {
				return nil, fmt.Errorf("artifact not found")
			}
			attestations, _ := st.ListAttestations(req.ArtifactID)
			if err := sigPolicy.ValidateDesiredArtifactWithAttestations(artifact, attestations, req.Policy); err != nil {
				return nil, err
			}
		}
		components["app_bundle"] = DesiredComponentResponse{
			ArtifactID:       req.ArtifactID,
			ArtifactType:     "app_bundle",
			DesiredVersion:   req.DesiredVersion,
			DesiredConfigRev: req.DesiredConfigRev,
			Policy:           req.Policy,
			Source:           source,
		}
	}
	if len(components) == 0 {
		return nil, nil
	}
	return components, nil
}

func normalizeDesiredComponentKey(val string) string {
	key := strings.TrimSpace(strings.ToLower(val))
	return key
}

func resolveArtifactType(artifactID, requestedType string, st store.Store) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(requestedType))
	if artifactID == "" {
		return normalized, nil
	}
	if st == nil {
		return normalized, nil
	}
	artifact, ok, err := st.GetArtifact(artifactID)
	if err != nil {
		return "", fmt.Errorf("artifact lookup failed: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("artifact not found")
	}
	actualType := strings.TrimSpace(strings.ToLower(artifact.Type))
	if normalized != "" && normalized != actualType {
		return "", fmt.Errorf("artifactType %q does not match artifact type %q", normalized, actualType)
	}
	return actualType, nil
}

func componentEquals(a, b DesiredComponentResponse) bool {
	return a.ArtifactID == b.ArtifactID &&
		a.ArtifactType == b.ArtifactType &&
		a.DesiredVersion == b.DesiredVersion &&
		a.DesiredConfigRev == b.DesiredConfigRev &&
		string(a.Policy) == string(b.Policy)
}

func enforceComponentLocks(existing, incoming map[string]DesiredComponentResponse) error {
	for key, comp := range existing {
		if !comp.Locked {
			continue
		}
		next, ok := incoming[key]
		if !ok {
			return fmt.Errorf("component %s is locked", key)
		}
		if !componentEquals(comp, next) {
			return fmt.Errorf("component %s is locked", key)
		}
	}
	return nil
}

func existingGroupComponents(st store.Store, groupID string) map[string]DesiredComponentResponse {
	if st == nil {
		return nil
	}
	group, ok, err := st.GetDesiredStateGroup(groupID)
	if err != nil || !ok {
		return nil
	}
	return decodeDesiredComponents(group.ComponentsJSON)
}

func legacyFromComponents(components map[string]DesiredComponentResponse) legacyDesired {
	if comp, ok := components["app_bundle"]; ok {
		return legacyDesired{
			ArtifactID:       comp.ArtifactID,
			DesiredVersion:   comp.DesiredVersion,
			DesiredConfigRev: comp.DesiredConfigRev,
			Policy:           comp.Policy,
		}
	}
	return legacyDesired{}
}

func encodeDesiredComponents(components map[string]DesiredComponentResponse) []byte {
	if len(components) == 0 {
		return nil
	}
	out, err := json.Marshal(components)
	if err != nil {
		return nil
	}
	return out
}

func decodeDesiredComponents(raw []byte) map[string]DesiredComponentResponse {
	if len(raw) == 0 {
		return map[string]DesiredComponentResponse{}
	}
	var comps map[string]DesiredComponentResponse
	if err := json.Unmarshal(raw, &comps); err != nil {
		return map[string]DesiredComponentResponse{}
	}
	return comps
}

func mergeLegacyDesiredComponents(components map[string]DesiredComponentResponse, artifactID, desiredVersion, desiredConfigRev string, policy []byte, source string) map[string]DesiredComponentResponse {
	if components == nil {
		components = map[string]DesiredComponentResponse{}
	}
	if artifactID != "" || desiredVersion != "" || desiredConfigRev != "" || len(policy) != 0 {
		if _, ok := components["app_bundle"]; !ok {
			components["app_bundle"] = DesiredComponentResponse{
				ArtifactID:       artifactID,
				ArtifactType:     "app_bundle",
				DesiredVersion:   desiredVersion,
				DesiredConfigRev: desiredConfigRev,
				Policy:           policy,
				Source:           source,
			}
		}
	}
	if comp, ok := components["app_bundle"]; ok && comp.Source == "" && source != "" {
		comp.Source = source
		components["app_bundle"] = comp
	}
	return components
}
