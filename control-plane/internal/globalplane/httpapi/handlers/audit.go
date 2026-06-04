package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/parcel/control-plane/internal/auth"
	"github.com/parcel/control-plane/internal/store"
)

// auditStore is the minimal interface needed by handlers that write audit events.
type auditStore interface {
	CreateAuditEvent(event store.AuditEvent) error
}

// auditQueryStore is the interface needed by the audit query handlers.
type auditQueryStore interface {
	ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error)
	DeleteAuditEventsBefore(cutoff time.Time) (int, error)
	CreateAuditEvent(event store.AuditEvent) error
}

// ── Audit helpers ─────────────────────────────────────────────────────────────

type auditActor struct {
	Type       string
	ID         string
	Email      string
	Roles      []string
	AuthMethod string
}

func buildAuditEvent(r *http.Request, trustProxy bool, actor auditActor, action, targetType, targetID string) store.AuditEvent {
	if actor.Type == "user" {
		if u, ok := auth.UserFromContext(r.Context()); ok {
			actor.ID = u.UserID
			actor.Email = u.Email
			actor.Roles = u.Roles
			if u.AuthMethod != "" {
				actor.AuthMethod = u.AuthMethod
			}
		} else if svc, ok := auth.ServiceTokenFromContext(r.Context()); ok {
			actor.Type = "service_token"
			actor.ID = svc.TokenID
			actor.Email = svc.Name
			actor.Roles = svc.Scopes
			if svc.AuthMethod != "" {
				actor.AuthMethod = svc.AuthMethod
			}
		} else if actor.ID == "" {
			actor.ID = "admin"
			if actor.Email == "" {
				actor.Email = "admin@local"
			}
			if len(actor.Roles) == 0 {
				actor.Roles = []string{"admin"}
			}
			if actor.AuthMethod == "" {
				actor.AuthMethod = "disabled"
			}
		}
	}
	if actor.Type == "" {
		actor.Type = "system"
	}
	if actor.AuthMethod == "" {
		actor.AuthMethod = "unknown"
	}
	rolesJSON, _ := json.Marshal(actor.Roles)
	return store.AuditEvent{
		OccurredAt:     time.Now().UTC(),
		ActorType:      actor.Type,
		ActorID:        actor.ID,
		ActorEmail:     actor.Email,
		ActorRolesJSON: rolesJSON,
		AuthMethod:     actor.AuthMethod,
		SourceIP:       clientIP(r, trustProxy),
		UserAgent:      r.UserAgent(),
		RequestID:      requestID(r),
		Action:         action,
		TargetType:     targetType,
		TargetID:       targetID,
		Status:         "success",
	}
}

func writeAudit(logger *log.Logger, st auditStore, event store.AuditEvent, err error) {
	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
	}
	if st == nil || event.Action == "" {
		return
	}
	if werr := st.CreateAuditEvent(event); werr != nil && logger != nil {
		logger.Printf("[global-plane] audit write error: %v", werr)
	}
}

func auditJSON(v any) []byte {
	if v == nil {
		return nil
	}
	data, _ := json.Marshal(v)
	if string(data) == "null" {
		return nil
	}
	return data
}

func clientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

// ── Audit query handlers ──────────────────────────────────────────────────────

const (
	defaultAuditLimit = 200
	maxAuditLimit     = 5000
)

type auditEventResponse struct {
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

// ListGlobalAuditEvents handles GET /api/v1/audit.
func ListGlobalAuditEvents(logger *log.Logger, st auditQueryStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseAuditFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		events, err := st.ListAuditEvents(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("[global-plane] list audit events: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		items := make([]auditEventResponse, 0, len(events))
		for _, ev := range events {
			items = append(items, toAuditEventResponse(ev))
		}
		type listResp struct {
			Items  []auditEventResponse `json:"items"`
			Limit  int                  `json:"limit"`
			Offset int                  `json:"offset"`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(listResp{Items: items, Limit: filter.Limit, Offset: filter.Offset})

		ev := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "ui"}, "audit.list", "audit", "")
		ev.MetadataJSON = auditJSON(map[string]any{"count": len(items)})
		writeAudit(logger, st, ev, nil)
	}
}

// ExportGlobalAuditCSV handles GET /api/v1/audit/export.
func ExportGlobalAuditCSV(logger *log.Logger, st auditQueryStore, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseAuditFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		events, err := st.ListAuditEvents(filter)
		if err != nil {
			if logger != nil {
				logger.Printf("[global-plane] export audit events: %v", err)
			}
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="global-audit.csv"`)
		wr := csv.NewWriter(w)
		_ = wr.Write([]string{
			"event_id", "occurred_at", "actor_type", "actor_id", "actor_email",
			"actor_roles", "auth_method", "source_ip", "user_agent", "request_id",
			"action", "target_type", "target_id", "status", "error",
			"before", "after", "metadata",
		})
		for _, ev := range events {
			_ = wr.Write([]string{
				ev.EventID, ev.OccurredAt.Format(time.RFC3339Nano),
				ev.ActorType, ev.ActorID, ev.ActorEmail,
				compactJSON(ev.ActorRolesJSON), ev.AuthMethod,
				ev.SourceIP, ev.UserAgent, ev.RequestID,
				ev.Action, ev.TargetType, ev.TargetID,
				ev.Status, ev.Error,
				compactJSON(ev.BeforeJSON), compactJSON(ev.AfterJSON), compactJSON(ev.MetadataJSON),
			})
		}
		wr.Flush()

		logEv := buildAuditEvent(r, trustProxy, auditActor{Type: "user", AuthMethod: "ui"}, "audit.export", "audit", "")
		logEv.MetadataJSON = auditJSON(map[string]any{"count": len(events)})
		writeAudit(logger, st, logEv, nil)
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
	return time.Time{}, fmt.Errorf("invalid time %q: must be RFC3339", raw)
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

func toAuditEventResponse(ev store.AuditEvent) auditEventResponse {
	return auditEventResponse{
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
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) ||
		bytes.Equal(trim, []byte("{}")) || bytes.Equal(trim, []byte("[]")) {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trim); err != nil {
		return string(trim)
	}
	return buf.String()
}
