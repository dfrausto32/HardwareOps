package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

type loginStore interface {
	CreateAuditEvent(event store.AuditEvent) error
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token        string    `json:"token,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
	TOTPRequired bool      `json:"totpRequired,omitempty"`
	PendingToken string    `json:"pendingToken,omitempty"`
}

// loginClock is the time source for login backoff bookkeeping.
// Package-level so tests can drive it deterministically.
var loginClock = func() time.Time { return time.Now().UTC() }

// Login handles POST /api/v1/auth/login for the global plane.
// It mirrors the regional login handler: login-backoff rate limiting,
// email normalization, TOTP-pending (202), and per-event audit logging.
func Login(logger *log.Logger, mgr *auth.Manager, st loginStore, trustProxy bool, backoff *auth.LoginBackoff) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mgr == nil || !mgr.Enabled() {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" || req.Password == "" {
			http.Error(w, "email and password required", http.StatusBadRequest)
			return
		}

		now := loginClock()
		if backoff != nil {
			if blocked, retry := backoff.Check(req.Email, now); blocked {
				setRetryAfter(w, retry)
				ev := buildAuditEvent(r, trustProxy,
					auditActor{Type: "anonymous", Email: req.Email, AuthMethod: mgr.Mode()},
					"auth.login", "user", req.Email)
				writeAudit(logger, st, ev, errors.New("login backoff active"))
				http.Error(w, "too many login attempts; try again later", http.StatusTooManyRequests)
				return
			}
		}

		user, token, exp, result, err := mgr.Authenticate(req.Email, req.Password)
		if err != nil {
			if backoff != nil {
				retry := backoff.RegisterFailure(req.Email, now)
				setRetryAfter(w, retry)
			}
			ev := buildAuditEvent(r, trustProxy,
				auditActor{Type: "anonymous", Email: req.Email, AuthMethod: mgr.Mode()},
				"auth.login", "user", req.Email)
			writeAudit(logger, st, ev, err)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if backoff != nil {
			backoff.RegisterSuccess(req.Email)
		}

		if result == auth.AuthResultTOTPPending {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(loginResponse{
				TOTPRequired: true,
				PendingToken: token,
				ExpiresAt:    exp,
			})
			ev := buildAuditEvent(r, trustProxy,
				auditActor{Type: "user", ID: user.UserID, Email: user.Email, Roles: rolesFromJSON(user.RolesJSON), AuthMethod: mgr.Mode()},
				"auth.login.totp_pending", "user", user.UserID)
			writeAudit(logger, st, ev, nil)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{Token: token, ExpiresAt: exp})

		ev := buildAuditEvent(r, trustProxy,
			auditActor{Type: "user", ID: user.UserID, Email: user.Email, Roles: rolesFromJSON(user.RolesJSON), AuthMethod: mgr.Mode()},
			"auth.login", "user", user.UserID)
		writeAudit(logger, st, ev, nil)
	}
}

func setRetryAfter(w http.ResponseWriter, retry time.Duration) {
	if retry <= 0 {
		return
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
}

func rolesFromJSON(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var roles []string
	if err := json.Unmarshal(data, &roles); err != nil {
		return nil
	}
	return roles
}
