package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hardwareops/control-plane/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) UpsertDevice(device store.Device) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cert := nullIfEmpty(device.CertFingerprint)
	labels := nullIfEmptyBytes(device.LabelsJSON)
	metadata := nullIfEmptyBytes(device.MetadataJSON)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (device_id, cert_fingerprint, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (device_id) DO UPDATE SET
			cert_fingerprint = COALESCE(EXCLUDED.cert_fingerprint, devices.cert_fingerprint),
			status = COALESCE(EXCLUDED.status, devices.status),
			last_seen = EXCLUDED.last_seen,
			labels = COALESCE(EXCLUDED.labels, devices.labels),
			metadata = COALESCE(EXCLUDED.metadata, devices.metadata)
	`, device.DeviceID, cert, device.Status, device.LastSeen, labels, metadata)
	return err
}

func (s *Store) UpsertDeviceState(state store.DeviceState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_state (device_id, current_version, current_config_rev, services, health, updated_at,
			last_apply_status, last_apply_error, last_apply_at, last_apply_artifact_id,
			last_preapply_status, last_preapply_error, last_preapply_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (device_id) DO UPDATE SET
			current_version = EXCLUDED.current_version,
			current_config_rev = EXCLUDED.current_config_rev,
			services = EXCLUDED.services,
			health = EXCLUDED.health,
			updated_at = EXCLUDED.updated_at,
			last_apply_status = COALESCE(EXCLUDED.last_apply_status, device_state.last_apply_status),
			last_apply_error = COALESCE(EXCLUDED.last_apply_error, device_state.last_apply_error),
			last_apply_at = COALESCE(EXCLUDED.last_apply_at, device_state.last_apply_at),
			last_apply_artifact_id = COALESCE(EXCLUDED.last_apply_artifact_id, device_state.last_apply_artifact_id),
			last_preapply_status = COALESCE(EXCLUDED.last_preapply_status, device_state.last_preapply_status),
			last_preapply_error = COALESCE(EXCLUDED.last_preapply_error, device_state.last_preapply_error),
			last_preapply_at = COALESCE(EXCLUDED.last_preapply_at, device_state.last_preapply_at)
	`, state.DeviceID, state.CurrentVersion, state.CurrentConfigRev, state.ServicesJSON, state.HealthJSON, state.UpdatedAt,
		nullIfEmpty(state.LastApplyStatus), nullIfEmpty(state.LastApplyError), nullIfZeroTime(state.LastApplyAt), nullIfEmpty(state.LastApplyArtifactID),
		nullIfEmpty(state.LastPreApplyStatus), nullIfEmpty(state.LastPreApplyError), nullIfZeroTime(state.LastPreApplyAt))
	return err
}

func (s *Store) CreateEnrollmentToken(tokenHash string, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollment_tokens (token_id, token_hash, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, now())
	`, tokenHash, expiresAt)
	return err
}

func (s *Store) ConsumeEnrollmentToken(tokenHash string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var ok bool
	row := s.pool.QueryRow(ctx, `
		DELETE FROM enrollment_tokens
		WHERE token_hash = $1 AND expires_at > now()
		RETURNING true
	`, tokenHash)
	if err := row.Scan(&ok); err != nil {
		return false, nil
	}
	return ok, nil
}

func (s *Store) CreateDevice(device store.Device) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cert := nullIfEmpty(device.CertFingerprint)
	labels := nullIfEmptyBytes(device.LabelsJSON)
	metadata := nullIfEmptyBytes(device.MetadataJSON)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (device_id, cert_fingerprint, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, device.DeviceID, cert, device.Status, device.LastSeen, labels, metadata)
	return err
}

func (s *Store) GetDevice(deviceID string) (store.Device, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.Device
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, COALESCE(cert_fingerprint, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE device_id = $1
	`, deviceID).Scan(&d.DeviceID, &d.CertFingerprint, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Device{}, false, nil
	}
	if err != nil {
		return store.Device{}, false, err
	}
	return d, true, nil
}

