package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

type WorkloadIdentityExchangeRequest struct {
	Provider string   `json:"provider"`
	IDToken  string   `json:"idToken"`
	Scopes   []string `json:"scopes"`
}

type WorkloadIdentityExchangeResponse struct {
	Token     string            `json:"token"`
	ExpiresAt time.Time         `json:"expiresAt"`
	Scopes    []string          `json:"scopes"`
	Provider  string            `json:"provider"`
	Subject   string            `json:"subject"`
	Name      string            `json:"name"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type WorkloadIdentityStatusResponse struct {
	Enabled   bool                                `json:"enabled"`
	Providers []auth.WorkloadIdentityProviderInfo `json:"providers,omitempty"`
}

func ExchangeWorkloadIdentityToken(logger *log.Logger, authManager *auth.Manager, exchanger auth.WorkloadIdentityExchanger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authManager == nil || !authManager.Enabled() || exchanger == nil {
			http.Error(w, "workload identity disabled", http.StatusNotFound)
			return
		}
		var req WorkloadIdentityExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Provider = strings.TrimSpace(req.Provider)
		req.IDToken = strings.TrimSpace(req.IDToken)
		if req.IDToken == "" {
			http.Error(w, "idToken required", http.StatusBadRequest)
			return
		}
		session, err := exchanger.Exchange(r.Context(), req.Provider, req.IDToken, req.Scopes)
		if err != nil {
			event := buildAuditEvent(r, trustProxy, AuditActor{
				Type:       "anonymous",
				AuthMethod: "workload_identity",
			}, "auth.workload_identity.exchange.failed", "workload_identity", strings.TrimSpace(req.Provider))
			event.MetadataJSON = auditJSON(map[string]any{
				"provider": req.Provider,
				"scopes":   req.Scopes,
			})
			writeAudit(logger, st, event, err)
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		token, expiresAt, err := authManager.IssueWorkloadIdentityToken(session)
		if err != nil {
			event := buildAuditEvent(r, trustProxy, AuditActor{
				Type:       "workload_identity",
				ID:         session.Subject,
				Email:      session.Name,
				Roles:      session.Scopes,
				AuthMethod: "workload_identity",
			}, "auth.workload_identity.exchange.failed", "workload_identity", session.Subject)
			event.MetadataJSON = auditJSON(map[string]any{
				"provider": session.Provider,
				"metadata": session.Metadata,
			})
			writeAudit(logger, st, event, err)
			http.Error(w, "token issuance error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "workload_identity",
			ID:         session.Subject,
			Email:      session.Name,
			Roles:      session.Scopes,
			AuthMethod: "workload_identity",
		}, "auth.workload_identity.exchange", "workload_identity", session.Subject)
		event.MetadataJSON = auditJSON(map[string]any{
			"provider":  session.Provider,
			"scopes":    session.Scopes,
			"expiresAt": expiresAt,
			"metadata":  session.Metadata,
		})
		writeAudit(logger, st, event, nil)
		writeJSON(w, WorkloadIdentityExchangeResponse{
			Token:     token,
			ExpiresAt: expiresAt.UTC(),
			Scopes:    session.Scopes,
			Provider:  session.Provider,
			Subject:   session.Subject,
			Name:      session.Name,
			Metadata:  session.Metadata,
		})
	}
}

func GetWorkloadIdentityStatus(exchanger *auth.WorkloadIdentityManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := WorkloadIdentityStatusResponse{
			Enabled: exchanger != nil && len(exchanger.ProviderInfos()) > 0,
		}
		if exchanger != nil {
			resp.Providers = exchanger.ProviderInfos()
		}
		writeJSON(w, resp)
	}
}
