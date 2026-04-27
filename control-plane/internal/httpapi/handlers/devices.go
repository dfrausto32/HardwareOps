package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

type DeviceSummary struct {
	DeviceID string          `json:"deviceId"`
	Status   string          `json:"status"`
	LastSeen *time.Time      `json:"lastSeen,omitempty"`
	Labels   json.RawMessage `json:"labels,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type DeviceCurrentState struct {
	SoftwareVersion     string                          `json:"softwareVersion"`
	ConfigRev           string                          `json:"configRev"`
	Services            json.RawMessage                 `json:"services,omitempty"`
	Health              json.RawMessage                 `json:"health,omitempty"`
	Components          map[string]DeviceComponentState `json:"components,omitempty"`
	UpdatedAt           *time.Time                      `json:"updatedAt,omitempty"`
	LastApplyStatus     string                          `json:"lastApplyStatus,omitempty"`
	LastApplyError      string                          `json:"lastApplyError,omitempty"`
	LastApplyAt         *time.Time                      `json:"lastApplyAt,omitempty"`
	LastApplyArtifactID string                          `json:"lastApplyArtifactId,omitempty"`
	LastPreApplyStatus  string                          `json:"lastPreApplyStatus,omitempty"`
	LastPreApplyError   string                          `json:"lastPreApplyError,omitempty"`
	LastPreApplyAt      *time.Time                      `json:"lastPreApplyAt,omitempty"`
}

type DeviceDetail struct {
	DeviceSummary
	Current *DeviceCurrentState `json:"current,omitempty"`
}

type DeviceListResponse struct {
	Items  []DeviceSummary `json:"items"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type DeviceUpdateRequest struct {
	Labels   *json.RawMessage `json:"labels,omitempty"`
	Metadata *json.RawMessage `json:"metadata,omitempty"`
}

type DeviceDecommissionRequest struct {
	Reason   string `json:"reason"`
	TicketID string `json:"ticketId,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

type DeviceDecommissionResponse struct {
	DeviceID         string     `json:"deviceId"`
	Reason           string     `json:"reason"`
	TicketID         string     `json:"ticketId,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	SlotBefore       int        `json:"slotBefore"`
	SlotAfter        int        `json:"slotAfter"`
	ReleasedSlot     bool       `json:"releasedSlot"`
	DecommissionedAt *time.Time `json:"decommissionedAt,omitempty"`
}

func ListDevices(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseInt(r.URL.Query().Get("limit"), 100)
		offset := parseInt(r.URL.Query().Get("offset"), 0)
		status := r.URL.Query().Get("status")

		items, err := st.ListDevices(store.ListDevicesFilter{Status: status, Limit: limit, Offset: offset})
		if err != nil {
			logger.Printf("list devices error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		resp := DeviceListResponse{
			Items:  make([]DeviceSummary, 0, len(items)),
			Limit:  limit,
			Offset: offset,
		}
		for _, d := range items {
			resp.Items = append(resp.Items, DeviceSummary{
				DeviceID: d.DeviceID,
				Status:   d.Status,
				LastSeen: timePtr(d.LastSeen),
				Labels:   json.RawMessage(d.LabelsJSON),
				Metadata: json.RawMessage(d.MetadataJSON),
			})
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.list", "device", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"status": status,
			"limit":  limit,
			"offset": offset,
			"count":  len(resp.Items),
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func PatchDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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

		var req DeviceUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Labels == nil && req.Metadata == nil {
			http.Error(w, "labels or metadata required", http.StatusBadRequest)
			return
		}
		if req.Labels != nil {
			if len(*req.Labels) == 0 || !isJSONObject(*req.Labels) {
				http.Error(w, "labels must be a JSON object", http.StatusBadRequest)
				return
			}
		}
		if req.Metadata != nil {
			if len(*req.Metadata) == 0 || !isJSONObject(*req.Metadata) {
				http.Error(w, "metadata must be a JSON object", http.StatusBadRequest)
				return
			}
		}

		device, ok, err := st.GetDevice(deviceID)
		if err != nil {
			logger.Printf("get device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		labels := device.LabelsJSON
		metadata := device.MetadataJSON
		if req.Labels != nil {
			labels = *req.Labels
		}
		if req.Metadata != nil {
			metadata = *req.Metadata
		}

		if err := st.UpsertDevice(store.Device{
			DeviceID:        device.DeviceID,
			CertFingerprint: device.CertFingerprint,
			Status:          device.Status,
			LastSeen:        device.LastSeen,
			LabelsJSON:      labels,
			MetadataJSON:    metadata,
		}); err != nil {
			logger.Printf("upsert device error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "device.update", "device", deviceID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.update", "device", deviceID)
		event.BeforeJSON = auditJSON(map[string]any{
			"labels":   json.RawMessage(device.LabelsJSON),
			"metadata": json.RawMessage(device.MetadataJSON),
		})
		event.AfterJSON = auditJSON(map[string]any{
			"labels":   json.RawMessage(labels),
			"metadata": json.RawMessage(metadata),
		})
		writeAudit(logger, st, event, nil)

		resp := DeviceSummary{
			DeviceID: device.DeviceID,
			Status:   device.Status,
			LastSeen: timePtr(device.LastSeen),
			Labels:   json.RawMessage(labels),
			Metadata: json.RawMessage(metadata),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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

		device, ok, err := st.GetDevice(deviceID)
		if err != nil {
			logger.Printf("get device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var current *DeviceCurrentState
		if stRec, ok, err := st.GetDeviceState(deviceID); err == nil && ok {
			current = &DeviceCurrentState{
				SoftwareVersion:     stRec.CurrentVersion,
				ConfigRev:           stRec.CurrentConfigRev,
				Services:            json.RawMessage(stRec.ServicesJSON),
				Health:              json.RawMessage(stRec.HealthJSON),
				Components:          decodeDeviceComponents(stRec.ComponentsJSON),
				UpdatedAt:           timePtr(stRec.UpdatedAt),
				LastApplyStatus:     stRec.LastApplyStatus,
				LastApplyError:      stRec.LastApplyError,
				LastApplyAt:         timePtr(stRec.LastApplyAt),
				LastApplyArtifactID: stRec.LastApplyArtifactID,
				LastPreApplyStatus:  stRec.LastPreApplyStatus,
				LastPreApplyError:   stRec.LastPreApplyError,
				LastPreApplyAt:      timePtr(stRec.LastPreApplyAt),
			}
		}

		resp := DeviceDetail{
			DeviceSummary: DeviceSummary{
				DeviceID: device.DeviceID,
				Status:   device.Status,
				LastSeen: timePtr(device.LastSeen),
				Labels:   json.RawMessage(device.LabelsJSON),
				Metadata: json.RawMessage(device.MetadataJSON),
			},
			Current: current,
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.read", "device", deviceID)
		event.MetadataJSON = auditJSON(map[string]any{
			"hasState": current != nil,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeleteDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "device delete disabled: use POST /api/v1/devices/{deviceId}/decommission", http.StatusBadRequest)
	}
}

func DecommissionDevice(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
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

		var req DeviceDecommissionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Reason = strings.TrimSpace(req.Reason)
		req.TicketID = strings.TrimSpace(req.TicketID)
		req.Notes = strings.TrimSpace(req.Notes)
		if req.Reason == "" {
			http.Error(w, "reason required", http.StatusBadRequest)
			return
		}

		device, ok, err := st.GetDevice(deviceID)
		if err != nil {
			logger.Printf("get device error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		slotBefore, err := st.CountDevices()
		if err != nil {
			logger.Printf("count devices before decommission error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		if err := st.DeleteDevice(deviceID); err != nil {
			logger.Printf("decommission device error: %v", err)
			event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.decommission", "device", deviceID)
			event.MetadataJSON = auditJSON(map[string]any{
				"reason":     req.Reason,
				"ticketId":   req.TicketID,
				"notes":      req.Notes,
				"slotBefore": slotBefore,
			})
			writeAudit(logger, st, event, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		slotAfter, err := st.CountDevices()
		if err != nil {
			logger.Printf("count devices after decommission error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC()
		resp := DeviceDecommissionResponse{
			DeviceID:         deviceID,
			Reason:           req.Reason,
			TicketID:         req.TicketID,
			Notes:            req.Notes,
			SlotBefore:       slotBefore,
			SlotAfter:        slotAfter,
			ReleasedSlot:     slotAfter < slotBefore,
			DecommissionedAt: &now,
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "device.decommission", "device", deviceID)
		event.BeforeJSON = auditJSON(map[string]any{
			"status":   device.Status,
			"lastSeen": timePtr(device.LastSeen),
			"labels":   json.RawMessage(device.LabelsJSON),
			"metadata": json.RawMessage(device.MetadataJSON),
		})
		event.AfterJSON = auditJSON(map[string]any{
			"decommissioned": true,
			"slotBefore":     slotBefore,
			"slotAfter":      slotAfter,
		})
		event.MetadataJSON = auditJSON(map[string]any{
			"reason":       req.Reason,
			"ticketId":     req.TicketID,
			"notes":        req.Notes,
			"releasedSlot": resp.ReleasedSlot,
		})
		writeAudit(logger, st, event, nil)

		writeJSON(w, resp)
	}
}

func parseInt(val string, def int) int {
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil && i >= 0 {
		return i
	}
	return def
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
