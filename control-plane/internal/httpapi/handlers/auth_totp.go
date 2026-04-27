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

// TOTPEnrollResponse is returned by POST /auth/totp/enroll.
// The client should display the OTP URI as a QR code and the plain secret
// as a fallback for manual entry. Neither is stored as enabled yet —
// the caller must confirm with a valid code via POST /auth/totp/confirm.
type TOTPEnrollResponse struct {
	OTPURI string `json:"otpUri"`
	Secret string `json:"secret"`
}

// TOTPConfirmRequest is the body for POST /auth/totp/confirm.
type TOTPConfirmRequest struct {
	Code string `json:"code"`
}

// TOTPConfirmResponse is returned after TOTP is successfully confirmed and enabled.
type TOTPConfirmResponse struct {
	Enabled bool `json:"enabled"`
}

// TOTPVerifyRequest is the body for POST /auth/totp/verify.
type TOTPVerifyRequest struct {
	PendingToken string `json:"pendingToken"`
	Code         string `json:"code"`
}

// TOTPDisableRequest is the body for POST /auth/totp/disable.
// Password confirmation prevents an attacker with a live session from
// silently disabling MFA.
type TOTPDisableRequest struct {
	Password string `json:"password"`
}

// TOTPEnroll generates a new TOTP key for the authenticated user.
// The key is stored encrypted in the DB but totp_enabled remains false until
// the user confirms ownership with a valid code via TOTPConfirm.
//
// POST /auth/totp/enroll  — requires viewer auth
func TOTPEnroll(logger *log.Logger, manager *auth.Manager, st store.Store, encKey []byte, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		if len(encKey) == 0 {
			http.Error(w, "TOTP not configured on this server", http.StatusServiceUnavailable)
			return
		}
		currentUser, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, found, err := st.GetUser(currentUser.UserID)
		if err != nil {
			logger.Printf("totp enroll get user error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		key, err := auth.GenerateTOTPKey(manager.Issuer(), user.Email)
		if err != nil {
			logger.Printf("totp generate key error: %v", err)
			http.Error(w, "failed to generate TOTP key", http.StatusInternalServerError)
			return
		}
		encrypted, err := auth.EncryptTOTPSecret(encKey, key.Secret())
		if err != nil {
			logger.Printf("totp encrypt secret error: %v", err)
			http.Error(w, "failed to encrypt TOTP secret", http.StatusInternalServerError)
			return
		}
		// Store the secret with totp_enabled = false (pending confirmation).
		if err := st.SetUserTOTP(currentUser.UserID, encrypted, false); err != nil {
			logger.Printf("totp store secret error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.enroll", "user", currentUser.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.enroll", "user", currentUser.UserID), nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TOTPEnrollResponse{
			OTPURI: key.URL(),
			Secret: key.Secret(),
		})
	}
}

// TOTPConfirm validates a TOTP code from the authenticator app and, on success,
// enables TOTP for the authenticated user.
//
// POST /auth/totp/confirm  — requires viewer auth
func TOTPConfirm(logger *log.Logger, manager *auth.Manager, st store.Store, encKey []byte, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		if len(encKey) == 0 {
			http.Error(w, "TOTP not configured on this server", http.StatusServiceUnavailable)
			return
		}
		currentUser, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req TOTPConfirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Code = strings.TrimSpace(req.Code)
		if req.Code == "" {
			http.Error(w, "code required", http.StatusBadRequest)
			return
		}
		user, found, err := st.GetUser(currentUser.UserID)
		if err != nil {
			logger.Printf("totp confirm get user error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found || user.TOTPSecret == "" {
			http.Error(w, "TOTP enrollment not started; call POST /auth/totp/enroll first", http.StatusBadRequest)
			return
		}
		rawSecret, err := auth.DecryptTOTPSecret(encKey, user.TOTPSecret)
		if err != nil {
			logger.Printf("totp confirm decrypt error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !auth.ValidateTOTPCode(req.Code, rawSecret) {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.confirm", "user", currentUser.UserID), errTOTPInvalid)
			http.Error(w, "invalid TOTP code", http.StatusBadRequest)
			return
		}
		if err := st.SetUserTOTP(currentUser.UserID, user.TOTPSecret, true); err != nil {
			logger.Printf("totp enable error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.confirm", "user", currentUser.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.confirm", "user", currentUser.UserID), nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TOTPConfirmResponse{Enabled: true})
	}
}

// TOTPVerify completes a two-step login: the client presents the pending token
// received from POST /auth/login (when totpRequired=true) together with the
// TOTP code. On success a full session JWT is issued.
//
// POST /auth/totp/verify  — unauthenticated
func TOTPVerify(logger *log.Logger, manager *auth.Manager, st store.Store, encKey []byte, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		if len(encKey) == 0 {
			http.Error(w, "TOTP not configured on this server", http.StatusServiceUnavailable)
			return
		}
		var req TOTPVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.PendingToken = strings.TrimSpace(req.PendingToken)
		req.Code = strings.TrimSpace(req.Code)
		if req.PendingToken == "" || req.Code == "" {
			http.Error(w, "pendingToken and code required", http.StatusBadRequest)
			return
		}
		userID, err := manager.VerifyTOTPPendingToken(req.PendingToken)
		if err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", AuthMethod: manager.Mode()},
				"auth.totp.verify", "user", ""), err)
			http.Error(w, "invalid or expired pending token", http.StatusUnauthorized)
			return
		}
		user, found, err := st.GetUser(userID)
		if err != nil {
			logger.Printf("totp verify get user error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found || user.Disabled {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !user.TOTPEnabled || user.TOTPSecret == "" {
			// TOTP was disabled between the login and verification steps — issue full token.
			token, exp, err := manager.IssueToken(user)
			if err != nil {
				http.Error(w, "token error", http.StatusInternalServerError)
				return
			}
			_ = st.SetUserLastLogin(user.UserID, time.Now().UTC())
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(LoginResponse{Token: token, ExpiresAt: exp, User: userView(user)})
			return
		}
		rawSecret, err := auth.DecryptTOTPSecret(encKey, user.TOTPSecret)
		if err != nil {
			logger.Printf("totp verify decrypt error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !auth.ValidateTOTPCode(req.Code, rawSecret) {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "user", ID: user.UserID, Email: user.Email, AuthMethod: manager.Mode()},
				"auth.totp.verify", "user", user.UserID), errTOTPInvalid)
			http.Error(w, "invalid TOTP code", http.StatusUnauthorized)
			return
		}
		token, exp, err := manager.IssueToken(user)
		if err != nil {
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		_ = st.SetUserLastLogin(user.UserID, time.Now().UTC())
		event := buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "user",
			ID:         user.UserID,
			Email:      user.Email,
			Roles:      rolesFromJSON(user.RolesJSON),
			AuthMethod: manager.Mode(),
		}, "auth.login", "user", user.UserID)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LoginResponse{Token: token, ExpiresAt: exp, User: userView(user)})
	}
}

// TOTPDisable disables TOTP for the authenticated user after password confirmation.
//
// POST /auth/totp/disable  — requires viewer auth
func TOTPDisable(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		currentUser, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req TOTPDisableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Password) == "" {
			http.Error(w, "password required", http.StatusBadRequest)
			return
		}
		user, found, err := st.GetUser(currentUser.UserID)
		if err != nil {
			logger.Printf("totp disable get user error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := manager.CheckPassword(user, req.Password); err != nil {
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.disable", "user", currentUser.UserID), err)
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		if err := st.SetUserTOTP(currentUser.UserID, "", false); err != nil {
			logger.Printf("totp disable error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.disable", "user", currentUser.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.totp.disable", "user", currentUser.UserID), nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"disabled": true})
	}
}

// errTOTPInvalid is a sentinel for audit logging without exposing internal errors.
var errTOTPInvalid = &totpInvalidErr{}

type totpInvalidErr struct{}

func (e *totpInvalidErr) Error() string { return "invalid TOTP code" }
