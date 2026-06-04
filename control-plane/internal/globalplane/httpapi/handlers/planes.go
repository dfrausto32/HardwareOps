package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
	"github.com/parcel/control-plane/internal/store"
)

type planesStore interface {
	CreateRegionalPlane(p globalplane.RegionalPlane) error
	GetRegionalPlane(planeID string) (globalplane.RegionalPlane, bool, error)
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
	UpdateRegionalPlane(planeID, name, baseURL string, encryptedToken []byte, tlsCAPem string, syncIntervalSeconds int) error
	DeleteRegionalPlane(planeID string) error
	ListAllPolicySyncStatus() ([]globalplane.PolicySyncStatus, error)
	CreateAuditEvent(event store.AuditEvent) error
}

type syncRefresher interface {
	Refresh(ctx interface{ Done() <-chan struct{} }) error
}

// ListPlanes handles GET /api/v1/planes
func ListPlanes(st planesStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planes, err := st.ListRegionalPlanes()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if planes == nil {
			planes = []globalplane.RegionalPlane{}
		}

		// Aggregate policy sync summary per plane: most recent success and any error.
		type syncSummary struct {
			lastSuccessAt *time.Time
			lastError     string
		}
		syncMap := make(map[string]syncSummary)
		if statuses, err := st.ListAllPolicySyncStatus(); err == nil {
			for _, s := range statuses {
				sum := syncMap[s.PlaneID]
				if s.PushError == "" && s.PushedAt != nil {
					if sum.lastSuccessAt == nil || s.PushedAt.After(*sum.lastSuccessAt) {
						sum.lastSuccessAt = s.PushedAt
					}
				} else if s.PushError != "" && sum.lastError == "" {
					sum.lastError = s.PushError
				}
				syncMap[s.PlaneID] = sum
			}
		}

		type planeResponse struct {
			PlaneID             string     `json:"planeId"`
			Name                string     `json:"name"`
			BaseURL             string     `json:"baseUrl"`
			Enabled             bool       `json:"enabled"`
			SyncIntervalSeconds int        `json:"syncIntervalSeconds"`
			LastSyncAt          *time.Time `json:"lastSyncAt"`
			LastSyncError       string     `json:"lastSyncError,omitempty"`
			LastPolicySyncAt    *time.Time `json:"lastPolicySyncAt,omitempty"`
			LastPolicySyncError string     `json:"lastPolicySyncError,omitempty"`
			CreatedBy           string     `json:"createdBy"`
			CreatedAt           time.Time  `json:"createdAt"`
			UpdatedAt           time.Time  `json:"updatedAt"`
		}
		resp := make([]planeResponse, len(planes))
		for i, p := range planes {
			sum := syncMap[p.PlaneID]
			resp[i] = planeResponse{
				PlaneID:             p.PlaneID,
				Name:                p.Name,
				BaseURL:             p.BaseURL,
				Enabled:             p.Enabled,
				SyncIntervalSeconds: p.SyncIntervalSeconds,
				LastSyncAt:          p.LastSyncAt,
				LastSyncError:       p.LastSyncError,
				LastPolicySyncAt:    sum.lastSuccessAt,
				LastPolicySyncError: sum.lastError,
				CreatedBy:           p.CreatedBy,
				CreatedAt:           p.CreatedAt,
				UpdatedAt:           p.UpdatedAt,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// RegisterPlane handles POST /api/v1/planes
func RegisterPlane(st planesStore, encKey []byte, syncMgr syncRefresher, logger *log.Logger, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name                string `json:"name"`
			BaseURL             string `json:"baseUrl"`
			ServiceToken        string `json:"serviceToken"`
			TLSCAPem            string `json:"tlsCaPem"`
			SyncIntervalSeconds int    `json:"syncIntervalSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.BaseURL == "" || req.ServiceToken == "" {
			http.Error(w, "name, baseUrl, and serviceToken are required", http.StatusBadRequest)
			return
		}
		if req.SyncIntervalSeconds == 0 {
			req.SyncIntervalSeconds = 60
		}
		enc, err := globalplane.EncryptToken(req.ServiceToken, encKey)
		if err != nil {
			http.Error(w, "token encryption failed", http.StatusInternalServerError)
			return
		}
		p := globalplane.RegionalPlane{
			PlaneID:             uuid.NewString(),
			Name:                req.Name,
			BaseURL:             req.BaseURL,
			EncryptedToken:      enc,
			TLSCAPem:            req.TLSCAPem,
			Enabled:             true,
			SyncIntervalSeconds: req.SyncIntervalSeconds,
		}
		if err := st.CreateRegionalPlane(p); err != nil {
			ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.register", "plane", p.PlaneID)
			writeAudit(logger, st, ev, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = syncMgr.Refresh(r.Context())

		ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.register", "plane", p.PlaneID)
		ev.AfterJSON = auditJSON(map[string]any{"name": p.Name, "baseUrl": p.BaseURL, "syncIntervalSeconds": p.SyncIntervalSeconds})
		writeAudit(logger, st, ev, nil)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"planeId": p.PlaneID})
	}
}

// GetPlane handles GET /api/v1/planes/{planeId}
func GetPlane(st planesStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planeID := chi.URLParam(r, "planeId")
		p, ok, err := st.GetRegionalPlane(planeID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		p.EncryptedToken = nil // redact
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	}
}

// UpdatePlane handles PATCH /api/v1/planes/{planeId}
func UpdatePlane(st planesStore, encKey []byte, syncMgr syncRefresher, logger *log.Logger, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planeID := chi.URLParam(r, "planeId")
		existing, ok, err := st.GetRegionalPlane(planeID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req struct {
			Name                string `json:"name"`
			BaseURL             string `json:"baseUrl"`
			ServiceToken        string `json:"serviceToken"` // optional — omit to keep current
			TLSCAPem            string `json:"tlsCaPem"`
			SyncIntervalSeconds int    `json:"syncIntervalSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			req.Name = existing.Name
		}
		if req.BaseURL == "" {
			req.BaseURL = existing.BaseURL
		}
		if req.SyncIntervalSeconds == 0 {
			req.SyncIntervalSeconds = existing.SyncIntervalSeconds
		}
		enc := existing.EncryptedToken
		if req.ServiceToken != "" {
			enc, err = globalplane.EncryptToken(req.ServiceToken, encKey)
			if err != nil {
				http.Error(w, "token encryption failed", http.StatusInternalServerError)
				return
			}
		}
		tlsCAPem := existing.TLSCAPem
		if req.TLSCAPem != "" {
			tlsCAPem = req.TLSCAPem
		}
		if err := st.UpdateRegionalPlane(planeID, req.Name, req.BaseURL, enc, tlsCAPem, req.SyncIntervalSeconds); err != nil {
			ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.update", "plane", planeID)
			writeAudit(logger, st, ev, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = syncMgr.Refresh(r.Context())

		ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.update", "plane", planeID)
		ev.AfterJSON = auditJSON(map[string]any{"name": req.Name, "baseUrl": req.BaseURL})
		writeAudit(logger, st, ev, nil)

		w.WriteHeader(http.StatusNoContent)
	}
}

// DeletePlane handles DELETE /api/v1/planes/{planeId}
func DeletePlane(st planesStore, syncMgr syncRefresher, logger *log.Logger, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planeID := chi.URLParam(r, "planeId")
		existing, ok, err := st.GetRegionalPlane(planeID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := st.DeleteRegionalPlane(planeID); err != nil {
			ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.delete", "plane", planeID)
			writeAudit(logger, st, ev, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = syncMgr.Refresh(r.Context())

		ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "local"}, "plane.delete", "plane", planeID)
		ev.BeforeJSON = auditJSON(map[string]any{"name": existing.Name, "baseUrl": existing.BaseURL})
		writeAudit(logger, st, ev, nil)

		w.WriteHeader(http.StatusNoContent)
	}
}
