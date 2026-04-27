package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// OIDCLogin generates a state token, sets the state cookie, and redirects to the IdP.
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

// OIDCCallback handles the IdP redirect, exchanges the code for a token, and
// returns a LoginResponse JSON or redirects the SPA to /?oidc_token=<jwt>.
func OIDCCallback(logger *log.Logger, provider *auth.OIDCProvider, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedState, err := auth.ReadStateCookie(r)
		auth.ClearStateCookie(w)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", ""),
				err)
			http.Error(w, "oidc state cookie missing", http.StatusBadRequest)
			return
		}

		stateParam := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", ""),
				newOIDCError(errParam, errDesc))
			http.Error(w, "oidc error: "+errParam, http.StatusUnauthorized)
			return
		}

		token, expiresAt, user, err := provider.Exchange(r.Context(), code, stateParam, expectedState)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", ""),
				err)
			http.Error(w, "oidc authentication failed", http.StatusUnauthorized)
			return
		}

		writeAudit(logger, st, buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "user",
			ID:         user.UserID,
			Email:      user.Email,
			Roles:      rolesFromJSON(user.RolesJSON),
			AuthMethod: "oidc",
		}, "auth.oidc.login", "user", user.UserID), nil)

		// Use the URL fragment so the token is not sent to the server and not
		// recorded in proxy access logs or server logs.
		redirectURL := "/#oidc_token=" + token
		_ = expiresAt
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// OIDCCallbackJSON is an alternative callback that returns a JSON LoginResponse
// (for non-SPA / API clients that hit the callback directly).
func OIDCCallbackJSON(logger *log.Logger, provider *auth.OIDCProvider, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expectedState, err := auth.ReadStateCookie(r)
		auth.ClearStateCookie(w)
		if err != nil {
			http.Error(w, "oidc state cookie missing", http.StatusBadRequest)
			return
		}

		stateParam := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")

		token, expiresAt, user, err := provider.Exchange(r.Context(), code, stateParam, expectedState)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", AuthMethod: "oidc"},
				"auth.oidc.login.failed", "user", ""),
				err)
			http.Error(w, "oidc authentication failed", http.StatusUnauthorized)
			return
		}

		writeAudit(logger, st, buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "user",
			ID:         user.UserID,
			Email:      user.Email,
			Roles:      rolesFromJSON(user.RolesJSON),
			AuthMethod: "oidc",
		}, "auth.oidc.login", "user", user.UserID), nil)

		resp := LoginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      userView(*user),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

type oidcError struct {
	code string
	desc string
}

func (e *oidcError) Error() string {
	if e.desc != "" {
		return e.code + ": " + e.desc
	}
	return e.code
}

func newOIDCError(code, desc string) error {
	return &oidcError{code: code, desc: desc}
}
