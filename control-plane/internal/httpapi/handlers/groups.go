package handlers

import (
	"errors"
	"encoding/json"
	"log"
	"net/http"
	"strings"
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

type GroupBatchAction struct {
	Action   string          `json:"action"`
	GroupID  string          `json:"groupId"`
	Name     string          `json:"name,omitempty"`
	Selector json.RawMessage `json:"selector,omitempty"`
}

type GroupBatchRequest struct {
	Actions []GroupBatchAction `json:"actions"`
}

type GroupBatchActionResult struct {
	Index   int    `json:"index"`
	Action  string `json:"action"`
	GroupID string `json:"groupId,omitempty"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type GroupBatchResponse struct {
	Applied int                      `json:"applied"`
	Failed  int                      `json:"failed"`
	Results []GroupBatchActionResult `json:"results"`
}

const maxGroupBatchActions = 1000

func normalizeGroupSelectorJSON(selector json.RawMessage) (json.RawMessage, error) {
	if len(selector) == 0 {
		return []byte("{}"), nil
	}
	if !isJSONObject(selector) {
		return nil, errors.New("selector must be object")
	}
	return selector, nil
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
		selector, err := normalizeGroupSelectorJSON(req.Selector)
		if err != nil {
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

func BatchGroups(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GroupBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if len(req.Actions) == 0 {
			http.Error(w, "actions required", http.StatusBadRequest)
			return
		}
		if len(req.Actions) > maxGroupBatchActions {
			http.Error(w, "too many actions", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		resp := GroupBatchResponse{
			Results: make([]GroupBatchActionResult, 0, len(req.Actions)),
		}

		for i, actionReq := range req.Actions {
			action := strings.ToLower(strings.TrimSpace(actionReq.Action))
			groupID := strings.TrimSpace(actionReq.GroupID)
			result := GroupBatchActionResult{
				Index:   i,
				Action:  action,
				GroupID: groupID,
				Status:  "failed",
			}

			switch action {
			case "upsert":
				if groupID == "" {
					groupID = uuid.NewString()
					result.GroupID = groupID
				}
				if _, err := uuid.Parse(groupID); err != nil {
					result.Error = "groupId must be uuid"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					continue
				}
				selector, err := normalizeGroupSelectorJSON(actionReq.Selector)
				if err != nil {
					result.Error = "selector must be a JSON object"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					continue
				}
				group := store.Group{
					GroupID:      groupID,
					Name:         actionReq.Name,
					SelectorJSON: selector,
					CreatedAt:    now,
				}
				if err := st.UpsertGroup(group); err != nil {
					result.Error = "storage error"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "group.batch.upsert", "group", groupID), err)
					continue
				}
				result.Status = "applied"
				resp.Applied++
				resp.Results = append(resp.Results, result)
				event := buildAuditEvent(r, trustProxy, actorUser("ui"), "group.batch.upsert", "group", groupID)
				event.MetadataJSON = auditJSON(map[string]any{"index": i})
				event.AfterJSON = auditJSON(map[string]any{
					"groupId":  groupID,
					"name":     group.Name,
					"selector": json.RawMessage(group.SelectorJSON),
				})
				writeAudit(logger, st, event, nil)

			case "delete":
				if groupID == "" {
					result.Error = "groupId required"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					continue
				}
				if _, err := uuid.Parse(groupID); err != nil {
					result.Error = "groupId must be uuid"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					continue
				}
				if err := st.DeleteGroup(groupID); err != nil {
					result.Error = "storage error"
					resp.Failed++
					resp.Results = append(resp.Results, result)
					writeAudit(logger, st, buildAuditEvent(r, trustProxy, actorUser("ui"), "group.batch.delete", "group", groupID), err)
					continue
				}
				result.Status = "applied"
				resp.Applied++
				resp.Results = append(resp.Results, result)
				event := buildAuditEvent(r, trustProxy, actorUser("ui"), "group.batch.delete", "group", groupID)
				event.MetadataJSON = auditJSON(map[string]any{"index": i})
				writeAudit(logger, st, event, nil)

			default:
				result.Error = "action must be upsert or delete"
				resp.Failed++
				resp.Results = append(resp.Results, result)
			}
		}

		summary := buildAuditEvent(r, trustProxy, actorUser("ui"), "group.batch", "group", "")
		summary.MetadataJSON = auditJSON(map[string]any{
			"applied": resp.Applied,
			"failed":  resp.Failed,
			"total":   len(req.Actions),
		})
		writeAudit(logger, st, summary, nil)

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
