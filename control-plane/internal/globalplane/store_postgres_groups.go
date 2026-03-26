package globalplane

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Global groups ─────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateGlobalGroup(g GlobalGroup) (GlobalGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sel := g.SelectorJSON
	if len(sel) == 0 {
		sel = []byte("{}")
	}
	now := time.Now().UTC()
	err := s.pool.QueryRow(ctx, `
		INSERT INTO global_groups (group_id, name, selector_json, created_by, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $4)
		RETURNING group_id, name, selector_json, created_by, created_at, updated_at
	`, g.Name, sel, g.CreatedBy, now).Scan(
		&g.GroupID, &g.Name, &g.SelectorJSON, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

func (s *PostgresStore) GetGlobalGroup(groupID string) (GlobalGroup, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var g GlobalGroup
	err := s.pool.QueryRow(ctx, `
		SELECT group_id, name, selector_json, COALESCE(created_by,''), created_at, updated_at
		FROM global_groups WHERE group_id = $1
	`, groupID).Scan(&g.GroupID, &g.Name, &g.SelectorJSON, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return GlobalGroup{}, false, nil
	}
	return g, err == nil, err
}

func (s *PostgresStore) ListGlobalGroups() ([]GlobalGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT group_id, name, selector_json, COALESCE(created_by,''), created_at, updated_at
		FROM global_groups ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlobalGroup
	for rows.Next() {
		var g GlobalGroup
		if err := rows.Scan(&g.GroupID, &g.Name, &g.SelectorJSON, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *PostgresStore) DeleteGlobalGroup(groupID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM global_groups WHERE group_id = $1`, groupID)
	return err
}

// ── Global desired state ──────────────────────────────────────────────────────

func (s *PostgresStore) UpsertGlobalDesiredState(state GlobalDesiredState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	policy := state.PolicyJSON
	if len(policy) == 0 {
		policy = []byte("{}")
	}
	components := state.ComponentsJSON
	if len(components) == 0 {
		components = []byte("{}")
	}
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO global_desired_state
			(group_id, artifact_id, desired_version, desired_config_rev, policy_json,
			 components_json, checkin_interval, updated_at, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (group_id) DO UPDATE SET
			artifact_id       = EXCLUDED.artifact_id,
			desired_version   = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy_json       = EXCLUDED.policy_json,
			components_json   = EXCLUDED.components_json,
			checkin_interval  = EXCLUDED.checkin_interval,
			updated_at        = $8,
			updated_by        = EXCLUDED.updated_by
	`, state.GroupID, state.ArtifactID, state.DesiredVersion, state.DesiredConfigRev,
		policy, components, state.CheckinInterval, now, state.UpdatedBy)
	return err
}

func (s *PostgresStore) GetGlobalDesiredState(groupID string) (GlobalDesiredState, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var d GlobalDesiredState
	err := s.pool.QueryRow(ctx, `
		SELECT group_id, COALESCE(artifact_id,''), COALESCE(desired_version,''),
		       COALESCE(desired_config_rev,''), policy_json, components_json,
		       checkin_interval, updated_at, COALESCE(updated_by,'')
		FROM global_desired_state WHERE group_id = $1
	`, groupID).Scan(
		&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev,
		&d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.UpdatedAt, &d.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GlobalDesiredState{}, false, nil
	}
	return d, err == nil, err
}

func (s *PostgresStore) ListGlobalDesiredStatesWithGroups() ([]GlobalDesiredStateWithGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT gg.group_id, gg.name, gg.selector_json, COALESCE(gg.created_by,''), gg.created_at, gg.updated_at,
		       COALESCE(gds.artifact_id,''), COALESCE(gds.desired_version,''),
		       COALESCE(gds.desired_config_rev,''), COALESCE(gds.policy_json,'{}'),
		       COALESCE(gds.components_json,'{}'), COALESCE(gds.checkin_interval,0),
		       COALESCE(gds.updated_at, gg.updated_at), COALESCE(gds.updated_by,'')
		FROM global_groups gg
		LEFT JOIN global_desired_state gds ON gds.group_id = gg.group_id
		ORDER BY gg.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlobalDesiredStateWithGroup
	for rows.Next() {
		var r GlobalDesiredStateWithGroup
		if err := rows.Scan(
			&r.GlobalGroup.GroupID, &r.GlobalGroup.Name, &r.GlobalGroup.SelectorJSON,
			&r.GlobalGroup.CreatedBy, &r.GlobalGroup.CreatedAt, &r.GlobalGroup.UpdatedAt,
			&r.GlobalDesiredState.ArtifactID, &r.GlobalDesiredState.DesiredVersion,
			&r.GlobalDesiredState.DesiredConfigRev, &r.GlobalDesiredState.PolicyJSON,
			&r.GlobalDesiredState.ComponentsJSON, &r.GlobalDesiredState.CheckinInterval,
			&r.GlobalDesiredState.UpdatedAt, &r.GlobalDesiredState.UpdatedBy,
		); err != nil {
			return nil, err
		}
		r.GlobalDesiredState.GroupID = r.GlobalGroup.GroupID
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) DeleteGlobalDesiredState(groupID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM global_desired_state WHERE group_id = $1`, groupID)
	return err
}
