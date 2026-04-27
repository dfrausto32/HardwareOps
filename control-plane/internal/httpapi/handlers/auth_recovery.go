package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/mailer"
	"github.com/parcel/control-plane/internal/store"
)

const recoveryCodeCount = 8

type GenerateRecoveryCodesResponse struct {
	Codes       []string  `json:"codes"`
	GeneratedAt time.Time `json:"generatedAt"`
}

type RecoveryCodeResetRequest struct {
	Email        string `json:"email"`
	RecoveryCode string `json:"recoveryCode"`
	NewPassword  string `json:"newPassword"`
}

type RecoveryCodeResetResponse struct {
	Success bool `json:"success"`
}

type CreatePasswordResetTokenRequest struct {
	TTLMinutes int    `json:"ttlMinutes"`
	Reason     string `json:"reason"`
	SendEmail  bool   `json:"sendEmail"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type SendUserInviteResponse struct {
	TokenID   string    `json:"tokenId"`
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CreatePasswordResetTokenResponse struct {
	TokenID   string    `json:"tokenId"`
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"token"`
}

type PasswordResetTokenCompleteRequest struct {
	Email       string `json:"email"`
	ResetToken  string `json:"resetToken"`
	NewPassword string `json:"newPassword"`
}

type PasswordResetTokenCompleteResponse struct {
	Success bool `json:"success"`
}

