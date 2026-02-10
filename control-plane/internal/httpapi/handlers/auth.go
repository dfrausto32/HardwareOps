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

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      UserView  `json:"user"`
}

type UserView struct {
	UserID      string          `json:"userId"`
	Email       string          `json:"email"`
	DisplayName string          `json:"displayName,omitempty"`
	Roles       json.RawMessage `json:"roles,omitempty"`
	Disabled    bool            `json:"disabled,omitempty"`
	CreatedAt   time.Time       `json:"createdAt,omitempty"`
	LastLoginAt time.Time       `json:"lastLoginAt,omitempty"`
}

type AuthStatusResponse struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

type RegisterRequest struct {
	Token       string `json:"token"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type RegisterResponse struct {
	User UserView `json:"user"`
}

func Login(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" || req.Password == "" {
			http.Error(w, "email and password required", http.StatusBadRequest)
			return
		}
		user, token, exp, err := manager.Authenticate(req.Email, req.Password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		resp := LoginResponse{
			Token:     token,
			ExpiresAt: exp,
			User:      userView(user),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		event := buildAuditEvent(r, trustProxy, actorUser("local"), "auth.login", "user", user.UserID)
		writeAudit(logger, st, event, nil)
	}
}

func GetMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId": user.UserID,
			"email":  user.Email,
			"roles":  user.Roles,
		})
	}
}

func AuthStatus(manager *auth.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enabled := manager != nil && manager.Enabled()
		mode := "disabled"
		if manager != nil {
			mode = manager.Mode()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AuthStatusResponse{Enabled: enabled, Mode: mode})
	}
}

func Register(logger *log.Logger, manager *auth.Manager, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if manager == nil || !manager.Enabled() {
			http.Error(w, "auth disabled", http.StatusNotFound)
			return
		}
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.Token = strings.TrimSpace(req.Token)
		if req.Token == "" || req.Email == "" || req.Password == "" {
			http.Error(w, "token, email, and password required", http.StatusBadRequest)
			return
		}
		tokenHash := auth.HashToken(req.Token)
		voucher, ok, err := st.GetAuthVoucherByTokenHash(tokenHash)
		if err != nil {
			logger.Printf("voucher lookup error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok || voucher.Revoked || (!voucher.UsedAt.IsZero()) {
			http.Error(w, "invalid or used voucher", http.StatusBadRequest)
			return
		}
		if !voucher.ExpiresAt.IsZero() && time.Now().UTC().After(voucher.ExpiresAt) {
			http.Error(w, "voucher expired", http.StatusBadRequest)
			return
		}
		if voucher.Email != "" && !strings.EqualFold(voucher.Email, req.Email) {
			http.Error(w, "voucher email mismatch", http.StatusBadRequest)
			return
		}

		roles := rolesFromJSON(voucher.RolesJSON)
		if len(roles) == 0 {
			roles = []string{"viewer"}
		}
		rolesJSON, _ := json.Marshal(roles)
		now := time.Now().UTC()
		if ok, err := st.MarkAuthVoucherUsed(voucher.VoucherID, req.Email, now); err != nil || !ok {
			if err != nil {
				logger.Printf("voucher consume error: %v", err)
			}
			http.Error(w, "voucher already used", http.StatusConflict)
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user := store.User{
			UserID:       uuid.NewString(),
			Email:        req.Email,
			DisplayName:  strings.TrimSpace(req.DisplayName),
			PasswordHash: hash,
			RolesJSON:    rolesJSON,
			AuthProvider: "local",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := st.CreateUser(user); err != nil {
			logger.Printf("register user error: %v", err)
			event := buildAuditEvent(r, trustProxy, AuditActor{Type: "voucher", ID: voucher.VoucherID, Email: req.Email, AuthMethod: "voucher"}, "auth.register", "user", user.UserID)
			writeAudit(logger, st, event, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, AuditActor{Type: "voucher", ID: voucher.VoucherID, Email: req.Email, AuthMethod: "voucher"}, "auth.register", "user", user.UserID)
		event.AfterJSON = auditJSON(map[string]any{
			"email": req.Email,
			"roles": roles,
		})
		writeAudit(logger, st, event, nil)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RegisterResponse{User: userView(user)})
	}
}

func userView(user store.User) UserView {
	return UserView{
		UserID:      user.UserID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       json.RawMessage(user.RolesJSON),
		Disabled:    user.Disabled,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

func rolesFromJSON(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var roles []string
	if err := json.Unmarshal(data, &roles); err != nil {
		return nil
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		out = append(out, role)
	}
	return out
}
