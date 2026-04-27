package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/globalplane"
)

type planesStore interface {
	CreateRegionalPlane(p globalplane.RegionalPlane) error
	GetRegionalPlane(planeID string) (globalplane.RegionalPlane, bool, error)
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
	UpdateRegionalPlane(planeID, name, baseURL string, encryptedToken []byte, tlsCAPem string, syncIntervalSeconds int) error
	DeleteRegionalPlane(planeID string) error
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
		// Redact encrypted tokens from the response.
		type planeResponse struct {
			PlaneID             string     `json:"planeId"`
			Name                string     `json:"name"`
			BaseURL             string     `json:"baseUrl"`
			Enabled             bool       `json:"enabled"`
			SyncIntervalSeconds int        `json:"syncIntervalSeconds"`
			LastSyncAt          *time.Time `json:"lastSyncAt"`
			LastSyncError       string     `json:"lastSyncError,omitempty"`
			CreatedBy           string     `json:"createdBy"`
			CreatedAt           time.Time  `json:"createdAt"`
			UpdatedAt           time.Time  `json:"updatedAt"`
		}
		resp := make([]planeResponse, len(planes))
		for i, p := range planes {
			resp[i] = planeResponse{
				PlaneID:             p.PlaneID,
				Name:                p.Name,
				BaseURL:             p.BaseURL,
				Enabled:             p.Enabled,
				SyncIntervalSeconds: p.SyncIntervalSeconds,
				LastSyncAt:          p.LastSyncAt,
				LastSyncError:       p.LastSyncError,
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
func RegisterPlane(st planesStore, encKey []byte, syncMgr interface {
	Refresh(ctx interface{ Done() <-chan struct{} }) error
}) http.HandlerFunc {
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
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		// Start the worker for the newly registered plane.
		_ = syncMgr.Refresh(r.Context())
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
func UpdatePlane(st planesStore, encKey []byte, syncMgr interface {
	Refresh(ctx interface{ Done() <-chan struct{} }) error
}) http.HandlerFunc {
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
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = syncMgr.Refresh(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeletePlane handles DELETE /api/v1/planes/{planeId}
func DeletePlane(st planesStore, syncMgr interface {
	Refresh(ctx interface{ Done() <-chan struct{} }) error
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planeID := chi.URLParam(r, "planeId")
		_, ok, err := st.GetRegionalPlane(planeID)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := st.DeleteRegionalPlane(planeID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = syncMgr.Refresh(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}