func GenerateRecoveryCodes(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
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
		user, found, err := st.GetUser(currentUser.UserID)
		if err != nil {
			logger.Printf("get user for recovery codes error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.recovery_codes.generate", "user", currentUser.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		codes, hashes, err := auth.GenerateRecoveryCodes(recoveryCodeCount)
		if err != nil {
			logger.Printf("generate recovery codes error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.recovery_codes.generate", "user", currentUser.UserID), err)
			http.Error(w, "failed to generate recovery codes", http.StatusInternalServerError)
			return
		}
		hashesJSON, err := json.Marshal(hashes)
		if err != nil {
			http.Error(w, "failed to encode recovery codes", http.StatusInternalServerError)
			return
		}
		generatedAt := time.Now().UTC()
		if err := st.SetUserRecoveryCodes(currentUser.UserID, hashesJSON, generatedAt); err != nil {
			logger.Printf("store recovery codes error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.recovery_codes.generate", "user", currentUser.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.recovery_codes.generate", "user", currentUser.UserID)
		event.AfterJSON = auditJSON(map[string]any{
			"email":          user.Email,
			"generatedCount": len(codes),
			"generatedAt":    generatedAt,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GenerateRecoveryCodesResponse{Codes: codes, GeneratedAt: generatedAt})
	}
}

func ResetPasswordWithRecoveryCode(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req RecoveryCodeResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.RecoveryCode = auth.NormalizeRecoveryCode(req.RecoveryCode)
		if req.Email == "" || req.RecoveryCode == "" || strings.TrimSpace(req.NewPassword) == "" {
			http.Error(w, "email, recoveryCode, and newPassword required", http.StatusBadRequest)
			return
		}
		passwordHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updatedUser, ok, err := st.ConsumeUserRecoveryCode(req.Email, auth.HashToken(req.RecoveryCode), passwordHash, time.Now().UTC())
		if err != nil {
			logger.Printf("consume recovery code error: %v", err)
			writeAudit(logger, st, buildAuditEvent(
				r,
				trustProxy,
				AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "recovery_code"},
				"auth.recovery_codes.reset",
				"user",
				req.Email,
			), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			writeAudit(logger, st, buildAuditEvent(
				r,
				trustProxy,
				AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "recovery_code"},
				"auth.recovery_codes.reset",
				"user",
				req.Email,
			), errors.New("invalid recovery code"))
			http.Error(w, "invalid recovery code", http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "recovery_code",
			ID:         updatedUser.UserID,
			Email:      updatedUser.Email,
			Roles:      rolesFromJSON(updatedUser.RolesJSON),
			AuthMethod: "recovery_code",
		}, "auth.recovery_codes.reset", "user", updatedUser.UserID)
		event.AfterJSON = auditJSON(map[string]any{
			"remainingRecoveryCodes": countRecoveryCodes(updatedUser.RecoveryCodesJSON),
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RecoveryCodeResetResponse{Success: true})
	}
}

func CreatePasswordResetToken(logger *log.Logger, manager *auth.Manager, st store.Store, m mailer.Mailer, appPublicURL string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		userID := strings.TrimSpace(chi.URLParam(r, "userId"))
		if userID == "" {
			http.Error(w, "userId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(userID); err != nil {
			http.Error(w, "userId must be uuid", http.StatusBadRequest)
			return
		}
		var req CreatePasswordResetTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.TTLMinutes <= 0 {
			req.TTLMinutes = 15
		}
		if req.TTLMinutes > 24*60 {
			http.Error(w, "ttlMinutes must be <= 1440", http.StatusBadRequest)
			return
		}
		req.Reason = strings.TrimSpace(req.Reason)
		user, found, err := st.GetUser(userID)
		if err != nil {
			logger.Printf("get user for password reset token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.password_reset_token.issue", "user", userID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		tokenValue, tokenHash, err := auth.GenerateVoucherToken()
		if err != nil {
			http.Error(w, "token generation error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		resetToken := store.PasswordResetToken{
			TokenID:      uuid.NewString(),
			UserID:       user.UserID,
			TokenHash:    tokenHash,
			DeliveryMode: "operator",
			Reason:       req.Reason,
			ExpiresAt:    now.Add(time.Duration(req.TTLMinutes) * time.Minute),
			CreatedAt:    now,
		}
		if currentUser, ok := auth.UserFromContext(r.Context()); ok {
			resetToken.IssuedByUserID = currentUser.UserID
		}
		if err := st.CreatePasswordResetToken(resetToken); err != nil {
			logger.Printf("create password reset token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.password_reset_token.issue", "user", userID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if req.SendEmail && m != nil {
			if err := m.SendPasswordReset(user.Email, tokenValue, resetToken.ExpiresAt, appPublicURL); err != nil {
				logger.Printf("send password reset email: %v", err)
				// non-fatal — token is still returned in response as admin backup
			} else {
				resetToken.DeliveryMode = "email"
				_ = st.UpdatePasswordResetTokenEmailSent(resetToken.TokenID, time.Now().UTC())
			}
		}
		event := buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.password_reset_token.issue", "user", userID)
		event.AfterJSON = auditJSON(map[string]any{
			"expiresAt": resetToken.ExpiresAt,
			"delivery":  resetToken.DeliveryMode,
			"reason":    req.Reason,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CreatePasswordResetTokenResponse{
			TokenID:   resetToken.TokenID,
			UserID:    user.UserID,
			Email:     user.Email,
			ExpiresAt: resetToken.ExpiresAt,
			Token:     tokenValue,
		})
	}
}

func ForgotPassword(logger *log.Logger, manager *auth.Manager, st store.Store, m mailer.Mailer, appPublicURL string, trustProxy bool) http.HandlerFunc {
	const genericMsg = "If that address is registered, a reset link has been sent."
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req ForgotPasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" {
			http.Error(w, "email required", http.StatusBadRequest)
			return
		}

		// Always return the same response — never reveal whether the email exists.
		respond := func() {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": genericMsg})
		}

		user, found, err := st.GetUserByEmail(req.Email)
		if err != nil {
			logger.Printf("forgot password lookup error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy,
				AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "forgot_password"},
				"auth.forgot_password.requested", "user", ""),
				err)
			respond()
			return
		}

		emailSent := false
		if found && !user.Disabled && user.AuthProvider == "local" {
			tokenValue, tokenHash, err := auth.GenerateVoucherToken()
			if err == nil {
				now := time.Now().UTC()
				resetToken := store.PasswordResetToken{
					TokenID:      uuid.NewString(),
					UserID:       user.UserID,
					TokenHash:    tokenHash,
					DeliveryMode: "email",
					ExpiresAt:    now.Add(30 * time.Minute),
					CreatedAt:    now,
				}
				if storeErr := st.CreatePasswordResetToken(resetToken); storeErr != nil {
					logger.Printf("forgot password create token error: %v", storeErr)
				} else if sendErr := m.SendPasswordReset(user.Email, tokenValue, resetToken.ExpiresAt, appPublicURL); sendErr != nil {
					logger.Printf("forgot password send email error: %v", sendErr)
				} else {
					emailSent = true
					_ = st.UpdatePasswordResetTokenEmailSent(resetToken.TokenID, time.Now().UTC())
				}
			}
		}

		// Audit includes whether email was found + sent, but NOT the user ID (prevents log enumeration).
		event := buildAuditEvent(r, trustProxy,
			AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "forgot_password"},
			"auth.forgot_password.requested", "user", "")
		event.AfterJSON = auditJSON(map[string]any{
			"emailFound": found,
			"emailSent":  emailSent,
		})
		writeAudit(logger, st, event, nil)
		respond()
	}
}

func SendUserInvite(logger *log.Logger, manager *auth.Manager, st store.Store, m mailer.Mailer, appPublicURL string, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		if m == nil {
			http.Error(w, "SMTP not configured", http.StatusServiceUnavailable)
			return
		}
		// NoopMailer — check via interface whether it's the no-op implementation.
		if _, isNoop := m.(*mailer.NoopMailer); isNoop {
			http.Error(w, "SMTP not configured", http.StatusServiceUnavailable)
			return
		}
		userID := strings.TrimSpace(chi.URLParam(r, "userId"))
		if userID == "" {
			http.Error(w, "userId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(userID); err != nil {
			http.Error(w, "userId must be uuid", http.StatusBadRequest)
			return
		}
		user, found, err := st.GetUser(userID)
		if err != nil {
			logger.Printf("get user for invite error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.user_invite.sent", "user", userID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		tokenValue, tokenHash, err := auth.GenerateVoucherToken()
		if err != nil {
			http.Error(w, "token generation error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		resetToken := store.PasswordResetToken{
			TokenID:      uuid.NewString(),
			UserID:       user.UserID,
			TokenHash:    tokenHash,
			DeliveryMode: "invite",
			ExpiresAt:    now.Add(72 * time.Hour),
			CreatedAt:    now,
		}
		if currentUser, ok := auth.UserFromContext(r.Context()); ok {
			resetToken.IssuedByUserID = currentUser.UserID
		}
		if err := st.CreatePasswordResetToken(resetToken); err != nil {
			logger.Printf("create invite token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.user_invite.sent", "user", userID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if sendErr := m.SendInvite(user.Email, tokenValue, resetToken.ExpiresAt, appPublicURL); sendErr != nil {
			logger.Printf("send invite email error: %v", sendErr)
			// Non-fatal: token is stored. Log and continue.
		} else {
			_ = st.UpdatePasswordResetTokenEmailSent(resetToken.TokenID, time.Now().UTC())
		}
		event := buildAuditEvent(r, trustProxy, actorUser(manager.Mode()), "auth.user_invite.sent", "user", userID)
		event.AfterJSON = auditJSON(map[string]any{
			"email":     user.Email,
			"expiresAt": resetToken.ExpiresAt,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SendUserInviteResponse{
			TokenID:   resetToken.TokenID,
			UserID:    user.UserID,
			Email:     user.Email,
			ExpiresAt: resetToken.ExpiresAt,
		})
	}
}

func CompletePasswordResetToken(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() || manager.Mode() != auth.ModeLocal {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req PasswordResetTokenCompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.ResetToken = strings.TrimSpace(req.ResetToken)
		if req.Email == "" || req.ResetToken == "" || strings.TrimSpace(req.NewPassword) == "" {
			http.Error(w, "email, resetToken, and newPassword required", http.StatusBadRequest)
			return
		}
		passwordHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updatedUser, ok, err := st.ConsumePasswordResetToken(req.Email, auth.HashToken(req.ResetToken), passwordHash, time.Now().UTC())
		if err != nil {
			logger.Printf("consume password reset token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(
				r,
				trustProxy,
				AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "password_reset_token"},
				"auth.password_reset_token.complete",
				"user",
				req.Email,
			), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			writeAudit(logger, st, buildAuditEvent(
				r,
				trustProxy,
				AuditActor{Type: "anonymous", Email: req.Email, AuthMethod: "password_reset_token"},
				"auth.password_reset_token.complete",
				"user",
				req.Email,
			), errors.New("invalid password reset token"))
			http.Error(w, "invalid reset token", http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, AuditActor{
			Type:       "password_reset_token",
			ID:         updatedUser.UserID,
			Email:      updatedUser.Email,
			Roles:      rolesFromJSON(updatedUser.RolesJSON),
			AuthMethod: "password_reset_token",
		}, "auth.password_reset_token.complete", "user", updatedUser.UserID)
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PasswordResetTokenCompleteResponse{Success: true})
	}
}

func countRecoveryCodes(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	var hashes []string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return 0
	}
	return len(hashes)
}