func (s *Store) GetDeviceByFingerprint(fingerprint string) (store.Device, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.Device
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, COALESCE(cert_fingerprint, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE cert_fingerprint = $1
	`, fingerprint).Scan(&d.DeviceID, &d.CertFingerprint, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Device{}, false, nil
	}
	if err != nil {
		return store.Device{}, false, err
	}
	return d, true, nil
}

func (s *Store) GetDeviceState(deviceID string) (store.DeviceState, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var st store.DeviceState
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, COALESCE(current_version, ''), COALESCE(current_config_rev, ''), services, health, updated_at,
		       COALESCE(last_apply_status, ''), COALESCE(last_apply_error, ''), COALESCE(last_apply_at, 'epoch'::timestamptz),
		       COALESCE(last_apply_artifact_id::text, ''),
		       COALESCE(last_preapply_status, ''), COALESCE(last_preapply_error, ''), COALESCE(last_preapply_at, 'epoch'::timestamptz)
		FROM device_state
		WHERE device_id = $1
	`, deviceID).Scan(&st.DeviceID, &st.CurrentVersion, &st.CurrentConfigRev, &st.ServicesJSON, &st.HealthJSON, &st.UpdatedAt,
		&st.LastApplyStatus, &st.LastApplyError, &st.LastApplyAt, &st.LastApplyArtifactID,
		&st.LastPreApplyStatus, &st.LastPreApplyError, &st.LastPreApplyAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DeviceState{}, false, nil
	}
	if err != nil {
		return store.DeviceState{}, false, err
	}
	return st, true, nil
}

func (s *Store) ListDevices(filter store.ListDevicesFilter) ([]store.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT device_id, COALESCE(cert_fingerprint, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE ($1 = '' OR status = $1)
		ORDER BY last_seen DESC NULLS LAST, device_id
		LIMIT $2 OFFSET $3
	`, filter.Status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.Device{}
	for rows.Next() {
		var d store.Device
		if err := rows.Scan(&d.DeviceID, &d.CertFingerprint, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteDevice(deviceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM desired_state_device WHERE device_id = $1
	`, deviceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM devices WHERE device_id = $1
	`, deviceID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) DeleteStaleDevices(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM desired_state_device
		WHERE device_id IN (
			SELECT device_id FROM devices WHERE last_seen < $1
		)
	`, cutoff); err != nil {
		return 0, err
	}

	rows, err := tx.Query(ctx, `
		DELETE FROM devices
		WHERE last_seen < $1
		RETURNING device_id
	`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		count++
	}
	if rows.Err() != nil {
		return 0, rows.Err()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) UpdateDeviceStatuses(staleCutoff, offlineCutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(ctx, `
		UPDATE devices d
		SET status = CASE
			WHEN d.last_seen IS NULL OR d.last_seen < $1 THEN 'offline'
			WHEN d.last_seen < $2 THEN 'stale'
			WHEN EXISTS (
				SELECT 1 FROM device_state ds
				WHERE ds.device_id = d.device_id
				  AND (ds.last_apply_status = 'error' OR ds.last_preapply_status = 'error')
			) THEN 'degraded'
			ELSE 'active'
		END
	`, offlineCutoff, staleCutoff)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) UpsertGroup(group store.Group) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	name := nullIfEmpty(group.Name)
	selector := group.SelectorJSON
	if len(selector) == 0 {
		selector = []byte("{}")
	}
	created := group.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO groups (group_id, name, selector, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (group_id) DO UPDATE SET
			name = COALESCE(EXCLUDED.name, groups.name),
			selector = EXCLUDED.selector
	`, group.GroupID, name, selector, created)
	return err
}

func (s *Store) ListGroups() ([]store.Group, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT group_id, COALESCE(name, ''), selector, created_at
		FROM groups
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.Group{}
	for rows.Next() {
		var g store.Group
		if err := rows.Scan(&g.GroupID, &g.Name, &g.SelectorJSON, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteGroup(groupID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if groupID == "" {
		return errors.New("group_id required")
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM desired_state_group WHERE group_id = $1`, groupID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE group_id = $1`, groupID)
	return err
}

func (s *Store) UpsertDesiredStateGroup(state store.DesiredStateGroup) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	artifact := nullIfEmpty(state.ArtifactID)
	policy := nullIfEmptyBytes(state.PolicyJSON)
	interval := nullIfZeroInt(state.CheckinInterval)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO desired_state_group (group_id, artifact_id, desired_version, desired_config_rev, policy, checkin_interval_sec, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (group_id) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			desired_version = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy = EXCLUDED.policy,
			checkin_interval_sec = EXCLUDED.checkin_interval_sec,
			updated_at = EXCLUDED.updated_at
	`, state.GroupID, artifact, state.DesiredVersion, state.DesiredConfigRev, policy, interval, state.UpdatedAt)
	return err
}

