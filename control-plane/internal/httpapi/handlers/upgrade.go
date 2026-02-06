package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/upgrade"
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

func ApplyUpgrade(runner *upgrade.Runner, maintenance MaintenanceStateView, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if runner == nil || !runner.Enabled() {
			http.Error(w, "upgrade runner not configured", http.StatusNotFound)
			return
		}
		if token != "" {
			header := strings.TrimSpace(r.Header.Get("X-Maintenance-Token"))
			if header == "" || header != token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
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
		status, err := runner.Start()
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
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
		matches, err := filepath.Glob(filepath.Join(updatesDir, "hardwareops-upgrade-*.tar.gz"))
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
