package globalplane

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) UpsertPolicySyncStatus(groupID, planeID string, pushedAt *time.Time, pushErr string, retryCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO global_policy_sync_status
			(group_id, plane_id, pushed_at, push_error, retry_count, last_attempt_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $6)
		ON CONFLICT (group_id, plane_id) DO UPDATE SET
			pushed_at       = CASE WHEN EXCLUDED.push_error = '' THEN EXCLUDED.pushed_at
			                       ELSE global_policy_sync_status.pushed_at END,
			push_error      = EXCLUDED.push_error,
			retry_count     = EXCLUDED.retry_count,
			last_attempt_at = EXCLUDED.last_attempt_at,
			updated_at      = EXCLUDED.updated_at
	`, groupID, planeID, pushedAt, pushErr, retryCount, now)
	return err
}

func (s *PostgresStore) GetPolicySyncStatus(groupID, planeID string) (PolicySyncStatus, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st PolicySyncStatus
	err := s.pool.QueryRow(ctx, `
		SELECT sync_id, group_id, plane_id, pushed_at, COALESCE(push_error,''),
		       retry_count, last_attempt_at, created_at, updated_at
		FROM global_policy_sync_status
		WHERE group_id = $1 AND plane_id = $2
	`, groupID, planeID).Scan(
		&st.SyncID, &st.GroupID, &st.PlaneID, &st.PushedAt, &st.PushError,
		&st.RetryCount, &st.LastAttemptAt, &st.CreatedAt, &st.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PolicySyncStatus{}, false, nil
	}
	return st, err == nil, err
}

func (s *PostgresStore) ListAllPolicySyncStatus() ([]PolicySyncStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT sync_id, group_id, plane_id, pushed_at, COALESCE(push_error,''),
		       retry_count, last_attempt_at, created_at, updated_at
		FROM global_policy_sync_status
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicySyncStatus
	for rows.Next() {
		var st PolicySyncStatus
		if err := rows.Scan(
			&st.SyncID, &st.GroupID, &st.PlaneID, &st.PushedAt, &st.PushError,
			&st.RetryCount, &st.LastAttemptAt, &st.CreatedAt, &st.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
