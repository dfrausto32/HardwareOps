package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
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
