package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/releaseautoupdate"
	"github.com/parcel/control-plane/internal/store"
)

type ReleaseAutoUpdateSettingsRequest struct {
	Enabled       bool `json:"enabled"`
	AllowUnsigned bool `json:"allowUnsigned"`
}

type ReleaseAutoUpdateStatusResponse struct {
	Enabled         bool                          `json:"enabled"`
	AllowUnsigned   bool                          `json:"allowUnsigned"`
	IntervalSeconds int64                         `json:"intervalSeconds"`
	Running         bool                          `json:"running"`
	UpdatedAt       *time.Time                    `json:"updatedAt,omitempty"`
	UpdatedByUserID string                        `json:"updatedByUserId,omitempty"`
	LastRun         *releaseautoupdate.RunSummary `json:"lastRun,omitempty"`
}

func GetReleaseAutoUpdateStatus(logger *log.Logger, st store.Store, mgr *releaseautoupdate.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var status releaseautoupdate.Status
		if mgr != nil {
			status = mgr.Status()
		} else {
			settings, err := st.GetReleaseAutoUpdateSettings()
			if err != nil {
				logger.Printf("get release auto-update settings error: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
			status = releaseautoupdate.Status{
				Enabled:         settings.Enabled,
				AllowUnsigned:   settings.AllowUnsigned,
				IntervalSeconds: 0,
				Running:         false,
				UpdatedByUserID: settings.UpdatedByUserID,
			}
			if !settings.UpdatedAt.IsZero() {
				t := settings.UpdatedAt
				status.UpdatedAt = &t
			}
		}
		writeJSON(w, ReleaseAutoUpdateStatusResponse{
			Enabled:         status.Enabled,
			AllowUnsigned:   status.AllowUnsigned,
			IntervalSeconds: status.IntervalSeconds,
			Running:         status.Running,
			UpdatedAt:       status.UpdatedAt,
			UpdatedByUserID: status.UpdatedByUserID,
			LastRun:         status.LastRun,
		})
	}
}

func SetReleaseAutoUpdateSettings(logger *log.Logger, st store.Store, mgr *releaseautoupdate.Manager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ReleaseAutoUpdateSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		updatedBy := ""
		if user, ok := auth.UserFromContext(r.Context()); ok {
			updatedBy = user.UserID
		} else {
			updatedBy = "admin"
		}
		settings, err := st.SetReleaseAutoUpdateSettings(store.ReleaseAutoUpdateSettings{
			Enabled:         req.Enabled,
			AllowUnsigned:   req.AllowUnsigned,
			UpdatedByUserID: updatedBy,
		})
		if err != nil {
			logger.Printf("set release auto-update settings error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "release_auto_update.settings.update", "release_auto_update", "global"), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if mgr != nil {
			mgr.Trigger("settings_update")
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "release_auto_update.settings.update", "release_auto_update", "global")
		event.AfterJSON = auditJSON(map[string]any{
			"enabled":       settings.Enabled,
			"allowUnsigned": settings.AllowUnsigned,
			"updatedBy":     settings.UpdatedByUserID,
		})
		writeAudit(logger, st, event, nil)

		writeJSON(w, ReleaseAutoUpdateStatusResponse{
			Enabled:         settings.Enabled,
			AllowUnsigned:   settings.AllowUnsigned,
			UpdatedAt:       timePtr(settings.UpdatedAt),
			UpdatedByUserID: settings.UpdatedByUserID,
			IntervalSeconds: func() int64 {
				if mgr == nil {
					return 0
				}
				return mgr.Status().IntervalSeconds
			}(),
		})
	}
}

func RunReleaseAutoUpdate(logger *log.Logger, st store.Store, mgr *releaseautoupdate.Manager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "release auto-update not configured", http.StatusNotImplemented)
			return
		}
		run, err := mgr.RunNow("manual_api")
		if err != nil {
			logger.Printf("release auto-update run error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "release_auto_update.run", "release_auto_update", "global"), err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "release_auto_update.run", "release_auto_update", "global")
		event.MetadataJSON = auditJSON(run)
		writeAudit(logger, st, event, nil)
		writeJSON(w, run)
	}
}
