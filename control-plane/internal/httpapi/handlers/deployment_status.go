package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/parcel/control-plane/internal/store"
)

type deviceRolloutStatus struct {
	DeviceID       string     `json:"deviceId"`
	Status         string     `json:"status"`
	AppliedVersion string     `json:"appliedVersion,omitempty"`
	Error          string     `json:"error,omitempty"`
	LastApplyAt    *time.Time `json:"lastApplyAt,omitempty"`
}

type groupDeploymentStatusResponse struct {
	GroupID    string                `json:"groupId"`
	ArtifactID string                `json:"artifactId"`
	Total      int                   `json:"total"`
	Pending    int                   `json:"pending"`
	Applied    int                   `json:"applied"`
	Failed     int                   `json:"failed"`
	Complete   bool                  `json:"complete"`
	Devices    []deviceRolloutStatus `json:"devices"`
}

// GetGroupDeploymentStatus returns per-device rollout state for the requested
// artifact across all non-decommissioned devices that match the group selector.
//
// GET /api/v1/groups/{groupId}/deployment-status?artifactId=<id>
//
// Response shape:
//
//	{
//	  "groupId": "...", "artifactId": "...",
//	  "total": 10, "pending": 3, "applied": 6, "failed": 1,
//	  "complete": false,
//	  "devices": [{"deviceId":"...","status":"applied","appliedVersion":"1.2.3"}, ...]
//	}
//
// "complete" is true when every device in the group has been attempted
// (pending == 0) and the group is non-empty.
func GetGroupDeploymentStatus(logger *log.Logger, st store.Store, _ bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		artifactID := r.URL.Query().Get("artifactId")
		if artifactID == "" {
			http.Error(w, "artifactId query parameter required", http.StatusBadRequest)
			return
		}

		// Verify the group exists so callers get a meaningful 404.
		if _, ok, err := st.GetGroup(groupID); err != nil {
			logger.Printf("get group %s: %v", groupID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}

		statuses, err := st.GetGroupDeploymentStatus(groupID, artifactID)
		if err != nil {
			logger.Printf("get group deployment status group=%s artifact=%s: %v", groupID, artifactID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		var pending, applied, failed int
		devices := make([]deviceRolloutStatus, 0, len(statuses))
		for _, s := range statuses {
			switch s.Status {
			case "success":
				applied++
			case "error":
				failed++
			default:
				pending++
			}
			ds := deviceRolloutStatus{
				DeviceID:       s.DeviceID,
				Status:         s.Status,
				AppliedVersion: s.AppliedVersion,
				Error:          s.Error,
			}
			if !s.LastApplyAt.IsZero() {
				t := s.LastApplyAt
				ds.LastApplyAt = &t
			}
			devices = append(devices, ds)
		}
		total := len(statuses)

		resp := groupDeploymentStatusResponse{
			GroupID:    groupID,
			ArtifactID: artifactID,
			Total:      total,
			Pending:    pending,
			Applied:    applied,
			Failed:     failed,
			Complete:   total > 0 && pending == 0,
			Devices:    devices,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Printf("encode deployment status: %v", err)
		}
	}
}
