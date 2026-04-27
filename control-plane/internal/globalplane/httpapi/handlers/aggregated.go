package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/parcel/control-plane/internal/globalplane"
)

// ListDevices handles GET /api/v1/devices
// Optional query params: planeId, status
func ListDevices(st interface {
	ListDeviceCache(filter globalplane.DeviceCacheFilter) ([]globalplane.CachedDevice, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := globalplane.DeviceCacheFilter{
			PlaneID: r.URL.Query().Get("planeId"),
			Status:  r.URL.Query().Get("status"),
		}
		devices, err := st.ListDeviceCache(filter)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if devices == nil {
			devices = []globalplane.CachedDevice{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(devices)
	}
}

// ListArtifacts handles GET /api/v1/artifacts
// Optional query params: planeId, name
func ListArtifacts(st interface {
	ListArtifactCache(filter globalplane.ArtifactCacheFilter) ([]globalplane.CachedArtifact, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := globalplane.ArtifactCacheFilter{
			PlaneID: r.URL.Query().Get("planeId"),
			Name:    r.URL.Query().Get("name"),
		}
		arts, err := st.ListArtifactCache(filter)
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if arts == nil {
			arts = []globalplane.CachedArtifact{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(arts)
	}
}

// HealthSummary handles GET /api/v1/health/summary
// Optional query param: planeId (returns only that plane's health + a roll-up)
func HealthSummary(st interface {
	ListHealthCache() ([]globalplane.HealthSnapshot, error)
	ListRegionalPlanes() ([]globalplane.RegionalPlane, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filterPlaneID := r.URL.Query().Get("planeId")

		snaps, err := st.ListHealthCache()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		planes, err := st.ListRegionalPlanes()
		if err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		// Build a name lookup.
		planeNames := make(map[string]string, len(planes))
		for _, p := range planes {
			planeNames[p.PlaneID] = p.Name
		}

		type planeHealth struct {
			PlaneID         string `json:"planeId"`
			PlaneName       string `json:"planeName"`
			TotalDevices    int    `json:"totalDevices"`
			ActiveDevices   int    `json:"activeDevices"`
			StaleDevices    int    `json:"staleDevices"`
			OfflineDevices  int    `json:"offlineDevices"`
			DegradedDevices int    `json:"degradedDevices"`
			SyncedAt        string `json:"syncedAt"`
		}
		type response struct {
			TotalDevices    int           `json:"totalDevices"`
			ActiveDevices   int           `json:"activeDevices"`
			StaleDevices    int           `json:"staleDevices"`
			OfflineDevices  int           `json:"offlineDevices"`
			DegradedDevices int           `json:"degradedDevices"`
			Planes          []planeHealth `json:"planes"`
		}

		var resp response
		for _, s := range snaps {
			if filterPlaneID != "" && s.PlaneID != filterPlaneID {
				continue
			}
			resp.TotalDevices += s.TotalDevices
			resp.ActiveDevices += s.ActiveDevices
			resp.StaleDevices += s.StaleDevices
			resp.OfflineDevices += s.OfflineDevices
			resp.DegradedDevices += s.DegradedDevices
			resp.Planes = append(resp.Planes, planeHealth{
				PlaneID:         s.PlaneID,
				PlaneName:       planeNames[s.PlaneID],
				TotalDevices:    s.TotalDevices,
				ActiveDevices:   s.ActiveDevices,
				StaleDevices:    s.StaleDevices,
				OfflineDevices:  s.OfflineDevices,
				DegradedDevices: s.DegradedDevices,
				SyncedAt:        s.SyncedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		if resp.Planes == nil {
			resp.Planes = []planeHealth{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
