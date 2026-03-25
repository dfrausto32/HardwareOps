package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/events"
	"github.com/hardwareops/control-plane/internal/store"
)

type TriggerApplyRequest struct {
	Reason string `json:"reason"`
}

type TriggerApplyResponse struct {
	Triggered int      `json:"triggered"`
	Devices   []string `json:"devices"`
}

// TriggerDeviceApply queues a deploy trigger for a single device.
func TriggerDeviceApply(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, triggerFanoutLimit int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceId")
		var req TriggerApplyRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		if _, found, err := st.GetDevice(deviceID); err != nil {
			logger.Printf("trigger apply: get device %s: %v", deviceID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}

		triggeredBy := triggeredByFromContext(r)
		now := time.Now().UTC()
		if err := st.UpsertDeployTrigger(store.DeployTrigger{
			DeviceID:    deviceID,
			TriggeredBy: triggeredBy,
			TriggeredAt: now,
			Reason:      req.Reason,
		}); err != nil {
			logger.Printf("trigger apply: upsert trigger device=%s: %v", deviceID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		payload := auditJSON(map[string]any{
			"deviceId":    deviceID,
			"triggeredBy": triggeredBy,
			"reason":      req.Reason,
			"deviceCount": 1,
		})
		emitRuntimeEvent(logger, st, hub, events.Event{
			Type:     events.TypeDeploymentTriggered,
			DeviceID: deviceID,
			At:       now,
			Payload:  payload,
		})
		event := buildAuditEvent(r, trustProxy, actorUser(triggeredBy), "deployment.trigger", "device", deviceID)
		event.MetadataJSON = payload
		writeAudit(logger, st, event, nil)

		writeJSON(w, TriggerApplyResponse{Triggered: 1, Devices: []string{deviceID}})
	}
}

// TriggerGroupApply queues a deploy trigger for every device in a group.
func TriggerGroupApply(logger *log.Logger, st store.Store, hub *events.Hub, trustProxy bool, triggerFanoutLimit int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		var req TriggerApplyRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		if triggerFanoutLimit <= 0 {
			triggerFanoutLimit = 500
		}
		devices, err := st.ListDevicesForGroup(groupID, triggerFanoutLimit+1)
		if err != nil {
			logger.Printf("trigger apply: list devices for group %s: %v", groupID, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if len(devices) == 0 {
			// Could be group not found or empty group — check the group exists.
			if _, found, gerr := st.GetGroup(groupID); gerr != nil {
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			} else if !found {
				http.Error(w, "group not found", http.StatusNotFound)
				return
			}
		}
		if len(devices) > triggerFanoutLimit {
			http.Error(w, "group exceeds trigger fanout limit", http.StatusUnprocessableEntity)
			return
		}

		triggeredBy := triggeredByFromContext(r)
		now := time.Now().UTC()
		deviceIDs := make([]string, 0, len(devices))
		for _, d := range devices {
			if d.Status == "decommissioned" {
				continue
			}
			if err := st.UpsertDeployTrigger(store.DeployTrigger{
				DeviceID:    d.DeviceID,
				TriggeredBy: triggeredBy,
				TriggeredAt: now,
				Reason:      req.Reason,
			}); err != nil {
				logger.Printf("trigger apply: upsert trigger device=%s: %v", d.DeviceID, err)
				// Best-effort: continue triggering remaining devices.
			} else {
				deviceIDs = append(deviceIDs, d.DeviceID)
			}
		}

		payload := auditJSON(map[string]any{
			"triggeredBy": triggeredBy,
			"reason":      req.Reason,
			"deviceCount": len(deviceIDs),
			"groupId":     groupID,
		})
		emitRuntimeEvent(logger, st, hub, events.Event{
			Type:    events.TypeDeploymentTriggered,
			At:      now,
			Payload: payload,
		})
		event := buildAuditEvent(r, trustProxy, actorUser(triggeredBy), "deployment.trigger", "group", groupID)
		event.MetadataJSON = payload
		writeAudit(logger, st, event, nil)

		writeJSON(w, TriggerApplyResponse{Triggered: len(deviceIDs), Devices: deviceIDs})
	}
}

// triggeredByFromContext extracts the actor identity (user ID or service token ID).
func triggeredByFromContext(r *http.Request) string {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return u.UserID
	}
	if t, ok := auth.ServiceTokenFromContext(r.Context()); ok {
		return "token:" + t.TokenID
	}
	return "unknown"
}
