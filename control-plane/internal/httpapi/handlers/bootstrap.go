package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/certs"
	"github.com/hardwareops/control-plane/internal/store"
)

type BootstrapStatusResponse struct {
	Enabled       bool   `json:"enabled"`
	AuthEnabled   bool   `json:"authEnabled"`
	TokenRequired bool   `json:"tokenRequired"`
	TokenHeader   string `json:"tokenHeader,omitempty"`
	CADownloadURL string `json:"caDownloadUrl,omitempty"`
}

func BootstrapStatus(authEnabled bool, bootstrapToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		tokenConfigured := strings.TrimSpace(bootstrapToken) != ""
		resp := BootstrapStatusResponse{
			Enabled:       authEnabled || tokenConfigured,
			AuthEnabled:   authEnabled,
			TokenRequired: tokenConfigured,
			CADownloadURL: "/api/v1/bootstrap/ca",
		}
		if tokenConfigured {
			resp.TokenHeader = "X-Bootstrap-Token"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DownloadBootstrapCA(logger *log.Logger, st store.Store, mgr *certs.Manager, bootstrapToken string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil {
			http.Error(w, "cert manager not configured", http.StatusServiceUnavailable)
			return
		}

		tokenConfigured := strings.TrimSpace(bootstrapToken) != ""
		actor := AuditActor{Type: "bootstrap", ID: "bootstrap-token", AuthMethod: "bootstrap_token"}
		if _, ok := auth.UserFromContext(r.Context()); ok {
			actor = actorUser("local")
		} else {
			if !tokenConfigured {
				http.Error(w, "bootstrap token not configured", http.StatusNotFound)
				return
			}
			presented := strings.TrimSpace(r.Header.Get("X-Bootstrap-Token"))
			if presented == "" {
				presented = strings.TrimSpace(r.URL.Query().Get("token"))
			}
			if subtle.ConstantTimeCompare([]byte(presented), []byte(strings.TrimSpace(bootstrapToken))) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		caPEM := mgr.CACertPEM()
		if len(caPEM) == 0 {
			http.Error(w, "CA certificate not available", http.StatusServiceUnavailable)
			return
		}

		event := buildAuditEvent(r, trustProxy, actor, "bootstrap.ca.download", "cert", "ca")
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=\"hardwareops-ca.crt\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(caPEM)
	}
}