func (s *Store) UpsertDesiredStateDevice(state store.DesiredStateDevice) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	artifact := nullIfEmpty(state.ArtifactID)
	policy := nullIfEmptyBytes(state.PolicyJSON)
	source := state.Source
	if source == "" {
		source = "manual"
	}
	interval := nullIfZeroInt(state.CheckinInterval)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO desired_state_device (device_id, artifact_id, desired_version, desired_config_rev, policy, checkin_interval_sec, source, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (device_id) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			desired_version = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy = EXCLUDED.policy,
			checkin_interval_sec = EXCLUDED.checkin_interval_sec,
			source = EXCLUDED.source,
			updated_at = EXCLUDED.updated_at
	`, state.DeviceID, artifact, state.DesiredVersion, state.DesiredConfigRev, policy, interval, source, state.UpdatedAt)
	return err
}

func (s *Store) GetDesiredStateDevice(deviceID string) (store.DesiredStateDevice, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.DesiredStateDevice
	err := s.pool.QueryRow(ctx, `
		SELECT device_id,
		       COALESCE(artifact_id::text, ''),
		       COALESCE(desired_version, ''),
		       COALESCE(desired_config_rev, ''),
		       policy,
		       COALESCE(checkin_interval_sec, 0),
		       source,
		       updated_at
		FROM desired_state_device
		WHERE device_id = $1
	`, deviceID).Scan(&d.DeviceID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.CheckinInterval, &d.Source, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DesiredStateDevice{}, false, nil
	}
	if err != nil {
		return store.DesiredStateDevice{}, false, err
	}
	return d, true, nil
}

func (s *Store) GetDesiredStateGroupForDevice(deviceID string) (store.DesiredStateGroup, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.DesiredStateGroup
	err := s.pool.QueryRow(ctx, `
		SELECT dsg.group_id,
		       COALESCE(dsg.artifact_id::text, ''),
		       COALESCE(dsg.desired_version, ''),
		       COALESCE(dsg.desired_config_rev, ''),
		       dsg.policy,
		       COALESCE(dsg.checkin_interval_sec, 0),
		       dsg.updated_at
		FROM desired_state_group dsg
		JOIN groups g ON g.group_id = dsg.group_id
		JOIN devices d ON d.device_id = $1
		WHERE COALESCE(d.labels, '{}'::jsonb) @> g.selector
		ORDER BY dsg.updated_at DESC
		LIMIT 1
	`, deviceID).Scan(&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.CheckinInterval, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DesiredStateGroup{}, false, nil
	}
	if err != nil {
		return store.DesiredStateGroup{}, false, err
	}
	return d, true, nil
}

func (s *Store) ListDesiredStateGroups() ([]store.DesiredStateGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT group_id,
		       COALESCE(artifact_id::text, ''),
		       COALESCE(desired_version, ''),
		       COALESCE(desired_config_rev, ''),
		       policy,
		       COALESCE(checkin_interval_sec, 0),
		       updated_at
		FROM desired_state_group
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.DesiredStateGroup{}
	for rows.Next() {
		var d store.DesiredStateGroup
		if err := rows.Scan(&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.CheckinInterval, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteDesiredStateGroup(groupID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if groupID == "" {
		return errors.New("group_id required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM desired_state_group WHERE group_id = $1`, groupID)
	return err
}

