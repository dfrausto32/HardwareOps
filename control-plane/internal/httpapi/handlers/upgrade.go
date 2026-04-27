package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/parcel/control-plane/internal/metrics"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/upgrade"
)

type MaintenanceStateView interface {
	Get() (bool, string, time.Time)
}

func GetUpgradeStatus(runner *upgrade.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if runner == nil || !runner.Enabled() {
			_ = json.NewEncoder(w).Encode(upgrade.Status{Enabled: false, State: "disabled"})
			return
		}
		_ = json.NewEncoder(w).Encode(runner.Status())
	}
}

func ApplyUpgrade(logger *log.Logger, st store.Store, runner *upgrade.Runner, maintenance MaintenanceStateView, trustProxy bool, updatesDir string, metricsCollector *metrics.Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil || !runner.Enabled() {
			if metricsCollector != nil {
				metricsCollector.IncUpgrade("error")
			}
			http.Error(w, "upgrade runner not configured", http.StatusNotFound)
			return
		}
		if maintenance == nil {
			http.Error(w, "maintenance state not configured", http.StatusPreconditionFailed)
			return
		}
		enabled, _, _ := maintenance.Get()
		if !enabled {
			http.Error(w, "maintenance mode must be enabled to apply upgrades", http.StatusConflict)
			return
		}
		preflight := BuildUpgradePreflight(runner, updatesDir)
		if !preflight.OK {
			if metricsCollector != nil {
				metricsCollector.IncUpgrade("error")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_ = json.NewEncoder(w).Encode(preflight)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "upgrade.apply", "upgrade", "control-plane")
		status, err := runner.Start()
		if err != nil {
			writeAudit(logger, st, event, err)
			if metricsCollector != nil {
				metricsCollector.IncUpgrade("error")
			}
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if metricsCollector != nil {
			metricsCollector.IncUpgrade("started")
		}
		event.AfterJSON = auditJSON(status)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

type UpgradeAvailableResponse struct {
	Available  bool     `json:"available"`
	UpdatesDir string   `json:"updatesDir,omitempty"`
	Latest     string   `json:"latest,omitempty"`
	Bundles    []string `json:"bundles,omitempty"`
}

func GetUpgradeAvailable(updatesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := UpgradeAvailableResponse{
			Available:  false,
			UpdatesDir: updatesDir,
		}
		if updatesDir == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		matches, err := filepath.Glob(filepath.Join(updatesDir, "parcel-upgrade-*.tar.gz"))
		if err != nil || len(matches) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		sort.Slice(matches, func(i, j int) bool {
			iInfo, iErr := os.Stat(matches[i])
			jInfo, jErr := os.Stat(matches[j])
			if iErr != nil || jErr != nil {
				return matches[i] > matches[j]
			}
			return iInfo.ModTime().After(jInfo.ModTime())
		})
		resp.Available = true
		resp.Latest = filepath.Base(matches[0])
		resp.Bundles = make([]string, 0, len(matches))
		for _, m := range matches {
			resp.Bundles = append(resp.Bundles, filepath.Base(m))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
