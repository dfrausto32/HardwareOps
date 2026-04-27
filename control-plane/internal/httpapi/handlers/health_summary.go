package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/parcel/control-plane/internal/store"
)

type HealthSummary struct {
	GeneratedAt time.Time           `json:"generatedAt"`
	Devices     HealthDeviceSummary `json:"devices"`
	Errors      map[string]string   `json:"errors,omitempty"`
}

type HealthDeviceSummary struct {
	Total    int        `json:"total"`
	Active   int        `json:"active"`
	Stale    int        `json:"stale"`
	Offline  int        `json:"offline"`
	Degraded int        `json:"degraded"`
	LastSeen *time.Time `json:"lastSeen,omitempty"`
}

func HealthSummaryHandler(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthSummary{
			GeneratedAt: time.Now().UTC(),
			Devices:     HealthDeviceSummary{},
			Errors:      map[string]string{},
		}
		if st == nil {
			resp.Errors["store"] = "store not configured"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if total, err := st.CountDevices(); err != nil {
			resp.Errors["devices.total"] = err.Error()
			if logger != nil {
				logger.Printf("health summary total error: %v", err)
			}
		} else {
			resp.Devices.Total = total
		}
		statuses := map[string]*int{
			"active":   &resp.Devices.Active,
			"stale":    &resp.Devices.Stale,
			"offline":  &resp.Devices.Offline,
			"degraded": &resp.Devices.Degraded,
		}
		for status, target := range statuses {
			count, err := st.CountDevicesByStatus(status)
			if err != nil {
				resp.Errors["devices."+status] = err.Error()
				if logger != nil {
					logger.Printf("health summary status=%s error: %v", status, err)
				}
				continue
			}
			*target = count
		}
		if lastSeen, err := st.LatestDeviceSeen(); err != nil {
			resp.Errors["devices.lastSeen"] = err.Error()
			if logger != nil {
				logger.Printf("health summary lastSeen error: %v", err)
			}
		} else if !lastSeen.IsZero() {
			resp.Devices.LastSeen = &lastSeen
		}

		if len(resp.Errors) == 0 {
			resp.Errors = nil
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
