package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// oidcStore is the minimal interface needed by the OIDC callback handler for audit.
type oidcStore interface {
	CreateAuditEvent(event store.AuditEvent) error
}

// OIDCLogin generates state, sets the state cookie, and redirects the browser
// to the IdP authorization endpoint.
func OIDCLogin(provider *auth.OIDCProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := auth.GenerateState()
		if err != nil {
			http.Error(w, "failed to generate state", http.StatusInternalServerError)
			return
		}
		auth.SetStateCookie(w, state)
		http.Redirect(w, r, provider.AuthCodeURL(state), http.StatusFound)
	}
}

// OIDCCallback handles the IdP redirect: exchanges the authorization code for a
// JWT, then redirects the browser to postLoginURL + ?global_oidc_token=<jwt>.
// If postLoginURL is empty, a minimal JSON response is returned instead (useful
// for API clients and mock IdP tests).
func OIDCCallback(logger *log.Logger, provider *auth.OIDCProvider, st oidcStore, trustProxy bool, postLoginURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedState, err := auth.ReadStateCookie(r)
		auth.ClearStateCookie(w)
		if err != nil {
			logEv := buildAuditEvent(r, trustProxy,
				auditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", "")
			writeAudit(logger, st, logEv, errors.New("oidc state cookie missing"))
			http.Error(w, "oidc state cookie missing", http.StatusBadRequest)
			return
		}

		stateParam := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			logEv := buildAuditEvent(r, trustProxy,
				auditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", "")
			writeAudit(logger, st, logEv, errors.New(errParam+": "+errDesc))
			http.Error(w, "oidc error: "+errParam, http.StatusUnauthorized)
			return
		}

		token, expiresAt, user, err := provider.Exchange(r.Context(), code, stateParam, expectedState)
		if err != nil {
			logEv := buildAuditEvent(r, trustProxy,
				auditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", "")
			writeAudit(logger, st, logEv, err)
			http.Error(w, "oidc authentication failed", http.StatusUnauthorized)
			return
		}

		logEv := buildAuditEvent(r, trustProxy,
			auditActor{
				Type:       "user",
				ID:         user.UserID,
				Email:      user.Email,
				Roles:      rolesFromJSON(user.RolesJSON),
				AuthMethod: "oidc",
			},
			"auth.oidc.login", "user", user.UserID)
		writeAudit(logger, st, logEv, nil)

		if postLoginURL != "" {
			http.Redirect(w, r, postLoginURL+"?global_oidc_token="+token, http.StatusFound)
			return
		}

		// No postLoginURL — return JSON (mock IdP tests and API clients).
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":     token,
			"expiresAt": expiresAt,
		})
	}
}
