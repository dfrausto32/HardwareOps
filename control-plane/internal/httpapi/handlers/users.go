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
	"github.com/parcel/control-plane/internal/store"
)

type CreateUserRequest struct {
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	DisplayName string   `json:"displayName"`
	Roles       []string `json:"roles"`
}

type UpdateUserRequest struct {
	DisplayName *string  `json:"displayName"`
	Password    *string  `json:"password"`
	Roles       []string `json:"roles"`
	Disabled    *bool    `json:"disabled"`
}

type UserListResponse struct {
	Items  []UserView `json:"items"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

func CreateUser(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		if req.Email == "" || req.Password == "" {
			http.Error(w, "email and password required", http.StatusBadRequest)
			return
		}
		roles, err := normalizeRoles(req.Roles)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rolesJSON, _ := json.Marshal(roles)
		now := time.Now().UTC()
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
			logger.Printf("create user error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "user.create", "user", user.UserID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "user.create", "user", user.UserID)
		event.AfterJSON = auditJSON(map[string]any{
			"email": user.Email,
			"roles": roles,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userView(user))
	}
}

func ListUsers(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseIntParam(r.URL.Query().Get("limit"), 100)
		offset := parseIntParam(r.URL.Query().Get("offset"), 0)
		users, err := st.ListUsers(limit, offset)
		if err != nil {
			logger.Printf("list users error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]UserView, 0, len(users))
		for _, user := range users {
			items = append(items, userView(user))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UserListResponse{Items: items, Limit: limit, Offset: offset})

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "user.list", "user", "")
		writeAudit(logger, st, event, nil)
	}
}

func UpdateUser(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		if userID == "" {
			http.Error(w, "userId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(userID); err != nil {
			http.Error(w, "userId must be uuid", http.StatusBadRequest)
			return
		}
		var req UpdateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		update := store.UserUpdate{UserID: userID}
		if req.DisplayName != nil {
			name := strings.TrimSpace(*req.DisplayName)
			update.DisplayName = &name
		}
		if req.Password != nil {
			if strings.TrimSpace(*req.Password) == "" {
				http.Error(w, "password cannot be empty", http.StatusBadRequest)
				return
			}
			hash, err := auth.HashPassword(*req.Password)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			update.PasswordHash = &hash
		}
		if req.Disabled != nil {
			update.Disabled = req.Disabled
		}
		if req.Roles != nil {
			roles, err := normalizeRoles(req.Roles)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			rolesJSON, _ := json.Marshal(roles)
			update.RolesJSON = rolesJSON
		}
		if update.DisplayName == nil && update.PasswordHash == nil && update.Disabled == nil && update.RolesJSON == nil {
			http.Error(w, "no fields to update", http.StatusBadRequest)
			return
		}
		if err := st.UpdateUser(update); err != nil {
			logger.Printf("update user error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "user.update", "user", userID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "user.update", "user", userID)
		event.AfterJSON = auditJSON(map[string]any{
			"roles":    req.Roles,
			"disabled": req.Disabled,
		})
		writeAudit(logger, st, event, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func normalizeRoles(roles []string) ([]string, error) {
	valid := map[string]struct{}{
		"admin":    {},
		"operator": {},
		"viewer":   {},
	}
	if len(roles) == 0 {
		return []string{"viewer"}, nil
	}
	out := make([]string, 0, len(roles))
	seen := map[string]struct{}{}
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		if _, ok := valid[role]; !ok {
			return nil, errors.New("invalid role: " + role)
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if len(out) == 0 {
		return []string{"viewer"}, nil
	}
	return out, nil
}
