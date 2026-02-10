package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
)

type AuditEventResponse struct {
	EventID    string          `json:"eventId"`
	OccurredAt time.Time       `json:"occurredAt"`
	ActorType  string          `json:"actorType,omitempty"`
	ActorID    string          `json:"actorId,omitempty"`
	ActorEmail string          `json:"actorEmail,omitempty"`
	ActorRoles json.RawMessage `json:"actorRoles,omitempty"`
	AuthMethod string          `json:"authMethod,omitempty"`
	SourceIP   string          `json:"sourceIp,omitempty"`
	UserAgent  string          `json:"userAgent,omitempty"`
	RequestID  string          `json:"requestId,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType,omitempty"`
	TargetID   string          `json:"targetId,omitempty"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type AuditListResponse struct {
	Items  []AuditEventResponse `json:"items"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

type AuditRetentionResponse struct {
	Days      int       `json:"days"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const (
	defaultAuditLimit = 200
	maxAuditLimit     = 5000
)

func ListAuditEvents(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseAuditFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		events, err := st.ListAuditEvents(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("list audit events error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]AuditEventResponse, 0, len(events))
		for _, ev := range events {
			items = append(items, auditEventResponse(ev))
		}
		resp := AuditListResponse{
			Items:  items,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "audit.list", "audit", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"count":  len(items),
			"filter": filter,
		})
		writeAudit(logger, st, event, nil)
	}
}

func ExportAuditCSV(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseAuditFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		events, err := st.ListAuditEvents(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("list audit events error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"audit.csv\"")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{
			"event_id",
			"occurred_at",
			"actor_type",
			"actor_id",
			"actor_email",
			"actor_roles",
			"auth_method",
			"source_ip",
			"user_agent",
			"request_id",
			"action",
			"target_type",
			"target_id",
			"status",
			"error",
			"before",
			"after",
			"metadata",
		})
		for _, ev := range events {
			_ = writer.Write([]string{
				ev.EventID,
				ev.OccurredAt.Format(time.RFC3339Nano),
				ev.ActorType,
				ev.ActorID,
				ev.ActorEmail,
				compactJSON(ev.ActorRolesJSON),
				ev.AuthMethod,
				ev.SourceIP,
				ev.UserAgent,
				ev.RequestID,
				ev.Action,
				ev.TargetType,
				ev.TargetID,
				ev.Status,
				ev.Error,
				compactJSON(ev.BeforeJSON),
				compactJSON(ev.AfterJSON),
				compactJSON(ev.MetadataJSON),
			})
		}
		writer.Flush()

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "audit.export", "audit", "")
		event.MetadataJSON = auditJSON(map[string]any{
			"count":  len(events),
			"filter": filter,
		})
		writeAudit(logger, st, event, nil)
	}
}

func GetAuditRetention(logger *log.Logger, st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retention, err := st.GetAuditRetentionDays()
		if err != nil {
			if logger != nil {
				logger.Printf("get audit retention error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AuditRetentionResponse{
			Days:      retention.Days,
			UpdatedAt: retention.UpdatedAt,
		})
	}
}

func SetAuditRetention(logger *log.Logger, st store.Store, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Days int `json:"days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Days <= 0 || req.Days > 3650 {
			http.Error(w, "days must be between 1 and 3650", http.StatusBadRequest)
			return
		}
		before, _ := st.GetAuditRetentionDays()
		retention, err := st.SetAuditRetentionDays(req.Days)
		if err != nil {
			if logger != nil {
				logger.Printf("set audit retention error: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		cutoff := time.Now().UTC().Add(-time.Duration(retention.Days) * 24 * time.Hour)
		if _, err := st.DeleteAuditEventsBefore(cutoff); err != nil && logger != nil {
			logger.Printf("prune audit events error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AuditRetentionResponse{
			Days:      retention.Days,
			UpdatedAt: retention.UpdatedAt,
		})

		event := buildAuditEvent(r, trustProxy, actorUser("ui"), "audit.retention.update", "audit", "")
		event.BeforeJSON = auditJSON(map[string]any{"days": before.Days, "updatedAt": before.UpdatedAt})
		event.AfterJSON = auditJSON(map[string]any{"days": retention.Days, "updatedAt": retention.UpdatedAt})
		writeAudit(logger, st, event, nil)
	}
}

func parseAuditFilter(r *http.Request) (store.AuditEventFilter, error) {
	q := r.URL.Query()
	since, err := parseTimeParam(q.Get("since"))
	if err != nil {
		return store.AuditEventFilter{}, err
	}
	until, err := parseTimeParam(q.Get("until"))
	if err != nil {
		return store.AuditEventFilter{}, err
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return store.AuditEventFilter{}, fmt.Errorf("until must be after since")
	}
	limit := parseIntParam(q.Get("limit"), defaultAuditLimit)
	if limit > maxAuditLimit {
		limit = maxAuditLimit
	}
	offset := parseIntParam(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	return store.AuditEventFilter{
		Action:     q.Get("action"),
		ActorType:  q.Get("actorType"),
		ActorID:    q.Get("actorId"),
		ActorEmail: q.Get("actorEmail"),
		TargetType: q.Get("targetType"),
		TargetID:   q.Get("targetId"),
		Status:     q.Get("status"),
		Since:      since,
		Until:      until,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func parseTimeParam(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q, must be RFC3339", raw)
}

func parseIntParam(raw string, def int) int {
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func auditEventResponse(ev store.AuditEvent) AuditEventResponse {
	return AuditEventResponse{
		EventID:    ev.EventID,
		OccurredAt: ev.OccurredAt,
		ActorType:  ev.ActorType,
		ActorID:    ev.ActorID,
		ActorEmail: ev.ActorEmail,
		ActorRoles: rawOrNil(ev.ActorRolesJSON),
		AuthMethod: ev.AuthMethod,
		SourceIP:   ev.SourceIP,
		UserAgent:  ev.UserAgent,
		RequestID:  ev.RequestID,
		Action:     ev.Action,
		TargetType: ev.TargetType,
		TargetID:   ev.TargetID,
		Status:     ev.Status,
		Error:      ev.Error,
		Before:     rawOrNil(ev.BeforeJSON),
		After:      rawOrNil(ev.AfterJSON),
		Metadata:   rawOrNil(ev.MetadataJSON),
	}
}

func rawOrNil(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	trim := bytes.TrimSpace(b)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil
	}
	if bytes.Equal(trim, []byte("{}")) || bytes.Equal(trim, []byte("[]")) {
		return nil
	}
	return json.RawMessage(trim)
}

func compactJSON(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	trim := bytes.TrimSpace(b)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) || bytes.Equal(trim, []byte("{}")) || bytes.Equal(trim, []byte("[]")) {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trim); err != nil {
		return string(trim)
	}
	return buf.String()
}
