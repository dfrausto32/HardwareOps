package postgres

import (
	"context"
	"database/sql"
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
	components := nullIfEmptyBytes(state.ComponentsJSON)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_state (device_id, current_version, current_config_rev, services, health, components, updated_at,
			last_apply_status, last_apply_error, last_apply_at, last_apply_artifact_id,
			last_preapply_status, last_preapply_error, last_preapply_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (device_id) DO UPDATE SET
			current_version = EXCLUDED.current_version,
			current_config_rev = EXCLUDED.current_config_rev,
			services = EXCLUDED.services,
			health = EXCLUDED.health,
			components = COALESCE(EXCLUDED.components, device_state.components),
			updated_at = EXCLUDED.updated_at,
			last_apply_status = COALESCE(EXCLUDED.last_apply_status, device_state.last_apply_status),
			last_apply_error = COALESCE(EXCLUDED.last_apply_error, device_state.last_apply_error),
			last_apply_at = COALESCE(EXCLUDED.last_apply_at, device_state.last_apply_at),
			last_apply_artifact_id = COALESCE(EXCLUDED.last_apply_artifact_id, device_state.last_apply_artifact_id),
			last_preapply_status = COALESCE(EXCLUDED.last_preapply_status, device_state.last_preapply_status),
			last_preapply_error = COALESCE(EXCLUDED.last_preapply_error, device_state.last_preapply_error),
			last_preapply_at = COALESCE(EXCLUDED.last_preapply_at, device_state.last_preapply_at)
	`, state.DeviceID, state.CurrentVersion, state.CurrentConfigRev, state.ServicesJSON, state.HealthJSON, components, state.UpdatedAt,
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

func (s *Store) EnrollDeviceWithToken(tokenHash string, device store.Device, maxDevices int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if tokenHash == "" {
		return errors.New("token_hash required")
	}
	if device.DeviceID == "" {
		return errors.New("device_id required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var tokenID string
	if err := tx.QueryRow(ctx, `
		SELECT token_id::text
		FROM enrollment_tokens
		WHERE token_hash = $1 AND expires_at > now()
		FOR UPDATE
	`, tokenHash).Scan(&tokenID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.ErrEnrollmentTokenInvalid
		}
		return err
	}

	if maxDevices > 0 {
		if _, err := tx.Exec(ctx, `LOCK TABLE devices IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&count); err != nil {
			return err
		}
		if count >= maxDevices {
			return store.ErrDeviceLimitExceeded
		}
	}

	cert := nullIfEmpty(device.CertFingerprint)
	labels := nullIfEmptyBytes(device.LabelsJSON)
	metadata := nullIfEmptyBytes(device.MetadataJSON)
	if _, err := tx.Exec(ctx, `
		INSERT INTO devices (device_id, cert_fingerprint, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, device.DeviceID, cert, device.Status, device.LastSeen, labels, metadata); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_id = $1::uuid`, tokenID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

func (s *Store) GetDeviceByHardwareID(hardwareID string) (store.Device, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if hardwareID == "" {
		return store.Device{}, false, nil
	}

	var d store.Device
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, COALESCE(cert_fingerprint, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE metadata #>> '{hwops,identity,hardwareId}' = $1
	`, hardwareID).Scan(&d.DeviceID, &d.CertFingerprint, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
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
		SELECT device_id, COALESCE(current_version, ''), COALESCE(current_config_rev, ''), services, health, COALESCE(components, '{}'::jsonb), updated_at,
		       COALESCE(last_apply_status, ''), COALESCE(last_apply_error, ''), COALESCE(last_apply_at, 'epoch'::timestamptz),
		       COALESCE(last_apply_artifact_id::text, ''),
		       COALESCE(last_preapply_status, ''), COALESCE(last_preapply_error, ''), COALESCE(last_preapply_at, 'epoch'::timestamptz)
		FROM device_state
		WHERE device_id = $1
	`, deviceID).Scan(&st.DeviceID, &st.CurrentVersion, &st.CurrentConfigRev, &st.ServicesJSON, &st.HealthJSON, &st.ComponentsJSON, &st.UpdatedAt,
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

func (s *Store) CountDevices() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountDevicesByStatus(status string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE status = $1`, status).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) LatestDeviceSeen() (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var latest time.Time
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(last_seen), '0001-01-01'::timestamptz) FROM devices`).Scan(&latest); err != nil {
		return time.Time{}, err
	}
	return latest, nil
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
	components := nullIfEmptyBytes(state.ComponentsJSON)
	interval := nullIfZeroInt(state.CheckinInterval)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO desired_state_group (group_id, artifact_id, desired_version, desired_config_rev, policy, components, checkin_interval_sec, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (group_id) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			desired_version = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy = EXCLUDED.policy,
			components = COALESCE(EXCLUDED.components, desired_state_group.components),
			checkin_interval_sec = EXCLUDED.checkin_interval_sec,
			updated_at = EXCLUDED.updated_at
	`, state.GroupID, artifact, state.DesiredVersion, state.DesiredConfigRev, policy, components, interval, state.UpdatedAt)
	return err
}

func (s *Store) UpsertDesiredStateDevice(state store.DesiredStateDevice) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	artifact := nullIfEmpty(state.ArtifactID)
	policy := nullIfEmptyBytes(state.PolicyJSON)
	components := nullIfEmptyBytes(state.ComponentsJSON)
	source := state.Source
	if source == "" {
		source = "manual"
	}
	interval := nullIfZeroInt(state.CheckinInterval)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO desired_state_device (device_id, artifact_id, desired_version, desired_config_rev, policy, components, checkin_interval_sec, source, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (device_id) DO UPDATE SET
			artifact_id = EXCLUDED.artifact_id,
			desired_version = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy = EXCLUDED.policy,
			components = COALESCE(EXCLUDED.components, desired_state_device.components),
			checkin_interval_sec = EXCLUDED.checkin_interval_sec,
			source = EXCLUDED.source,
			updated_at = EXCLUDED.updated_at
	`, state.DeviceID, artifact, state.DesiredVersion, state.DesiredConfigRev, policy, components, interval, source, state.UpdatedAt)
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
		       COALESCE(components, '{}'::jsonb),
		       COALESCE(checkin_interval_sec, 0),
		       source,
		       updated_at
		FROM desired_state_device
		WHERE device_id = $1
	`, deviceID).Scan(&d.DeviceID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.Source, &d.UpdatedAt)
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
		       COALESCE(dsg.components, '{}'::jsonb),
		       COALESCE(dsg.checkin_interval_sec, 0),
		       dsg.updated_at
		FROM desired_state_group dsg
		JOIN groups g ON g.group_id = dsg.group_id
		JOIN devices d ON d.device_id = $1
		WHERE COALESCE(d.labels, '{}'::jsonb) @> g.selector
		ORDER BY dsg.updated_at DESC
		LIMIT 1
	`, deviceID).Scan(&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.UpdatedAt)
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
		       COALESCE(components, '{}'::jsonb),
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
		if err := rows.Scan(&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.UpdatedAt); err != nil {
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
		       COALESCE(components, '{}'::jsonb),
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
		if err := rows.Scan(&d.DeviceID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.Source, &d.UpdatedAt); err != nil {
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
	status := artifact.Status
	if status == "" {
		status = "active"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifacts (artifact_id, name, version, type, status, object_key, sha256, signature, size_bytes, metadata, created_at, deprecated_at, delete_after)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, artifact.ArtifactID, artifact.Name, artifact.Version, atype, status, artifact.ObjectKey, artifact.SHA256, nullIfEmpty(artifact.Signature), artifact.SizeBytes, nullIfEmptyBytes(artifact.MetadataJSON), artifact.CreatedAt, nullIfZeroTime(artifact.DeprecatedAt), nullIfZeroTime(artifact.DeleteAfter))
	return err
}

func (s *Store) GetArtifact(artifactID string) (store.Artifact, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a store.Artifact
	err := s.pool.QueryRow(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''), size_bytes, COALESCE(metadata, '{}'::jsonb), created_at, COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz)
		FROM artifacts
		WHERE artifact_id = $1
	`, artifactID).Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter)
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
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''), size_bytes, COALESCE(metadata, '{}'::jsonb), created_at, COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz)
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
		if err := rows.Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeprecateArtifact(artifactID string, deprecatedAt, deleteAfter time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if artifactID == "" {
		return errors.New("artifact_id required")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE artifacts
		SET status = 'deprecated',
		    deprecated_at = $2,
		    delete_after = $3
		WHERE artifact_id = $1
	`, artifactID, deprecatedAt, nullIfZeroTime(deleteAfter))
	return err
}

func (s *Store) RestoreArtifact(artifactID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if artifactID == "" {
		return errors.New("artifact_id required")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE artifacts
		SET status = 'active',
		    deprecated_at = NULL,
		    delete_after = NULL
		WHERE artifact_id = $1
	`, artifactID)
	return err
}

func (s *Store) ListArtifactsForPrune(cutoff time.Time, limit int) ([]store.Artifact, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''), size_bytes, COALESCE(metadata, '{}'::jsonb), created_at, COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz)
		FROM artifacts
		WHERE status = 'deprecated'
		  AND delete_after IS NOT NULL
		  AND delete_after <= $1
		ORDER BY delete_after ASC
		LIMIT $2
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.Artifact, 0, limit)
	for rows.Next() {
		var a store.Artifact
		if err := rows.Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) CountArtifactReferences(artifactID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if artifactID == "" {
		return 0, errors.New("artifact_id required")
	}

	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT COUNT(*) FROM desired_state_device WHERE artifact_id::text = $1), 0) +
			COALESCE((SELECT COUNT(*) FROM desired_state_group WHERE artifact_id::text = $1), 0) +
			COALESCE((
				SELECT COUNT(*)
				FROM desired_state_device d
				CROSS JOIN LATERAL jsonb_each(COALESCE(d.components, '{}'::jsonb)) j
				WHERE j.value->>'artifactId' = $1
			), 0) +
			COALESCE((
				SELECT COUNT(*)
				FROM desired_state_group g
				CROSS JOIN LATERAL jsonb_each(COALESCE(g.components, '{}'::jsonb)) j
				WHERE j.value->>'artifactId' = $1
			), 0)
	`, artifactID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) GetArtifactLifecyclePolicy() (store.ArtifactLifecyclePolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var policy store.ArtifactLifecyclePolicy
	err := s.pool.QueryRow(ctx, `
		SELECT deprecated_delete_after_days, updated_at
		FROM artifact_lifecycle_policy
		WHERE policy_id = 1
	`).Scan(&policy.DeprecatedDeleteAfterDays, &policy.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		now := time.Now().UTC()
		return store.ArtifactLifecyclePolicy{
			DeprecatedDeleteAfterDays: 30,
			UpdatedAt:                 now,
		}, nil
	}
	if err != nil {
		return store.ArtifactLifecyclePolicy{}, err
	}
	return policy, nil
}

func (s *Store) SetArtifactLifecyclePolicy(days int) (store.ArtifactLifecyclePolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if days < 1 {
		return store.ArtifactLifecyclePolicy{}, errors.New("days must be >= 1")
	}
	var policy store.ArtifactLifecyclePolicy
	err := s.pool.QueryRow(ctx, `
		INSERT INTO artifact_lifecycle_policy (policy_id, deprecated_delete_after_days, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (policy_id)
		DO UPDATE SET deprecated_delete_after_days = EXCLUDED.deprecated_delete_after_days, updated_at = now()
		RETURNING deprecated_delete_after_days, updated_at
	`, days).Scan(&policy.DeprecatedDeleteAfterDays, &policy.UpdatedAt)
	if err != nil {
		return store.ArtifactLifecyclePolicy{}, err
	}
	return policy, nil
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
		UPDATE desired_state_device
		SET components = (
			SELECT COALESCE(jsonb_object_agg(key, value), '{}'::jsonb)
			FROM jsonb_each(components)
			WHERE value->>'artifactId' <> $1
		)
		WHERE components IS NOT NULL AND components <> '{}'::jsonb
	`, artifactID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE desired_state_group
		SET components = (
			SELECT COALESCE(jsonb_object_agg(key, value), '{}'::jsonb)
			FROM jsonb_each(components)
			WHERE value->>'artifactId' <> $1
		)
		WHERE components IS NOT NULL AND components <> '{}'::jsonb
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

func (s *Store) GetArtifactStats() (store.ArtifactStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stats store.ArtifactStats
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM artifacts
	`).Scan(&stats.Count, &stats.SizeBytes); err != nil {
		return store.ArtifactStats{}, err
	}
	return stats, nil
}

func (s *Store) CreateApplyResult(result store.ApplyResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_apply_results (apply_id, device_id, artifact_id, component, status, applied_version, applied_config_rev, error, preapply_status, preapply_error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, result.ApplyID, result.DeviceID, nullIfEmpty(result.ArtifactID), nullIfEmpty(result.Component), result.Status, nullIfEmpty(result.AppliedVersion),
		nullIfEmpty(result.AppliedConfigRev), nullIfEmpty(result.Error), nullIfEmpty(result.PreApplyStatus), nullIfEmpty(result.PreApplyError), result.CreatedAt)
	return err
}

func (s *Store) CreateRuntimeEvent(event store.RuntimeEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO runtime_events (
			event_id, occurred_at, event_type, device_id, payload
		) VALUES (
			COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5
		)
	`, nullIfEmpty(event.EventID),
		occurredAt,
		event.Type,
		nullIfEmpty(event.DeviceID),
		nullIfEmptyBytes(event.PayloadJSON),
	)
	return err
}

func (s *Store) ListRuntimeEvents(filter store.RuntimeEventFilter) ([]store.RuntimeEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT event_id, occurred_at, event_type, COALESCE(device_id, ''), COALESCE(payload, '{}'::jsonb)
		FROM runtime_events
		WHERE ($1 = '' OR event_type = $1)
		  AND ($2 = '' OR device_id = $2)
		  AND ($3::timestamptz IS NULL OR occurred_at >= $3)
		  AND ($4::timestamptz IS NULL OR occurred_at <= $4)
		ORDER BY occurred_at DESC
		LIMIT $5 OFFSET $6
	`, filter.Type, filter.DeviceID, nullIfZeroTime(filter.Since), nullIfZeroTime(filter.Until), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.RuntimeEvent{}
	for rows.Next() {
		var ev store.RuntimeEvent
		if err := rows.Scan(&ev.EventID, &ev.OccurredAt, &ev.Type, &ev.DeviceID, &ev.PayloadJSON); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteRuntimeEventsBefore(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cutoff.IsZero() {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM runtime_events WHERE occurred_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) EnsureRuntimeEventRetentionDays(days int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if days <= 0 {
		days = 30
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO runtime_event_retention (id, days, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET days = EXCLUDED.days, updated_at = now()
	`, days)
	return err
}

func (s *Store) GetRuntimeEventRetentionDays() (store.RuntimeEventRetention, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out store.RuntimeEventRetention
	err := s.pool.QueryRow(ctx, `
		SELECT days, updated_at
		FROM runtime_event_retention
		WHERE id = 1
	`).Scan(&out.Days, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Days = 30
		out.UpdatedAt = time.Now().UTC()
		return out, nil
	}
	if err != nil {
		return store.RuntimeEventRetention{}, err
	}
	return out, nil
}

func (s *Store) SetRuntimeEventRetentionDays(days int) (store.RuntimeEventRetention, error) {
	if err := s.EnsureRuntimeEventRetentionDays(days); err != nil {
		return store.RuntimeEventRetention{}, err
	}
	return s.GetRuntimeEventRetentionDays()
}

func (s *Store) CreateAuditEvent(event store.AuditEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_events (
			event_id, occurred_at, actor_type, actor_id, actor_email, actor_roles,
			auth_method, source_ip, user_agent, request_id, action,
			target_type, target_id, status, error, before, after, metadata
		) VALUES (
			COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18
		)
	`, nullIfEmpty(event.EventID),
		occurredAt,
		nullIfEmpty(event.ActorType),
		nullIfEmpty(event.ActorID),
		nullIfEmpty(event.ActorEmail),
		nullIfEmptyBytes(event.ActorRolesJSON),
		nullIfEmpty(event.AuthMethod),
		nullIfEmpty(event.SourceIP),
		nullIfEmpty(event.UserAgent),
		nullIfEmpty(event.RequestID),
		event.Action,
		nullIfEmpty(event.TargetType),
		nullIfEmpty(event.TargetID),
		nullIfEmpty(event.Status),
		nullIfEmpty(event.Error),
		nullIfEmptyBytes(event.BeforeJSON),
		nullIfEmptyBytes(event.AfterJSON),
		nullIfEmptyBytes(event.MetadataJSON),
	)
	return err
}

func (s *Store) ListAuditEvents(filter store.AuditEventFilter) ([]store.AuditEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.pool.Query(ctx, `
		SELECT event_id, occurred_at, actor_type, actor_id, COALESCE(actor_email, ''), COALESCE(actor_roles, '[]'::jsonb),
		       COALESCE(auth_method, ''), COALESCE(source_ip, ''), COALESCE(user_agent, ''), COALESCE(request_id, ''),
		       action, COALESCE(target_type, ''), COALESCE(target_id, ''), status, COALESCE(error, ''),
		       COALESCE(before, '{}'::jsonb), COALESCE(after, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb)
		FROM audit_events
		WHERE ($1 = '' OR action = $1)
		  AND ($2 = '' OR actor_type = $2)
		  AND ($3 = '' OR actor_id = $3)
		  AND ($4 = '' OR actor_email = $4)
		  AND ($5 = '' OR target_type = $5)
		  AND ($6 = '' OR target_id = $6)
		  AND ($7 = '' OR status = $7)
		  AND ($8::timestamptz IS NULL OR occurred_at >= $8)
		  AND ($9::timestamptz IS NULL OR occurred_at <= $9)
		ORDER BY occurred_at DESC
		LIMIT $10 OFFSET $11
	`, filter.Action, filter.ActorType, filter.ActorID, filter.ActorEmail, filter.TargetType, filter.TargetID, filter.Status,
		nullIfZeroTime(filter.Since), nullIfZeroTime(filter.Until), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.AuditEvent{}
	for rows.Next() {
		var ev store.AuditEvent
		if err := rows.Scan(&ev.EventID, &ev.OccurredAt, &ev.ActorType, &ev.ActorID, &ev.ActorEmail, &ev.ActorRolesJSON,
			&ev.AuthMethod, &ev.SourceIP, &ev.UserAgent, &ev.RequestID, &ev.Action, &ev.TargetType, &ev.TargetID,
			&ev.Status, &ev.Error, &ev.BeforeJSON, &ev.AfterJSON, &ev.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) DeleteAuditEventsBefore(cutoff time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cutoff.IsZero() {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE occurred_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) EnsureAuditRetentionDays(days int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if days <= 0 {
		days = 90
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_retention (id, days, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET days = EXCLUDED.days, updated_at = now()
	`, days)
	return err
}

func (s *Store) GetAuditRetentionDays() (store.AuditRetention, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out store.AuditRetention
	err := s.pool.QueryRow(ctx, `
		SELECT days, updated_at
		FROM audit_retention
		WHERE id = 1
	`).Scan(&out.Days, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Days = 90
		out.UpdatedAt = time.Now().UTC()
		return out, nil
	}
	if err != nil {
		return store.AuditRetention{}, err
	}
	return out, nil
}

func (s *Store) SetAuditRetentionDays(days int) (store.AuditRetention, error) {
	if err := s.EnsureAuditRetentionDays(days); err != nil {
		return store.AuditRetention{}, err
	}
	return s.GetAuditRetentionDays()
}

func (s *Store) GetCertRotationState() (store.CertRotationState, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st store.CertRotationState
	var cleanedAt sql.NullTime
	var cleanedReason sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT active_fingerprint, COALESCE(previous_fingerprint, ''), rotated_at, grace_period_seconds,
		       cleaned_at, cleaned_reason
		FROM cert_rotation_state
		WHERE id = 1
	`).Scan(&st.ActiveFingerprint, &st.PreviousFingerprint, &st.RotatedAt, &st.GracePeriodSeconds, &cleanedAt, &cleanedReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.CertRotationState{}, false, nil
	}
	if err != nil {
		return store.CertRotationState{}, false, err
	}
	if cleanedAt.Valid {
		st.CleanedAt = cleanedAt.Time
	}
	if cleanedReason.Valid {
		st.CleanedReason = cleanedReason.String
	}
	return st, true, nil
}

func (s *Store) SetCertRotationState(state store.CertRotationState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cert_rotation_state (
			id, active_fingerprint, previous_fingerprint, rotated_at, grace_period_seconds, cleaned_at, cleaned_reason
		) VALUES (1, $1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			active_fingerprint = EXCLUDED.active_fingerprint,
			previous_fingerprint = EXCLUDED.previous_fingerprint,
			rotated_at = EXCLUDED.rotated_at,
			grace_period_seconds = EXCLUDED.grace_period_seconds,
			cleaned_at = EXCLUDED.cleaned_at,
			cleaned_reason = EXCLUDED.cleaned_reason
	`, state.ActiveFingerprint, nullIfEmpty(state.PreviousFingerprint), state.RotatedAt, state.GracePeriodSeconds,
		nullIfZeroTime(state.CleanedAt), nullIfEmpty(state.CleanedReason))
	return err
}

func (s *Store) CreateUser(user store.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if user.AuthProvider == "" {
		user.AuthProvider = "local"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (user_id, email, display_name, password_hash, roles, disabled, auth_provider, external_id, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, user.UserID, user.Email, nullIfEmpty(user.DisplayName), user.PasswordHash, nullIfEmptyBytes(user.RolesJSON),
		user.Disabled, user.AuthProvider, nullIfEmpty(user.ExternalID), user.CreatedAt, user.UpdatedAt, nullIfZeroTime(user.LastLoginAt))
	return err
}

func (s *Store) GetUser(userID string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       disabled, COALESCE(auth_provider, 'local'), COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz)
		FROM users
		WHERE user_id = $1
	`, userID).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.Disabled,
		&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	return u, true, nil
}

func (s *Store) GetUserByEmail(email string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       disabled, COALESCE(auth_provider, 'local'), COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz)
		FROM users
		WHERE email = $1
	`, email).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.Disabled,
		&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	return u, true, nil
}

func (s *Store) ListUsers(limit, offset int) ([]store.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       disabled, COALESCE(auth_provider, 'local'), COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz)
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.User{}
	for rows.Next() {
		var u store.User
		if err := rows.Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.Disabled,
			&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) UpdateUser(update store.UserUpdate) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if update.UserID == "" {
		return errors.New("user_id required")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET display_name = COALESCE($2, display_name),
		    roles = COALESCE($3, roles),
		    disabled = COALESCE($4, disabled),
		    password_hash = COALESCE($5, password_hash),
		    updated_at = now()
		WHERE user_id = $1
	`, update.UserID,
		nullIfEmpty(ptrString(update.DisplayName)),
		nullIfEmptyBytes(update.RolesJSON),
		ptrBool(update.Disabled),
		nullIfEmpty(ptrString(update.PasswordHash)),
	)
	return err
}

func (s *Store) SetUserLastLogin(userID string, at time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET last_login_at = $2,
		    updated_at = now()
		WHERE user_id = $1
	`, userID, at)
	return err
}

func (s *Store) CreateAuthVoucher(voucher store.AuthVoucher) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_vouchers (voucher_id, token_hash, email, roles, expires_at, created_at, created_by, used_at, used_by, revoked)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, voucher.VoucherID,
		voucher.TokenHash,
		nullIfEmpty(voucher.Email),
		nullIfEmptyBytes(voucher.RolesJSON),
		voucher.ExpiresAt,
		voucher.CreatedAt,
		nullIfEmpty(voucher.CreatedBy),
		nullIfZeroTime(voucher.UsedAt),
		nullIfEmpty(voucher.UsedBy),
		voucher.Revoked,
	)
	return err
}

func (s *Store) GetAuthVoucherByTokenHash(tokenHash string) (store.AuthVoucher, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var v store.AuthVoucher
	err := s.pool.QueryRow(ctx, `
		SELECT voucher_id, token_hash, COALESCE(email, ''), COALESCE(roles, '[]'::jsonb),
		       expires_at, created_at, COALESCE(created_by, ''), COALESCE(used_at, '0001-01-01'::timestamptz),
		       COALESCE(used_by, ''), revoked
		FROM auth_vouchers
		WHERE token_hash = $1
	`, tokenHash).Scan(&v.VoucherID, &v.TokenHash, &v.Email, &v.RolesJSON, &v.ExpiresAt, &v.CreatedAt,
		&v.CreatedBy, &v.UsedAt, &v.UsedBy, &v.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AuthVoucher{}, false, nil
	}
	if err != nil {
		return store.AuthVoucher{}, false, err
	}
	return v, true, nil
}

func (s *Store) MarkAuthVoucherUsed(voucherID, usedBy string, at time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE auth_vouchers
		SET used_at = $2,
		    used_by = $3
		WHERE voucher_id = $1
		  AND revoked = false
		  AND used_at IS NULL
	`, voucherID, at, nullIfEmpty(usedBy))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) CreateServiceToken(token store.ServiceToken) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO service_tokens (token_id, name, token_hash, scopes, expires_at, created_at, created_by, last_used_at, revoked_at, revoked_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, token.TokenID,
		token.Name,
		token.TokenHash,
		nullIfEmptyBytes(token.ScopesJSON),
		token.ExpiresAt,
		token.CreatedAt,
		nullIfEmpty(token.CreatedBy),
		nullIfZeroTime(token.LastUsedAt),
		nullIfZeroTime(token.RevokedAt),
		nullIfEmpty(token.RevokedBy),
	)
	return err
}

func (s *Store) GetServiceTokenByTokenHash(tokenHash string) (store.ServiceToken, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var token store.ServiceToken
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime
	var revokedBy sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT token_id, name, token_hash, COALESCE(scopes, '[]'::jsonb),
		       expires_at, created_at, COALESCE(created_by, ''), last_used_at, revoked_at, revoked_by
		FROM service_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&token.TokenID, &token.Name, &token.TokenHash, &token.ScopesJSON,
		&token.ExpiresAt, &token.CreatedAt, &token.CreatedBy, &lastUsedAt, &revokedAt, &revokedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ServiceToken{}, false, nil
	}
	if err != nil {
		return store.ServiceToken{}, false, err
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = revokedAt.Time
	}
	if revokedBy.Valid {
		token.RevokedBy = revokedBy.String
	}
	return token, true, nil
}

func (s *Store) ListServiceTokens(limit, offset int) ([]store.ServiceToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT token_id, name, token_hash, COALESCE(scopes, '[]'::jsonb),
		       expires_at, created_at, COALESCE(created_by, ''),
		       last_used_at, revoked_at, revoked_by
		FROM service_tokens
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.ServiceToken{}
	for rows.Next() {
		var token store.ServiceToken
		var lastUsedAt sql.NullTime
		var revokedAt sql.NullTime
		var revokedBy sql.NullString
		if err := rows.Scan(&token.TokenID, &token.Name, &token.TokenHash, &token.ScopesJSON,
			&token.ExpiresAt, &token.CreatedAt, &token.CreatedBy, &lastUsedAt, &revokedAt, &revokedBy); err != nil {
			return nil, err
		}
		if lastUsedAt.Valid {
			token.LastUsedAt = lastUsedAt.Time
		}
		if revokedAt.Valid {
			token.RevokedAt = revokedAt.Time
		}
		if revokedBy.Valid {
			token.RevokedBy = revokedBy.String
		}
		out = append(out, token)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) SetServiceTokenLastUsed(tokenID string, at time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE service_tokens
		SET last_used_at = $2
		WHERE token_id = $1
	`, tokenID, at)
	return err
}

func (s *Store) RevokeServiceToken(tokenID, revokedBy string, at time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE service_tokens
		SET revoked_at = $2,
		    revoked_by = $3
		WHERE token_id = $1
		  AND revoked_at IS NULL
	`, tokenID, at, nullIfEmpty(revokedBy))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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

func ptrString(val *string) string {
	if val == nil {
		return ""
	}
	return *val
}

func ptrBool(val *bool) interface{} {
	if val == nil {
		return nil
	}
	return *val
}
