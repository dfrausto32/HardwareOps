package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

type MaintenanceResponse struct {
	Enabled   bool      `json:"enabled"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type MaintenanceRequest struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message,omitempty"`
}

type MaintenanceState interface {
	Get() (bool, string, time.Time)
	Set(enabled bool, message string)
}

func GetMaintenance(state MaintenanceState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enabled, message, updatedAt := false, "", time.Time{}
		if state != nil {
			enabled, message, updatedAt = state.Get()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MaintenanceResponse{
			Enabled:   enabled,
			Message:   message,
			UpdatedAt: updatedAt,
		})
	}
}

func SetMaintenance(logger *log.Logger, st store.Store, state MaintenanceState, token string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if state == nil {
			http.Error(w, "maintenance not configured", http.StatusNotFound)
			return
		}
		if token != "" {
			header := strings.TrimSpace(r.Header.Get("X-Maintenance-Token"))
			if header == "" || header != token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		var req MaintenanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		beforeEnabled, beforeMessage, beforeUpdatedAt := state.Get()
		state.Set(req.Enabled, strings.TrimSpace(req.Message))
		enabled, message, updatedAt := state.Get()
		authMethod := "ui"
		if token != "" {
			authMethod = "maintenance_token"
		}
		event := buildAuditEvent(r, trustProxy, actorUser(authMethod), "maintenance.set", "maintenance", "control-plane")
		event.BeforeJSON = auditJSON(map[string]any{
			"enabled":   beforeEnabled,
			"message":   beforeMessage,
			"updatedAt": beforeUpdatedAt,
		})
		event.AfterJSON = auditJSON(map[string]any{
			"enabled":   enabled,
			"message":   message,
			"updatedAt": updatedAt,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MaintenanceResponse{
			Enabled:   enabled,
			Message:   message,
			UpdatedAt: updatedAt,
		})
	}
}
