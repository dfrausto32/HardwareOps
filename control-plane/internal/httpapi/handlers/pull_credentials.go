package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/parcel/control-plane/internal/artifactingest"
	"github.com/parcel/control-plane/internal/store"
)

func GetPullCredentialStatus(logger *log.Logger, mgr *artifactingest.PullCredentialManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "pull credential manager not configured", http.StatusServiceUnavailable)
			return
		}
		status := mgr.Status()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			logger.Printf("encode pull credential status error: %v", err)
		}
	}
}

func ReloadPullCredentials(logger *log.Logger, st store.Store, mgr *artifactingest.PullCredentialManager, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "pull credential manager not configured", http.StatusServiceUnavailable)
			return
		}
		status, err := mgr.Reload(r.Context())
		if err != nil {
			logger.Printf("reload pull credentials error: %v", err)
			http.Error(w, "reload failed", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "artifact.pull_credentials.reload", "artifact_pull_credentials", "")
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			logger.Printf("encode pull credential reload response error: %v", err)
		}
	}
}
