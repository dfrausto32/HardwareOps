package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

type LDAPLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LDAPLogin handles POST /api/v1/auth/ldap/login.
// It authenticates the user against the configured LDAP/AD directory and
// returns a JWT identical to local/OIDC tokens on success.
func LDAPLogin(logger *log.Logger, provider *auth.LDAPProvider, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			http.Error(w, "ldap not configured", http.StatusNotFound)
			return
		}

		var req LDAPLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password required", http.StatusBadRequest)
			return
		}

		token, expiresAt, user, err := provider.Authenticate(r.Context(), req.Username, req.Password)
		if err != nil {
			actor := AuditActor{Type: "anonymous", Email: req.Username, AuthMethod: "ldap"}
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actor, "user.login_failed", "user", req.Username), err)

			if errors.Is(err, auth.ErrLDAPInvalidCredentials) {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			if errors.Is(err, auth.ErrLDAPAccountDisabled) {
				http.Error(w, "account disabled", http.StatusForbidden)
				return
			}
			logger.Printf("ldap login error: %v", err)
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		writeAudit(logger, st, buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "user",
			ID:         user.UserID,
			Email:      user.Email,
			Roles:      rolesFromJSON(user.RolesJSON),
			AuthMethod: "ldap",
		}, "user.login", "user", user.UserID), nil)

		resp := LoginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      userView(*user),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
