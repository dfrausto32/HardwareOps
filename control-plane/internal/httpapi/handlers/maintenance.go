package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
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

func SetMaintenance(state MaintenanceState, token string) http.HandlerFunc {
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
		state.Set(req.Enabled, strings.TrimSpace(req.Message))
		enabled, message, updatedAt := state.Get()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MaintenanceResponse{
			Enabled:   enabled,
			Message:   message,
			UpdatedAt: updatedAt,
		})
	}
}
