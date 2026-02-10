package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
)

type CreateVoucherRequest struct {
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	TTLHours int      `json:"ttlHours"`
}

type CreateVoucherResponse struct {
	VoucherID string    `json:"voucherId"`
	Token     string    `json:"token"`
	Email     string    `json:"email,omitempty"`
	Roles     []string  `json:"roles,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func CreateAuthVoucher(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateVoucherRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.TTLHours <= 0 {
			req.TTLHours = 24
		}
		if req.TTLHours > 720 {
			http.Error(w, "ttlHours must be <= 720", http.StatusBadRequest)
			return
		}
		roles, err := normalizeRoles(req.Roles)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rolesJSON, _ := json.Marshal(roles)
		token, tokenHash, err := auth.GenerateVoucherToken()
		if err != nil {
			logger.Printf("voucher token error: %v", err)
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		voucher := store.AuthVoucher{
			VoucherID: uuid.NewString(),
			TokenHash: tokenHash,
			Email:     req.Email,
			RolesJSON: rolesJSON,
			ExpiresAt: now.Add(time.Duration(req.TTLHours) * time.Hour),
			CreatedAt: now,
		}
		if u, ok := auth.UserFromContext(r.Context()); ok {
			voucher.CreatedBy = u.UserID
		}
		if err := st.CreateAuthVoucher(voucher); err != nil {
			logger.Printf("create voucher error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.voucher.create", "voucher", voucher.VoucherID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.voucher.create", "voucher", voucher.VoucherID)
		event.AfterJSON = auditJSON(map[string]any{
			"email":     voucher.Email,
			"roles":     roles,
			"expiresAt": voucher.ExpiresAt,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CreateVoucherResponse{
			VoucherID: voucher.VoucherID,
			Token:     token,
			Email:     voucher.Email,
			Roles:     roles,
			ExpiresAt: voucher.ExpiresAt,
		})
	}
}
