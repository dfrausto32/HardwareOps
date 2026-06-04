package globalplane

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store"
)

func (s *PostgresStore) CreateAuditEvent(event store.AuditEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.Status == "" {
		event.Status = "success"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_events (
			event_id, occurred_at,
			actor_type, actor_id, actor_email, actor_roles, auth_method,
			source_ip, user_agent, request_id,
			action, target_type, target_id, status, error,
			before_json, after_json, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18
		)`,
		event.EventID, event.OccurredAt,
		event.ActorType, event.ActorID, event.ActorEmail,
		nullableJSON(event.ActorRolesJSON),
		event.AuthMethod,
		event.SourceIP, event.UserAgent, event.RequestID,
		event.Action, event.TargetType, event.TargetID,
		event.Status, event.Error,
		nullableJSON(event.BeforeJSON),
		nullableJSON(event.AfterJSON),
		nullableJSON(event.MetadataJSON),
	)
	return err
}

func (s *PostgresStore) ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	where := "WHERE 1=1"
	add := func(cond string, val any) {
		args = append(args, val)
		where += " AND " + cond
	}
	if filter.Action != "" {
		add("action = $"+argN(len(args)+1), filter.Action)
	}
	if filter.ActorType != "" {
		add("actor_type = $"+argN(len(args)+1), filter.ActorType)
	}
	if filter.ActorID != "" {
		add("actor_id = $"+argN(len(args)+1), filter.ActorID)
	}
	if filter.ActorEmail != "" {
		add("actor_email = $"+argN(len(args)+1), filter.ActorEmail)
	}
	if filter.TargetType != "" {
		add("target_type = $"+argN(len(args)+1), filter.TargetType)
	}
	if filter.TargetID != "" {
		add("target_id = $"+argN(len(args)+1), filter.TargetID)
	}
	if filter.Status != "" {
		add("status = $"+argN(len(args)+1), filter.Status)
	}
	if !filter.Since.IsZero() {
		add("occurred_at >= $"+argN(len(args)+1), filter.Since)
	}
	if !filter.Until.IsZero() {
		add("occurred_at <= $"+argN(len(args)+1), filter.Until)
	}
	args = append(args, limit, offset)
	q := `SELECT
		event_id, occurred_at,
		actor_type, actor_id, actor_email, actor_roles, auth_method,
		source_ip, user_agent, request_id,
		action, target_type, target_id, status, error,
		before_json, after_json, metadata_json
	FROM audit_events ` + where +
		` ORDER BY occurred_at DESC LIMIT $` + argN(len(args)-1) +
		` OFFSET $` + argN(len(args))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []store.AuditEvent
	for rows.Next() {
		var ev store.AuditEvent
		var actorRoles, beforeJSON, afterJSON, metaJSON []byte
		if err := rows.Scan(
			&ev.EventID, &ev.OccurredAt,
			&ev.ActorType, &ev.ActorID, &ev.ActorEmail, &actorRoles, &ev.AuthMethod,
			&ev.SourceIP, &ev.UserAgent, &ev.RequestID,
			&ev.Action, &ev.TargetType, &ev.TargetID, &ev.Status, &ev.Error,
			&beforeJSON, &afterJSON, &metaJSON,
		); err != nil {
			return nil, err
		}
		ev.ActorRolesJSON = actorRoles
		ev.BeforeJSON = beforeJSON
		ev.AfterJSON = afterJSON
		ev.MetadataJSON = metaJSON
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *PostgresStore) DeleteAuditEventsBefore(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE occurred_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// nullableJSON returns nil for empty/null JSON byte slices so pgx stores NULL.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// argN converts an int to a PostgreSQL "$N" positional placeholder string.
func argN(n int) string { return strconv.Itoa(n) }
