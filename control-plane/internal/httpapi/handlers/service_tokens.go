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
	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
)

type CreateServiceTokenRequest struct {
	Name     string   `json:"name"`
	Scopes   []string `json:"scopes"`
	TTLHours int      `json:"ttlHours"`
}

type CreateServiceTokenResponse struct {
	TokenID   string    `json:"tokenId"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"token"`
}

type RotateServiceTokenRequest struct {
	Reason   string `json:"reason"`
	TTLHours int    `json:"ttlHours,omitempty"`
}

type RotateServiceTokenResponse struct {
	ReplacedTokenID string    `json:"replacedTokenId"`
	TokenID         string    `json:"tokenId"`
	Name            string    `json:"name"`
	Scopes          []string  `json:"scopes"`
	ExpiresAt       time.Time `json:"expiresAt"`
	Token           string    `json:"token"`
	RevokedAt       time.Time `json:"revokedAt"`
}

type ServiceTokenView struct {
	TokenID    string    `json:"tokenId"`
	Name       string    `json:"name"`
	Scopes     []string  `json:"scopes"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	CreatedBy  string    `json:"createdBy,omitempty"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  time.Time `json:"revokedAt,omitempty"`
	RevokedBy  string    `json:"revokedBy,omitempty"`
}

type ServiceTokenListResponse struct {
	Items  []ServiceTokenView `json:"items"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

func CreateServiceToken(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateServiceTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		scopes, err := normalizeServiceTokenScopes(req.Scopes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.TTLHours <= 0 {
			req.TTLHours = 24
		}
		if req.TTLHours > 24*30 {
			http.Error(w, "ttlHours must be <= 720", http.StatusBadRequest)
			return
		}
		token, tokenHash, err := auth.GenerateVoucherToken()
		if err != nil {
			logger.Printf("service token generation error: %v", err)
			http.Error(w, "token generation error", http.StatusInternalServerError)
			return
		}
		scopesJSON, _ := json.Marshal(scopes)
		now := time.Now().UTC()
		serviceToken := store.ServiceToken{
			TokenID:    uuid.NewString(),
			Name:       req.Name,
			TokenHash:  tokenHash,
			ScopesJSON: scopesJSON,
			ExpiresAt:  now.Add(time.Duration(req.TTLHours) * time.Hour),
			CreatedAt:  now,
		}
		if u, ok := auth.UserFromContext(r.Context()); ok {
			serviceToken.CreatedBy = u.UserID
		}
		if err := st.CreateServiceToken(serviceToken); err != nil {
			logger.Printf("create service token error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.service_token.create", "service_token", serviceToken.TokenID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.service_token.create", "service_token", serviceToken.TokenID)
		event.AfterJSON = auditJSON(map[string]any{
			"name":      serviceToken.Name,
			"scopes":    scopes,
			"expiresAt": serviceToken.ExpiresAt,
		})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CreateServiceTokenResponse{
			TokenID:   serviceToken.TokenID,
			Name:      serviceToken.Name,
			Scopes:    scopes,
			ExpiresAt: serviceToken.ExpiresAt,
			Token:     token,
		})
	}
}

func ListServiceTokens(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseInt(r.URL.Query().Get("limit"), 100)
		offset := parseInt(r.URL.Query().Get("offset"), 0)
		tokens, err := st.ListServiceTokens(limit, offset)
		if err != nil {
			logger.Printf("list service tokens error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]ServiceTokenView, 0, len(tokens))
		for _, token := range tokens {
			items = append(items, serviceTokenView(token))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ServiceTokenListResponse{
			Items:  items,
			Limit:  limit,
			Offset: offset,
		})
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.service_token.list", "service_token", "")
		writeAudit(logger, st, event, nil)
	}
}

func RevokeServiceToken(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenID := strings.TrimSpace(chi.URLParam(r, "tokenId"))
		if tokenID == "" {
			http.Error(w, "tokenId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(tokenID); err != nil {
			http.Error(w, "tokenId must be uuid", http.StatusBadRequest)
			return
		}
		var req BreakglassRequest
		if err := decodeBreakglassRequest(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.service_token.revoke", "service_token", tokenID)
		token, found, err := st.GetServiceToken(tokenID)
		if err != nil {
			logger.Printf("get service token error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "storage_error", req.Reason, nil)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_not_found", req.Reason, nil)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		event.BeforeJSON = auditJSON(serviceTokenAuditSnapshot(token))
		if !token.RevokedAt.IsZero() {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_already_revoked", req.Reason, map[string]any{
				"revokedAt": token.RevokedAt,
				"revokedBy": token.RevokedBy,
			})
			http.Error(w, "service token already revoked", http.StatusConflict)
			return
		}
		now := time.Now().UTC()
		revokedBy := ""
		if u, ok := auth.UserFromContext(r.Context()); ok {
			revokedBy = u.UserID
		}
		ok, err := st.RevokeServiceToken(tokenID, revokedBy, now)
		if err != nil {
			logger.Printf("revoke service token error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "storage_error", req.Reason, nil)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_already_revoked", req.Reason, nil)
			http.Error(w, "service token already revoked", http.StatusConflict)
			return
		}
		event.AfterJSON = auditJSON(map[string]any{
			"revokedAt": now,
			"revokedBy": revokedBy,
		})
		event.MetadataJSON = breakglassMetadata(req.Reason, nil)
		writeAudit(logger, st, event, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func RotateServiceToken(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenID := strings.TrimSpace(chi.URLParam(r, "tokenId"))
		if tokenID == "" {
			http.Error(w, "tokenId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(tokenID); err != nil {
			http.Error(w, "tokenId must be uuid", http.StatusBadRequest)
			return
		}

		var req RotateServiceTokenRequest
		if r.Body == nil {
			http.Error(w, "reason required", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		req.Reason = strings.TrimSpace(req.Reason)
		if req.Reason == "" {
			http.Error(w, "reason required", http.StatusBadRequest)
			return
		}
		if req.TTLHours < 0 {
			http.Error(w, "ttlHours must be >= 0", http.StatusBadRequest)
			return
		}
		if req.TTLHours > 24*30 {
			http.Error(w, "ttlHours must be <= 720", http.StatusBadRequest)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "auth.service_token.rotate", "service_token", tokenID)
		existing, found, err := st.GetServiceToken(tokenID)
		if err != nil {
			logger.Printf("get service token error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "storage_error", req.Reason, nil)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !found {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_not_found", req.Reason, nil)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		event.BeforeJSON = auditJSON(serviceTokenAuditSnapshot(existing))
		if !existing.RevokedAt.IsZero() {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_already_revoked", req.Reason, map[string]any{
				"revokedAt": existing.RevokedAt,
				"revokedBy": existing.RevokedBy,
			})
			http.Error(w, "service token already revoked", http.StatusConflict)
			return
		}

		now := time.Now().UTC()
		if !existing.ExpiresAt.IsZero() && !existing.ExpiresAt.After(now) {
			auditBreakglassFailure(logger, st, event, "denied", "service_token_expired", req.Reason, map[string]any{
				"expiresAt": existing.ExpiresAt,
			})
			http.Error(w, "service token expired", http.StatusConflict)
			return
		}

		expiresAt := existing.ExpiresAt
		if req.TTLHours > 0 {
			expiresAt = now.Add(time.Duration(req.TTLHours) * time.Hour)
		}

		rawToken, tokenHash, err := auth.GenerateVoucherToken()
		if err != nil {
			logger.Printf("rotate service token generation error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "token_generation_error", req.Reason, nil)
			http.Error(w, "token generation error", http.StatusInternalServerError)
			return
		}

		actorID := ""
		if u, ok := auth.UserFromContext(r.Context()); ok {
			actorID = u.UserID
		}
		replacement := store.ServiceToken{
			TokenID:    uuid.NewString(),
			Name:       existing.Name,
			TokenHash:  tokenHash,
			ScopesJSON: existing.ScopesJSON,
			ExpiresAt:  expiresAt,
			CreatedAt:  now,
			CreatedBy:  actorID,
		}
		if err := st.CreateServiceToken(replacement); err != nil {
			logger.Printf("create replacement service token error: %v", err)
			auditBreakglassFailure(logger, st, event, "error", "storage_error", req.Reason, nil)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		ok, err := st.RevokeServiceToken(tokenID, actorID, now)
		if err != nil {
			logger.Printf("revoke original service token error: %v", err)
			if _, revokeErr := st.RevokeServiceToken(replacement.TokenID, actorID, now); revokeErr != nil {
				logger.Printf("revoke replacement service token compensation error: %v", revokeErr)
			}
			auditBreakglassFailure(logger, st, event, "error", "storage_error", req.Reason, map[string]any{
				"replacementTokenId": replacement.TokenID,
			})
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		if !ok {
			if _, revokeErr := st.RevokeServiceToken(replacement.TokenID, actorID, now); revokeErr != nil {
				logger.Printf("revoke replacement service token compensation error: %v", revokeErr)
			}
			auditBreakglassFailure(logger, st, event, "denied", "service_token_already_revoked", req.Reason, map[string]any{
				"replacementTokenId": replacement.TokenID,
			})
			http.Error(w, "service token already revoked", http.StatusConflict)
			return
		}

		event.AfterJSON = auditJSON(serviceTokenAuditSnapshot(replacement))
		event.MetadataJSON = breakglassMetadata(req.Reason, map[string]any{
			"replacementTokenId": replacement.TokenID,
			"preserveExpiresAt":  req.TTLHours == 0,
			"ttlHours":           req.TTLHours,
			"revokedAt":          now,
		})
		writeAudit(logger, st, event, nil)

		writeJSON(w, RotateServiceTokenResponse{
			ReplacedTokenID: tokenID,
			TokenID:         replacement.TokenID,
			Name:            replacement.Name,
			Scopes:          authScopesFromJSON(replacement.ScopesJSON),
			ExpiresAt:       replacement.ExpiresAt,
			Token:           rawToken,
			RevokedAt:       now,
		})
	}
}

func normalizeServiceTokenScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{"artifact.publish"}, nil
	}
	valid := map[string]struct{}{
		"artifact.publish": {},
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if _, ok := valid[scope]; !ok {
			return nil, errors.New("invalid scope: " + scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return []string{"artifact.publish"}, nil
	}
	return out, nil
}

func serviceTokenView(token store.ServiceToken) ServiceTokenView {
	return ServiceTokenView{
		TokenID:    token.TokenID,
		Name:       token.Name,
		Scopes:     authScopesFromJSON(token.ScopesJSON),
		ExpiresAt:  token.ExpiresAt,
		CreatedAt:  token.CreatedAt,
		CreatedBy:  token.CreatedBy,
		LastUsedAt: token.LastUsedAt,
		RevokedAt:  token.RevokedAt,
		RevokedBy:  token.RevokedBy,
	}
}

func authScopesFromJSON(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal(data, &scopes); err != nil {
		return nil
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func serviceTokenAuditSnapshot(token store.ServiceToken) map[string]any {
	snapshot := map[string]any{
		"tokenId":   token.TokenID,
		"name":      token.Name,
		"scopes":    authScopesFromJSON(token.ScopesJSON),
		"expiresAt": token.ExpiresAt,
		"createdAt": token.CreatedAt,
		"createdBy": token.CreatedBy,
	}
	if !token.LastUsedAt.IsZero() {
		snapshot["lastUsedAt"] = token.LastUsedAt
	}
	if !token.RevokedAt.IsZero() {
		snapshot["revokedAt"] = token.RevokedAt
		snapshot["revokedBy"] = token.RevokedBy
	}
	return snapshot
}
