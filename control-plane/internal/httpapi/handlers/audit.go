package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hardwareops/control-plane/internal/auth"
	"github.com/hardwareops/control-plane/internal/store"
)

type AuditActor struct {
	Type       string
	ID         string
	Email      string
	Roles      []string
	AuthMethod string
}

func actorDevice(deviceID string) AuditActor {
	return AuditActor{
		Type:       "device",
		ID:         deviceID,
		AuthMethod: "mtls",
	}
}

func actorUser(authMethod string) AuditActor {
	if authMethod == "" {
		authMethod = "unknown"
	}
	return AuditActor{
		Type:       "user",
		AuthMethod: authMethod,
	}
}

func buildAuditEvent(r *http.Request, trustProxy bool, actor AuditActor, action, targetType, targetID string) store.AuditEvent {
	if actor.Type == "user" {
		if u, ok := auth.UserFromContext(r.Context()); ok {
			actor.ID = u.UserID
			actor.Email = u.Email
			actor.Roles = u.Roles
			if u.AuthMethod != "" {
				actor.AuthMethod = u.AuthMethod
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

func writeAudit(logger *log.Logger, st store.Store, event store.AuditEvent, err error) {
	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
	}
	if st == nil {
		return
	}
	if event.Action == "" {
		return
	}
	if err := st.CreateAuditEvent(event); err != nil && logger != nil {
		logger.Printf("audit write error: %v", err)
	}
}

func auditJSON(v any) []byte {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if string(data) == "null" {
		return nil
	}
	return data
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-Request-ID")); v != "" {
		return v
	}
	return ""
}
