package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/store"
)

type GroupRequest struct {
	Name     string          `json:"name"`
	Selector json.RawMessage `json:"selector"`
}

type GroupResponse struct {
	GroupID   string          `json:"groupId"`
	Name      string          `json:"name,omitempty"`
	Selector  json.RawMessage `json:"selector"`
	CreatedAt time.Time       `json:"createdAt"`
}

type GroupListResponse struct {
	Items []GroupResponse `json:"items"`
}

func PutGroup(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		if groupID == "" {
			http.Error(w, "groupId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(groupID); err != nil {
			http.Error(w, "groupId must be uuid", http.StatusBadRequest)
			return
		}

		var req GroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		selector := req.Selector
		if len(selector) == 0 {
			selector = []byte("{}")
		} else if !isJSONObject(selector) {
			http.Error(w, "selector must be a JSON object", http.StatusBadRequest)
			return
		}

		group := store.Group{
			GroupID:      groupID,
			Name:         req.Name,
			SelectorJSON: selector,
			CreatedAt:    time.Now().UTC(),
		}
		if err := st.UpsertGroup(group); err != nil {
			logger.Printf("upsert group error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "group.upsert", "group", groupID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "group.upsert", "group", groupID)
		event.AfterJSON = auditJSON(map[string]any{
			"groupId":  group.GroupID,
			"name":     group.Name,
			"selector": json.RawMessage(group.SelectorJSON),
		})
		writeAudit(logger, st, event, nil)

		resp := GroupResponse{
			GroupID:   group.GroupID,
			Name:      group.Name,
			Selector:  group.SelectorJSON,
			CreatedAt: group.CreatedAt,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func ListGroups(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groups, err := st.ListGroups()
		if err != nil {
			logger.Printf("list groups error: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		resp := GroupListResponse{Items: make([]GroupResponse, 0, len(groups))}
		for _, g := range groups {
			resp.Items = append(resp.Items, GroupResponse{
				GroupID:   g.GroupID,
				Name:      g.Name,
				Selector:  g.SelectorJSON,
				CreatedAt: g.CreatedAt,
			})
		}
		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "group.list", "group", "")
		event.MetadataJSON = auditJSON(map[string]any{"count": len(resp.Items)})
		writeAudit(logger, st, event, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func DeleteGroup(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupId")
		if groupID == "" {
			http.Error(w, "groupId required", http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(groupID); err != nil {
			http.Error(w, "groupId must be uuid", http.StatusBadRequest)
			return
		}
		if err := st.DeleteGroup(groupID); err != nil {
			logger.Printf("delete group error: %v", err)
			writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "group.delete", "group", groupID), err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "group.delete", "group", groupID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

// isJSONObject is defined in util.go