func (s *Store) ListDesiredStateDevices() ([]store.DesiredStateDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT device_id,
		       COALESCE(artifact_id::text, ''),
		       COALESCE(desired_version, ''),
		       COALESCE(desired_config_rev, ''),
		       policy,
		       COALESCE(checkin_interval_sec, 0),
		       source,
		       updated_at
		FROM desired_state_device
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.DesiredStateDevice{}
	for rows.Next() {
		var d store.DesiredStateDevice
		if err := rows.Scan(&d.DeviceID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.CheckinInterval, &d.Source, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteDesiredStateDevice(deviceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if deviceID == "" {
		return errors.New("device_id required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM desired_state_device WHERE device_id = $1`, deviceID)
	return err
}

func (s *Store) CreateArtifact(artifact store.Artifact) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	atype := artifact.Type
	if atype == "" {
		atype = "app_bundle"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifacts (artifact_id, name, version, type, object_key, sha256, signature, size_bytes, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, artifact.ArtifactID, artifact.Name, artifact.Version, atype, artifact.ObjectKey, artifact.SHA256, nullIfEmpty(artifact.Signature), artifact.SizeBytes, nullIfEmptyBytes(artifact.MetadataJSON), artifact.CreatedAt)
	return err
}

func (s *Store) GetArtifact(artifactID string) (store.Artifact, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a store.Artifact
	err := s.pool.QueryRow(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), object_key, sha256, COALESCE(signature, ''), size_bytes, COALESCE(metadata, '{}'::jsonb), created_at
		FROM artifacts
		WHERE artifact_id = $1
	`, artifactID).Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Artifact{}, false, nil
	}
	if err != nil {
		return store.Artifact{}, false, err
	}
	return a, true, nil
}

func (s *Store) ListArtifacts(name, version string, limit, offset int) ([]store.Artifact, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), object_key, sha256, COALESCE(signature, ''), size_bytes, COALESCE(metadata, '{}'::jsonb), created_at
		FROM artifacts
		WHERE ($1 = '' OR name = $1)
		  AND ($2 = '' OR version = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, name, version, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.Artifact{}
	for rows.Next() {
		var a store.Artifact
		if err := rows.Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteArtifact(artifactID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM desired_state_device WHERE artifact_id = $1
	`, artifactID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM desired_state_group WHERE artifact_id = $1
	`, artifactID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM artifacts WHERE artifact_id = $1
	`, artifactID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) CreateApplyResult(result store.ApplyResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	preApplyAt := time.Time{}
	if result.PreApplyStatus != "" {
		preApplyAt = result.CreatedAt
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_apply_results (apply_id, device_id, artifact_id, status, applied_version, applied_config_rev, error, preapply_status, preapply_error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, result.ApplyID, result.DeviceID, nullIfEmpty(result.ArtifactID), result.Status, nullIfEmpty(result.AppliedVersion), nullIfEmpty(result.AppliedConfigRev),
		nullIfEmpty(result.Error), nullIfEmpty(result.PreApplyStatus), nullIfEmpty(result.PreApplyError), result.CreatedAt)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE device_state
		SET last_apply_status = $2,
		    last_apply_error = $3,
		    last_apply_at = $4,
		    last_apply_artifact_id = COALESCE($5, last_apply_artifact_id),
		    last_preapply_status = COALESCE($6, last_preapply_status),
		    last_preapply_error = COALESCE($7, last_preapply_error),
		    last_preapply_at = COALESCE($8, last_preapply_at),
		    current_version = COALESCE($9, current_version),
		    current_config_rev = COALESCE($10, current_config_rev),
		    updated_at = now()
		WHERE device_id = $1
	`, result.DeviceID,
		result.Status,
		nullIfEmpty(result.Error),
		result.CreatedAt,
		nullIfEmpty(result.ArtifactID),
		nullIfEmpty(result.PreApplyStatus),
		nullIfEmpty(result.PreApplyError),
		nullIfZeroTime(preApplyAt),
		nullIfEmpty(result.AppliedVersion),
		nullIfEmpty(result.AppliedConfigRev))
	return err
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmptyBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullIfZeroTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullIfZeroInt(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
