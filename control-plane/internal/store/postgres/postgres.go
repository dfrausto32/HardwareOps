package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/parcel/control-plane/internal/store"
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
		INSERT INTO devices (device_id, cert_fingerprint, cert_serial, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (device_id) DO UPDATE SET
			cert_fingerprint = COALESCE(EXCLUDED.cert_fingerprint, devices.cert_fingerprint),
			cert_serial = CASE WHEN EXCLUDED.cert_serial != '' THEN EXCLUDED.cert_serial ELSE devices.cert_serial END,
			status = COALESCE(EXCLUDED.status, devices.status),
			last_seen = EXCLUDED.last_seen,
			labels = COALESCE(EXCLUDED.labels, devices.labels),
			metadata = COALESCE(EXCLUDED.metadata, devices.metadata)
	`, device.DeviceID, cert, device.CertSerial, device.Status, device.LastSeen, labels, metadata)
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
		INSERT INTO devices (device_id, cert_fingerprint, cert_serial, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, device.DeviceID, cert, device.CertSerial, device.Status, device.LastSeen, labels, metadata); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM enrollment_tokens WHERE token_id = $1::uuid`, tokenID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateEnrollmentProfile(profile store.EnrollmentProfile) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if profile.ProfileID == "" {
		return errors.New("profile_id required")
	}
	if profile.Name == "" {
		return errors.New("name required")
	}
	if profile.TokenHash == "" {
		return errors.New("token_hash required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollment_profiles (
			profile_id, name, token_hash, previous_token_hash, previous_token_expires_at, token_rotated_at, require_approval, allow_untrusted_hw,
			challenge_hash, challenge_hint, approval_delay_seconds,
			max_uses, uses, expires_at, default_labels, created_at, created_by, disabled, cert_validity_days
		)
		VALUES (
			$1::uuid, $2, $3, NULLIF($4, ''), $5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15, COALESCE($16, now()), $17, $18, $19
		)
	`, profile.ProfileID, profile.Name, profile.TokenHash, nullIfEmpty(profile.PreviousTokenHash), nullIfZeroTime(profile.PreviousTokenExpiresAt), nullIfZeroTime(profile.TokenRotatedAt), profile.RequireApproval, profile.AllowUntrustedHW,
		nullIfEmpty(profile.ChallengeHash), nullIfEmpty(profile.ChallengeHint), profile.ApprovalDelaySec,
		profile.MaxUses, profile.Uses, nullIfZeroTime(profile.ExpiresAt), nullIfEmptyBytes(profile.DefaultLabelsJSON),
		nullIfZeroTime(profile.CreatedAt), nullIfEmpty(profile.CreatedBy), profile.Disabled, profile.CertValidityDays)
	return err
}

func (s *Store) ListEnrollmentProfiles() ([]store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
		FROM enrollment_profiles
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.EnrollmentProfile{}
	for rows.Next() {
		profile, err := scanEnrollmentProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetEnrollmentProfile(profileID string) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		SELECT
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
		FROM enrollment_profiles
		WHERE profile_id = $1::uuid
	`, profileID)
	profile, err := scanEnrollmentProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	return profile, nil
}

func (s *Store) UpdateEnrollmentProfile(profileID string, update store.EnrollmentProfileUpdate) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		UPDATE enrollment_profiles
		SET
			name = $2,
			require_approval = $3,
			allow_untrusted_hw = $4,
			challenge_hash = NULLIF($5, ''),
			challenge_hint = NULLIF($6, ''),
			approval_delay_seconds = $7,
			max_uses = $8,
			default_labels = $9,
			cert_validity_days = $10
		WHERE profile_id = $1::uuid
		RETURNING
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
	`, profileID, update.Name, update.RequireApproval, update.AllowUntrustedHW, update.ChallengeHash, update.ChallengeHint, update.ApprovalDelaySec, update.MaxUses, nullIfEmptyBytes(update.DefaultLabelsJSON), update.CertValidityDays)
	profile, err := scanEnrollmentProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	return profile, nil
}

func (s *Store) SetEnrollmentProfileDisabled(profileID string, disabled bool) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		UPDATE enrollment_profiles
		SET disabled = $2
		WHERE profile_id = $1::uuid
		RETURNING
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
	`, profileID, disabled)
	profile, err := scanEnrollmentProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	return profile, nil
}

func (s *Store) RotateEnrollmentProfileToken(profileID, tokenHash string, previousTokenValidUntil time.Time) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if tokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("token_hash required")
	}
	prevUntil := nullIfZeroTime(previousTokenValidUntil)
	row := s.pool.QueryRow(ctx, `
		UPDATE enrollment_profiles
		SET
			previous_token_hash = CASE WHEN $3 IS NULL THEN NULL ELSE token_hash END,
			previous_token_expires_at = $3,
			token_hash = $2,
			token_rotated_at = now()
		WHERE profile_id = $1::uuid
		RETURNING
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
	`, profileID, tokenHash, prevUntil)
	profile, err := scanEnrollmentProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileNotFound
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	return profile, nil
}

func (s *Store) GetEnrollmentProfileByTokenHash(profileTokenHash string) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if profileTokenHash == "" {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	row := s.pool.QueryRow(ctx, `
		SELECT
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
		FROM enrollment_profiles
		WHERE token_hash = $1
			OR previous_token_hash = $1
	`, profileTokenHash)
	profile, err := scanEnrollmentProfile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	now := time.Now().UTC()
	if profile.Disabled || (!profile.ExpiresAt.IsZero() && now.After(profile.ExpiresAt)) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.PreviousTokenHash == profileTokenHash && (profile.PreviousTokenExpiresAt.IsZero() || now.After(profile.PreviousTokenExpiresAt)) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.MaxUses > 0 && profile.Uses >= profile.MaxUses {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileExhausted
	}
	return profile, nil
}

func (s *Store) CreatePendingEnrollmentForProfileToken(profileTokenHash string, pending store.PendingEnrollment) (store.EnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if profileTokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("profile_token_hash required")
	}
	if pending.RequestID == "" {
		return store.EnrollmentProfile{}, errors.New("request_id required")
	}
	if pending.ClaimTokenHash == "" {
		return store.EnrollmentProfile{}, errors.New("claim_token_hash required")
	}
	if pending.CSR == "" {
		return store.EnrollmentProfile{}, errors.New("csr required")
	}
	if pending.ExpiresAt.IsZero() {
		return store.EnrollmentProfile{}, errors.New("expires_at required")
	}
	if pending.Status == "" {
		pending.Status = "pending"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	defer tx.Rollback(ctx)

	profileRow := tx.QueryRow(ctx, `
		SELECT
			profile_id::text,
			name,
			token_hash,
			COALESCE(previous_token_hash, ''),
			previous_token_expires_at,
			token_rotated_at,
			require_approval,
			allow_untrusted_hw,
			COALESCE(challenge_hash, ''),
			COALESCE(challenge_hint, ''),
			approval_delay_seconds,
			max_uses,
			uses,
			expires_at,
			created_at,
			created_by,
			disabled,
			default_labels,
			cert_validity_days
		FROM enrollment_profiles
		WHERE token_hash = $1
			OR previous_token_hash = $1
		FOR UPDATE
	`, profileTokenHash)
	profile, err := scanEnrollmentProfile(profileRow)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	now := time.Now().UTC()
	if profile.Disabled || (!profile.ExpiresAt.IsZero() && now.After(profile.ExpiresAt)) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.PreviousTokenHash == profileTokenHash && (profile.PreviousTokenExpiresAt.IsZero() || now.After(profile.PreviousTokenExpiresAt)) {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileInvalid
	}
	if profile.MaxUses > 0 && profile.Uses >= profile.MaxUses {
		return store.EnrollmentProfile{}, store.ErrEnrollmentProfileExhausted
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO pending_enrollments (
			request_id,
			profile_id,
			status,
			csr,
			claim_token_hash,
			capabilities,
			metadata,
			source_ip,
			user_agent,
			agent_version,
			hardware_id,
			denied_reason,
			expires_at,
			approval_available_at,
			approved_at,
			approved_by_user_id,
			denied_at,
			denied_by_user_id,
			issued_at,
			issued_device_id,
			created_at
		)
		VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			$13,
			$14,
			$15,
			$16::uuid,
			$17,
			$18::uuid,
			$19,
			$20::uuid,
			COALESCE($21, now())
		)
	`, pending.RequestID, profile.ProfileID, pending.Status, pending.CSR, pending.ClaimTokenHash,
		nullIfEmptyBytes(pending.CapabilitiesJSON), nullIfEmptyBytes(pending.MetadataJSON),
		nullIfEmpty(pending.SourceIP), nullIfEmpty(pending.UserAgent), nullIfEmpty(pending.AgentVersion),
		nullIfEmpty(pending.HardwareID), nullIfEmpty(pending.DeniedReason),
		pending.ExpiresAt, nullIfZeroTime(pending.ApprovalAvailableAt), nullIfZeroTime(pending.ApprovedAt), nullIfEmpty(pending.ApprovedByUserID),
		nullIfZeroTime(pending.DeniedAt), nullIfEmpty(pending.DeniedByUserID),
		nullIfZeroTime(pending.IssuedAt), nullIfEmpty(pending.IssuedDeviceID),
		nullIfZeroTime(pending.CreatedAt))
	if err != nil {
		return store.EnrollmentProfile{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE enrollment_profiles
		SET uses = uses + 1
		WHERE profile_id = $1::uuid
	`, profile.ProfileID); err != nil {
		return store.EnrollmentProfile{}, err
	}
	profile.Uses += 1

	if err := tx.Commit(ctx); err != nil {
		return store.EnrollmentProfile{}, err
	}
	return profile, nil
}

func (s *Store) ListPendingEnrollments(filter store.PendingEnrollmentFilter) ([]store.PendingEnrollment, error) {
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
		SELECT
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
		FROM pending_enrollments
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, filter.Status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]store.PendingEnrollment, 0, limit)
	for rows.Next() {
		var row store.PendingEnrollment
		if err := rows.Scan(
			&row.RequestID,
			&row.ProfileID,
			&row.Status,
			&row.CSR,
			&row.ClaimTokenHash,
			&row.CapabilitiesJSON,
			&row.MetadataJSON,
			&row.SourceIP,
			&row.UserAgent,
			&row.AgentVersion,
			&row.HardwareID,
			&row.DeniedReason,
			&row.ExpiresAt,
			&row.ApprovalAvailableAt,
			&row.ApprovedAt,
			&row.ApprovedByUserID,
			&row.DeniedAt,
			&row.DeniedByUserID,
			&row.IssuedAt,
			&row.IssuedDeviceID,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ApprovePendingEnrollment(requestID, approvedByUserID string, at time.Time) (store.PendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" {
		return store.PendingEnrollment{}, errors.New("request_id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE pending_enrollments
		SET
			status = 'approved',
			approved_at = $2,
			approved_by_user_id = NULLIF($3, '')::uuid
		WHERE request_id = $1::uuid
		  AND status = 'pending'
		  AND COALESCE(approval_available_at, created_at) <= $2
		  AND expires_at > now()
		RETURNING
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
	`, requestID, at, approvedByUserID)
	pending, err := scanPendingEnrollment(row)
	if err == nil {
		return pending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, err
	}
	current, ok, err := s.pendingEnrollmentByID(ctx, requestID)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if current.Status == "pending" && !current.ApprovalAvailableAt.IsZero() && current.ApprovalAvailableAt.After(at) {
		return current, store.ErrPendingEnrollmentThrottled
	}
	return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
}

func (s *Store) DenyPendingEnrollment(requestID, reason, deniedByUserID string, at time.Time) (store.PendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" {
		return store.PendingEnrollment{}, errors.New("request_id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE pending_enrollments
		SET
			status = 'denied',
			denied_reason = NULLIF($2, ''),
			denied_at = $3,
			denied_by_user_id = NULLIF($4, '')::uuid
		WHERE request_id = $1::uuid
		  AND status IN ('pending', 'approved')
		RETURNING
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
	`, requestID, reason, at, deniedByUserID)
	pending, err := scanPendingEnrollment(row)
	if err == nil {
		return pending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, err
	}
	_, ok, err := s.pendingEnrollmentByID(ctx, requestID)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
}

func (s *Store) ConflictPendingEnrollment(requestID, reason string, at time.Time) (store.PendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" {
		return store.PendingEnrollment{}, errors.New("request_id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE pending_enrollments
		SET
			status = 'conflict',
			denied_reason = NULLIF($2, ''),
			denied_at = $3
		WHERE request_id = $1::uuid
		  AND status IN ('pending', 'approved')
		RETURNING
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
	`, requestID, reason, at)
	pending, err := scanPendingEnrollment(row)
	if err == nil {
		return pending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, err
	}
	_, ok, err := s.pendingEnrollmentByID(ctx, requestID)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
}

func (s *Store) ResetPendingEnrollment(requestID string, expiresAt time.Time) (store.PendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" {
		return store.PendingEnrollment{}, errors.New("request_id required")
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE pending_enrollments
		SET
			status = 'pending',
			expires_at = $2,
			denied_reason = NULL,
			approved_at = NULL,
			approved_by_user_id = NULL,
			denied_at = NULL,
			denied_by_user_id = NULL,
			issued_at = NULL,
			issued_device_id = NULL
		WHERE request_id = $1::uuid
		  AND status IN ('denied', 'conflict', 'expired')
		RETURNING
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
	`, requestID, expiresAt)
	pending, err := scanPendingEnrollment(row)
	if err == nil {
		return pending, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, err
	}
	_, ok, err := s.pendingEnrollmentByID(ctx, requestID)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if !ok {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
}

func (s *Store) ExpirePendingEnrollments(before time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE pending_enrollments
		SET status = 'expired'
		WHERE status IN ('pending', 'approved')
		  AND expires_at <= $1
	`, before)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CountActivePendingEnrollments(profileID, sourceIP string, now time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pending_enrollments
		WHERE status IN ('pending', 'approved')
		  AND expires_at > $1
		  AND ($2 = '' OR profile_id = $2::uuid)
		  AND ($3 = '' OR source_ip = $3)
	`, now, profileID, sourceIP).Scan(&count)
	return count, err
}

func (s *Store) GetPendingEnrollmentForClaim(requestID, claimTokenHash string) (store.PendingEnrollment, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" || claimTokenHash == "" {
		return store.PendingEnrollment{}, false, nil
	}
	row := s.pool.QueryRow(ctx, `
		SELECT
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
		FROM pending_enrollments
		WHERE request_id = $1::uuid AND claim_token_hash = $2
	`, requestID, claimTokenHash)
	pending, err := scanPendingEnrollment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, false, nil
	}
	if err != nil {
		return store.PendingEnrollment{}, false, err
	}
	if (pending.Status == "pending" || pending.Status == "approved") && time.Now().UTC().After(pending.ExpiresAt) {
		if _, err := s.pool.Exec(ctx, `
			UPDATE pending_enrollments
			SET status = 'expired'
			WHERE request_id = $1::uuid
			  AND status IN ('pending', 'approved')
			  AND expires_at <= now()
		`, requestID); err != nil {
			return store.PendingEnrollment{}, false, err
		}
		pending.Status = "expired"
	}
	return pending, true, nil
}

func (s *Store) MarkPendingEnrollmentIssued(requestID, claimTokenHash string, device store.Device, maxDevices int, issuedAt time.Time) (store.PendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if requestID == "" {
		return store.PendingEnrollment{}, errors.New("request_id required")
	}
	if claimTokenHash == "" {
		return store.PendingEnrollment{}, errors.New("claim_token_hash required")
	}
	if device.DeviceID == "" {
		return store.PendingEnrollment{}, errors.New("device_id required")
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
		FROM pending_enrollments
		WHERE request_id = $1::uuid
		FOR UPDATE
	`, requestID)
	pending, err := scanPendingEnrollment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentNotFound
	}
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if pending.ClaimTokenHash != claimTokenHash {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentToken
	}
	if (pending.Status == "pending" || pending.Status == "approved") && time.Now().UTC().After(pending.ExpiresAt) {
		if _, err := tx.Exec(ctx, `
			UPDATE pending_enrollments
			SET status = 'expired'
			WHERE request_id = $1::uuid
		`, requestID); err != nil {
			return store.PendingEnrollment{}, err
		}
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}
	if pending.Status != "approved" {
		return store.PendingEnrollment{}, store.ErrPendingEnrollmentState
	}

	if maxDevices > 0 {
		if _, err := tx.Exec(ctx, `LOCK TABLE devices IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return store.PendingEnrollment{}, err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&count); err != nil {
			return store.PendingEnrollment{}, err
		}
		if count >= maxDevices {
			return store.PendingEnrollment{}, store.ErrDeviceLimitExceeded
		}
	}

	cert := nullIfEmpty(device.CertFingerprint)
	labels := nullIfEmptyBytes(device.LabelsJSON)
	metadata := nullIfEmptyBytes(device.MetadataJSON)
	if _, err := tx.Exec(ctx, `
		INSERT INTO devices (device_id, cert_fingerprint, cert_serial, status, last_seen, labels, metadata)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
	`, device.DeviceID, cert, device.CertSerial, device.Status, device.LastSeen, labels, metadata); err != nil {
		return store.PendingEnrollment{}, err
	}

	row = tx.QueryRow(ctx, `
		UPDATE pending_enrollments
		SET
			status = 'issued',
			issued_at = $2,
			issued_device_id = $3::uuid
		WHERE request_id = $1::uuid
		RETURNING
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
	`, requestID, issuedAt, device.DeviceID)
	pending, err = scanPendingEnrollment(row)
	if err != nil {
		return store.PendingEnrollment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.PendingEnrollment{}, err
	}
	return pending, nil
}

func scanPendingEnrollment(row pgx.Row) (store.PendingEnrollment, error) {
	var pending store.PendingEnrollment
	err := row.Scan(
		&pending.RequestID,
		&pending.ProfileID,
		&pending.Status,
		&pending.CSR,
		&pending.ClaimTokenHash,
		&pending.CapabilitiesJSON,
		&pending.MetadataJSON,
		&pending.SourceIP,
		&pending.UserAgent,
		&pending.AgentVersion,
		&pending.HardwareID,
		&pending.DeniedReason,
		&pending.ExpiresAt,
		&pending.ApprovalAvailableAt,
		&pending.ApprovedAt,
		&pending.ApprovedByUserID,
		&pending.DeniedAt,
		&pending.DeniedByUserID,
		&pending.IssuedAt,
		&pending.IssuedDeviceID,
		&pending.CreatedAt,
	)
	return pending, err
}

func scanEnrollmentProfile(row interface {
	Scan(dest ...any) error
}) (store.EnrollmentProfile, error) {
	var profile store.EnrollmentProfile
	var previousTokenHash sql.NullString
	var previousTokenExpiresAt sql.NullTime
	var tokenRotatedAt sql.NullTime
	var expiresAt sql.NullTime
	var createdBy sql.NullString
	err := row.Scan(
		&profile.ProfileID,
		&profile.Name,
		&profile.TokenHash,
		&previousTokenHash,
		&previousTokenExpiresAt,
		&tokenRotatedAt,
		&profile.RequireApproval,
		&profile.AllowUntrustedHW,
		&profile.ChallengeHash,
		&profile.ChallengeHint,
		&profile.ApprovalDelaySec,
		&profile.MaxUses,
		&profile.Uses,
		&expiresAt,
		&profile.CreatedAt,
		&createdBy,
		&profile.Disabled,
		&profile.DefaultLabelsJSON,
		&profile.CertValidityDays,
	)
	if err != nil {
		return store.EnrollmentProfile{}, err
	}
	if previousTokenHash.Valid {
		profile.PreviousTokenHash = previousTokenHash.String
	}
	if previousTokenExpiresAt.Valid {
		profile.PreviousTokenExpiresAt = previousTokenExpiresAt.Time
	}
	if tokenRotatedAt.Valid {
		profile.TokenRotatedAt = tokenRotatedAt.Time
	}
	if expiresAt.Valid {
		profile.ExpiresAt = expiresAt.Time
	}
	if createdBy.Valid {
		profile.CreatedBy = createdBy.String
	}
	return profile, nil
}

func (s *Store) pendingEnrollmentByID(ctx context.Context, requestID string) (store.PendingEnrollment, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			request_id::text,
			profile_id::text,
			status,
			csr,
			COALESCE(claim_token_hash, ''),
			capabilities,
			metadata,
			COALESCE(source_ip, ''),
			COALESCE(user_agent, ''),
			COALESCE(agent_version, ''),
			COALESCE(hardware_id, ''),
			COALESCE(denied_reason, ''),
			expires_at,
			COALESCE(approval_available_at, created_at),
			COALESCE(approved_at, 'epoch'::timestamptz),
			COALESCE(approved_by_user_id::text, ''),
			COALESCE(denied_at, 'epoch'::timestamptz),
			COALESCE(denied_by_user_id::text, ''),
			COALESCE(issued_at, 'epoch'::timestamptz),
			COALESCE(issued_device_id::text, ''),
			created_at
		FROM pending_enrollments
		WHERE request_id = $1::uuid
	`, requestID)
	pending, err := scanPendingEnrollment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.PendingEnrollment{}, false, nil
	}
	if err != nil {
		return store.PendingEnrollment{}, false, err
	}
	return pending, true, nil
}

func (s *Store) CreateDevice(device store.Device) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cert := nullIfEmpty(device.CertFingerprint)
	labels := nullIfEmptyBytes(device.LabelsJSON)
	metadata := nullIfEmptyBytes(device.MetadataJSON)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (device_id, cert_fingerprint, cert_serial, status, last_seen, labels, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, device.DeviceID, cert, device.CertSerial, device.Status, device.LastSeen, labels, metadata)
	return err
}

func (s *Store) GetDevice(deviceID string) (store.Device, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.Device
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, COALESCE(cert_fingerprint, ''), COALESCE(cert_serial, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE device_id = $1
	`, deviceID).Scan(&d.DeviceID, &d.CertFingerprint, &d.CertSerial, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
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
		SELECT device_id, COALESCE(cert_fingerprint, ''), COALESCE(cert_serial, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE cert_fingerprint = $1
	`, fingerprint).Scan(&d.DeviceID, &d.CertFingerprint, &d.CertSerial, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
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
		SELECT device_id, COALESCE(cert_fingerprint, ''), COALESCE(cert_serial, ''), status, last_seen, labels, metadata
		FROM devices
		WHERE metadata #>> '{hwops,identity,hardwareId}' = $1
	`, hardwareID).Scan(&d.DeviceID, &d.CertFingerprint, &d.CertSerial, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON)
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
		SELECT device_id, COALESCE(cert_fingerprint, ''), COALESCE(cert_serial, ''), status, last_seen, labels, metadata
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
		if err := rows.Scan(&d.DeviceID, &d.CertFingerprint, &d.CertSerial, &d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON); err != nil {
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

	// Capture cert serial before deletion so we can add it to the blocklist.
	var certSerial string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(cert_serial, '') FROM devices WHERE device_id = $1`, deviceID).Scan(&certSerial)
	if certSerial != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cert_revoked_serials (serial, device_id, revoked_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (serial) DO NOTHING
		`, certSerial, deviceID); err != nil {
			return err
		}
	}

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

func (s *Store) GetDesiredStateGroup(groupID string) (store.DesiredStateGroup, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d store.DesiredStateGroup
	err := s.pool.QueryRow(ctx, `
		SELECT group_id,
		       COALESCE(artifact_id::text, ''),
		       COALESCE(desired_version, ''),
		       COALESCE(desired_config_rev, ''),
		       policy,
		       COALESCE(components, '{}'::jsonb),
		       COALESCE(checkin_interval_sec, 0),
		       updated_at
		FROM desired_state_group
		WHERE group_id = $1
	`, groupID).Scan(&d.GroupID, &d.ArtifactID, &d.DesiredVersion, &d.DesiredConfigRev, &d.PolicyJSON, &d.ComponentsJSON, &d.CheckinInterval, &d.UpdatedAt)
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
	verificationStatus := artifact.VerificationStatus
	if verificationStatus == "" {
		verificationStatus = "legacy"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifacts (
			artifact_id, name, version, type, status, object_key, sha256, signature,
			signature_type, signature_key_id, verification_status, verification_error, verified_at,
			size_bytes, metadata, created_at, deprecated_at, delete_after
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, artifact.ArtifactID, artifact.Name, artifact.Version, atype, status, artifact.ObjectKey, artifact.SHA256, nullIfEmpty(artifact.Signature),
		nullIfEmpty(artifact.SignatureType), nullIfEmpty(artifact.SignatureKeyID), verificationStatus, nullIfEmpty(artifact.VerificationError), nullIfZeroTime(artifact.VerifiedAt),
		artifact.SizeBytes, nullIfEmptyBytes(artifact.MetadataJSON), artifact.CreatedAt, nullIfZeroTime(artifact.DeprecatedAt), nullIfZeroTime(artifact.DeleteAfter))
	return err
}

func (s *Store) GetArtifact(artifactID string) (store.Artifact, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a store.Artifact
	err := s.pool.QueryRow(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''),
		       COALESCE(signature_type, ''), COALESCE(signature_key_id, ''), COALESCE(verification_status, 'legacy'), COALESCE(verification_error, ''),
		       COALESCE(verified_at, '0001-01-01T00:00:00Z'::timestamptz),
		       size_bytes, COALESCE(metadata, '{}'::jsonb), created_at,
		       COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz),
		       COALESCE(sbom_object_key, '')
		FROM artifacts
		WHERE artifact_id = $1
	`, artifactID).Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SignatureType, &a.SignatureKeyID, &a.VerificationStatus, &a.VerificationError, &a.VerifiedAt, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter, &a.SBOMObjectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Artifact{}, false, nil
	}
	if err != nil {
		return store.Artifact{}, false, err
	}
	return a, true, nil
}

func (s *Store) FindArtifactByNameTypeVersion(name, artifactType, version string) (store.Artifact, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var a store.Artifact
	err := s.pool.QueryRow(ctx, `
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''),
		       COALESCE(signature_type, ''), COALESCE(signature_key_id, ''), COALESCE(verification_status, 'legacy'), COALESCE(verification_error, ''),
		       COALESCE(verified_at, '0001-01-01T00:00:00Z'::timestamptz),
		       size_bytes, COALESCE(metadata, '{}'::jsonb), created_at,
		       COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz),
		       COALESCE(sbom_object_key, '')
		FROM artifacts
		WHERE name = $1 AND type = $2 AND version = $3 AND status = 'active'
		ORDER BY created_at DESC, artifact_id DESC
		LIMIT 1
	`, name, artifactType, version).Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SignatureType, &a.SignatureKeyID, &a.VerificationStatus, &a.VerificationError, &a.VerifiedAt, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter, &a.SBOMObjectKey)
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
		SELECT artifact_id, name, version, COALESCE(type, 'app_bundle'), COALESCE(status, 'active'), object_key, sha256, COALESCE(signature, ''),
		       COALESCE(signature_type, ''), COALESCE(signature_key_id, ''), COALESCE(verification_status, 'legacy'), COALESCE(verification_error, ''),
		       COALESCE(verified_at, '0001-01-01T00:00:00Z'::timestamptz),
		       size_bytes, COALESCE(metadata, '{}'::jsonb), created_at,
		       COALESCE(deprecated_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(delete_after, '0001-01-01T00:00:00Z'::timestamptz),
		       COALESCE(sbom_object_key, '')
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
		if err := rows.Scan(&a.ArtifactID, &a.Name, &a.Version, &a.Type, &a.Status, &a.ObjectKey, &a.SHA256, &a.Signature, &a.SignatureType, &a.SignatureKeyID, &a.VerificationStatus, &a.VerificationError, &a.VerifiedAt, &a.SizeBytes, &a.MetadataJSON, &a.CreatedAt, &a.DeprecatedAt, &a.DeleteAfter, &a.SBOMObjectKey); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *Store) CountArtifactsByVerificationStatus(status string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM artifacts
		WHERE COALESCE(verification_status, 'legacy') = $1
	`, status).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SetArtifactSBOMObjectKey(artifactID, sbomObjectKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE artifacts SET sbom_object_key = $2 WHERE artifact_id = $1`,
		artifactID, sbomObjectKey,
	)
	return err
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

func (s *Store) GetReleaseAutoUpdateSettings() (store.ReleaseAutoUpdateSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var settings store.ReleaseAutoUpdateSettings
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, allow_unsigned, updated_at, COALESCE(updated_by_user_id, '')
		FROM release_auto_update_settings
		WHERE settings_id = 1
	`).Scan(&settings.Enabled, &settings.AllowUnsigned, &settings.UpdatedAt, &settings.UpdatedByUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		now := time.Now().UTC()
		return store.ReleaseAutoUpdateSettings{
			Enabled:       false,
			AllowUnsigned: false,
			UpdatedAt:     now,
		}, nil
	}
	if err != nil {
		return store.ReleaseAutoUpdateSettings{}, err
	}
	return settings, nil
}

func (s *Store) SetReleaseAutoUpdateSettings(settings store.ReleaseAutoUpdateSettings) (store.ReleaseAutoUpdateSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out store.ReleaseAutoUpdateSettings
	err := s.pool.QueryRow(ctx, `
		INSERT INTO release_auto_update_settings (settings_id, enabled, allow_unsigned, updated_at, updated_by_user_id)
		VALUES (1, $1, $2, now(), NULLIF($3, ''))
		ON CONFLICT (settings_id)
		DO UPDATE SET
			enabled = EXCLUDED.enabled,
			allow_unsigned = EXCLUDED.allow_unsigned,
			updated_at = now(),
			updated_by_user_id = EXCLUDED.updated_by_user_id
		RETURNING enabled, allow_unsigned, updated_at, COALESCE(updated_by_user_id, '')
	`, settings.Enabled, settings.AllowUnsigned, settings.UpdatedByUserID).Scan(
		&out.Enabled,
		&out.AllowUnsigned,
		&out.UpdatedAt,
		&out.UpdatedByUserID,
	)
	if err != nil {
		return store.ReleaseAutoUpdateSettings{}, err
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

func (s *Store) ListTrustedSigningKeys(includeRetired bool) ([]store.TrustedSigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT key_id, display_name, algorithm, public_key_pem, state,
		       created_at, COALESCE(retired_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(notes, '')
		FROM trusted_signing_keys
	`
	args := []any{}
	if !includeRetired {
		query += ` WHERE state <> 'retired'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.TrustedSigningKey{}
	for rows.Next() {
		var key store.TrustedSigningKey
		if err := rows.Scan(&key.KeyID, &key.DisplayName, &key.Algorithm, &key.PublicKeyPEM, &key.State, &key.CreatedAt, &key.RetiredAt, &key.Notes); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetTrustedSigningKey(keyID string) (store.TrustedSigningKey, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var key store.TrustedSigningKey
	err := s.pool.QueryRow(ctx, `
		SELECT key_id, display_name, algorithm, public_key_pem, state,
		       created_at, COALESCE(retired_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(notes, '')
		FROM trusted_signing_keys
		WHERE key_id = $1
	`, keyID).Scan(&key.KeyID, &key.DisplayName, &key.Algorithm, &key.PublicKeyPEM, &key.State, &key.CreatedAt, &key.RetiredAt, &key.Notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.TrustedSigningKey{}, false, nil
	}
	if err != nil {
		return store.TrustedSigningKey{}, false, err
	}
	return key, true, nil
}

func (s *Store) UpsertTrustedSigningKey(key store.TrustedSigningKey) (store.TrustedSigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out store.TrustedSigningKey
	err := s.pool.QueryRow(ctx, `
		INSERT INTO trusted_signing_keys (key_id, display_name, algorithm, public_key_pem, state, created_at, retired_at, notes)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, now()), $7, NULLIF($8, ''))
		ON CONFLICT (key_id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			algorithm = EXCLUDED.algorithm,
			public_key_pem = EXCLUDED.public_key_pem,
			state = EXCLUDED.state,
			retired_at = EXCLUDED.retired_at,
			notes = EXCLUDED.notes
		RETURNING key_id, display_name, algorithm, public_key_pem, state,
		          created_at, COALESCE(retired_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(notes, '')
	`, key.KeyID, key.DisplayName, key.Algorithm, key.PublicKeyPEM, key.State, nullIfZeroTime(key.CreatedAt), nullIfZeroTime(key.RetiredAt), key.Notes).Scan(
		&out.KeyID, &out.DisplayName, &out.Algorithm, &out.PublicKeyPEM, &out.State, &out.CreatedAt, &out.RetiredAt, &out.Notes,
	)
	if err != nil {
		return store.TrustedSigningKey{}, err
	}
	return out, nil
}

func (s *Store) RetireTrustedSigningKey(keyID string, retiredAt time.Time) (store.TrustedSigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out store.TrustedSigningKey
	err := s.pool.QueryRow(ctx, `
		UPDATE trusted_signing_keys
		SET state = 'retired',
		    retired_at = $2
		WHERE key_id = $1
		RETURNING key_id, display_name, algorithm, public_key_pem, state,
		          created_at, COALESCE(retired_at, '0001-01-01T00:00:00Z'::timestamptz), COALESCE(notes, '')
	`, keyID, retiredAt).Scan(
		&out.KeyID, &out.DisplayName, &out.Algorithm, &out.PublicKeyPEM, &out.State, &out.CreatedAt, &out.RetiredAt, &out.Notes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.TrustedSigningKey{}, errors.New("trusted signing key not found")
	}
	if err != nil {
		return store.TrustedSigningKey{}, err
	}
	return out, nil
}

func (s *Store) GetArtifactTrustPolicy() (store.ArtifactTrustPolicy, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var policy store.ArtifactTrustPolicy
	var provJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT verification_mode,
		       COALESCE(allowed_signing_key_ids, '[]'::jsonb),
		       COALESCE(allowed_signature_types, '[]'::jsonb),
		       updated_at,
		       COALESCE(updated_by_user_id, ''),
		       provenance_policy
		FROM artifact_trust_policy
		WHERE policy_id = 1
	`).Scan(&policy.VerificationMode, &policy.AllowedSigningKeyIDsJSON, &policy.AllowedSignatureTypesJSON, &policy.UpdatedAt, &policy.UpdatedByUserID, &provJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ArtifactTrustPolicy{}, false, nil
	}
	if err != nil {
		return store.ArtifactTrustPolicy{}, false, err
	}
	policy.ProvenancePolicyJSON = provJSON
	return policy, true, nil
}

func (s *Store) SetArtifactTrustPolicy(policy store.ArtifactTrustPolicy) (store.ArtifactTrustPolicy, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out store.ArtifactTrustPolicy
	var provJSON []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO artifact_trust_policy (
			policy_id, verification_mode, allowed_signing_key_ids, allowed_signature_types, updated_at, updated_by_user_id, provenance_policy
		)
		VALUES (1, $1, COALESCE($2, '[]'::jsonb), COALESCE($3, '[]'::jsonb), now(), NULLIF($4, ''), $5)
		ON CONFLICT (policy_id) DO UPDATE SET
			verification_mode = EXCLUDED.verification_mode,
			allowed_signing_key_ids = EXCLUDED.allowed_signing_key_ids,
			allowed_signature_types = EXCLUDED.allowed_signature_types,
			updated_at = now(),
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			provenance_policy = EXCLUDED.provenance_policy
		RETURNING verification_mode,
		          COALESCE(allowed_signing_key_ids, '[]'::jsonb),
		          COALESCE(allowed_signature_types, '[]'::jsonb),
		          updated_at,
		          COALESCE(updated_by_user_id, ''),
		          provenance_policy
	`, policy.VerificationMode, nullIfEmptyBytes(policy.AllowedSigningKeyIDsJSON), nullIfEmptyBytes(policy.AllowedSignatureTypesJSON), policy.UpdatedByUserID, nullIfEmptyBytes(policy.ProvenancePolicyJSON)).Scan(
		&out.VerificationMode, &out.AllowedSigningKeyIDsJSON, &out.AllowedSignatureTypesJSON, &out.UpdatedAt, &out.UpdatedByUserID, &provJSON,
	)
	if err != nil {
		return store.ArtifactTrustPolicy{}, err
	}
	out.ProvenancePolicyJSON = provJSON
	return out, nil
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
			target_type, target_id, status, error, before, after, metadata,
			phi_touched, minimum_necessary
		) VALUES (
			COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, $18,
			$19, $20
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
		event.PhiTouched,
		event.MinimumNecessary,
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
		       COALESCE(before, '{}'::jsonb), COALESCE(after, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb),
		       COALESCE(phi_touched, false), COALESCE(minimum_necessary, '')
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
		  AND ($10::boolean IS NULL OR phi_touched = $10)
		ORDER BY occurred_at DESC
		LIMIT $11 OFFSET $12
	`, filter.Action, filter.ActorType, filter.ActorID, filter.ActorEmail, filter.TargetType, filter.TargetID, filter.Status,
		nullIfZeroTime(filter.Since), nullIfZeroTime(filter.Until), filter.PhiTouched, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []store.AuditEvent{}
	for rows.Next() {
		var ev store.AuditEvent
		if err := rows.Scan(&ev.EventID, &ev.OccurredAt, &ev.ActorType, &ev.ActorID, &ev.ActorEmail, &ev.ActorRolesJSON,
			&ev.AuthMethod, &ev.SourceIP, &ev.UserAgent, &ev.RequestID, &ev.Action, &ev.TargetType, &ev.TargetID,
			&ev.Status, &ev.Error, &ev.BeforeJSON, &ev.AfterJSON, &ev.MetadataJSON,
			&ev.PhiTouched, &ev.MinimumNecessary); err != nil {
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
	recoveryCodes := user.RecoveryCodesJSON
	if len(recoveryCodes) == 0 {
		recoveryCodes = []byte(`[]`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (
			user_id, email, display_name, password_hash, roles, recovery_codes, disabled, auth_provider, external_id,
			created_at, updated_at, last_login_at, recovery_codes_generated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, user.UserID, user.Email, nullIfEmpty(user.DisplayName), user.PasswordHash, nullIfEmptyBytes(user.RolesJSON),
		recoveryCodes, user.Disabled, user.AuthProvider, nullIfEmpty(user.ExternalID),
		user.CreatedAt, user.UpdatedAt, nullIfZeroTime(user.LastLoginAt), nullIfZeroTime(user.RecoveryCodesGeneratedAt))
	return err
}

func (s *Store) GetUser(userID string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       COALESCE(recovery_codes, '[]'::jsonb), disabled, COALESCE(auth_provider, 'local'),
		       COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz),
		       COALESCE(recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(totp_secret, ''), COALESCE(totp_enabled, false)
		FROM users
		WHERE user_id = $1
	`, userID).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.RecoveryCodesJSON, &u.Disabled,
		&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.RecoveryCodesGeneratedAt,
		&u.TOTPSecret, &u.TOTPEnabled)
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
		       COALESCE(recovery_codes, '[]'::jsonb), disabled, COALESCE(auth_provider, 'local'),
		       COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz),
		       COALESCE(recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(totp_secret, ''), COALESCE(totp_enabled, false)
		FROM users
		WHERE email = $1
	`, email).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.RecoveryCodesJSON, &u.Disabled,
		&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.RecoveryCodesGeneratedAt,
		&u.TOTPSecret, &u.TOTPEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	return u, true, nil
}

func (s *Store) GetUserByExternalID(provider, externalID string) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       COALESCE(recovery_codes, '[]'::jsonb), disabled, COALESCE(auth_provider, 'local'),
		       COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz),
		       COALESCE(recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(totp_secret, ''), COALESCE(totp_enabled, false)
		FROM users
		WHERE auth_provider = $1 AND external_id = $2 AND NOT disabled
		LIMIT 1
	`, provider, externalID).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.RecoveryCodesJSON, &u.Disabled,
		&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.RecoveryCodesGeneratedAt,
		&u.TOTPSecret, &u.TOTPEnabled)
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
		       COALESCE(recovery_codes, '[]'::jsonb), disabled, COALESCE(auth_provider, 'local'),
		       COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz),
		       COALESCE(recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(totp_secret, ''), COALESCE(totp_enabled, false)
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
		if err := rows.Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.RolesJSON, &u.RecoveryCodesJSON, &u.Disabled,
			&u.AuthProvider, &u.ExternalID, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt, &u.RecoveryCodesGeneratedAt,
			&u.TOTPSecret, &u.TOTPEnabled); err != nil {
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

func (s *Store) SetUserRecoveryCodes(userID string, recoveryCodesJSON []byte, generatedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if userID == "" {
		return errors.New("user_id required")
	}
	if len(recoveryCodesJSON) == 0 {
		recoveryCodesJSON = []byte(`[]`)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET recovery_codes = $2,
		    recovery_codes_generated_at = $3,
		    updated_at = now()
		WHERE user_id = $1
	`, userID, recoveryCodesJSON, nullIfZeroTime(generatedAt))
	return err
}

func (s *Store) SetUserTOTP(userID, encryptedSecret string, enabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET totp_secret = $2,
		    totp_enabled = $3,
		    updated_at = now()
		WHERE user_id = $1
	`, userID, nullIfEmpty(encryptedSecret), enabled)
	return err
}

func (s *Store) ConsumeUserRecoveryCode(email, recoveryCodeHash, passwordHash string, at time.Time) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if email == "" || recoveryCodeHash == "" || passwordHash == "" {
		return store.User{}, false, errors.New("email, recovery_code_hash, and password_hash required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.User{}, false, err
	}
	defer tx.Rollback(ctx)

	var user store.User
	err = tx.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), password_hash, COALESCE(roles, '[]'::jsonb),
		       COALESCE(recovery_codes, '[]'::jsonb), disabled, COALESCE(auth_provider, 'local'),
		       COALESCE(external_id, ''), created_at, updated_at,
		       COALESCE(last_login_at, '0001-01-01'::timestamptz),
		       COALESCE(recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(totp_secret, ''), COALESCE(totp_enabled, false)
		FROM users
		WHERE email = $1
		FOR UPDATE
	`, email).Scan(&user.UserID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.RolesJSON, &user.RecoveryCodesJSON,
		&user.Disabled, &user.AuthProvider, &user.ExternalID, &user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt, &user.RecoveryCodesGeneratedAt,
		&user.TOTPSecret, &user.TOTPEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	if user.Disabled {
		return store.User{}, false, nil
	}
	var hashes []string
	if len(user.RecoveryCodesJSON) > 0 {
		if err := json.Unmarshal(user.RecoveryCodesJSON, &hashes); err != nil {
			return store.User{}, false, err
		}
	}
	match := -1
	for i, hash := range hashes {
		if hash == recoveryCodeHash {
			match = i
			break
		}
	}
	if match < 0 {
		return store.User{}, false, nil
	}
	hashes = append(hashes[:match], hashes[match+1:]...)
	nextJSON, err := json.Marshal(hashes)
	if err != nil {
		return store.User{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
		    recovery_codes = $3,
		    updated_at = $4
		WHERE user_id = $1
	`, user.UserID, passwordHash, nextJSON, at); err != nil {
		return store.User{}, false, err
	}
	user.PasswordHash = passwordHash
	user.RecoveryCodesJSON = nextJSON
	user.UpdatedAt = at
	if err := tx.Commit(ctx); err != nil {
		return store.User{}, false, err
	}
	return user, true, nil
}

func (s *Store) CreatePasswordResetToken(token store.PasswordResetToken) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (
			token_id, user_id, token_hash, delivery_mode, reason, expires_at, created_at, issued_by_user_id, used_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, token.TokenID, token.UserID, token.TokenHash, nullIfEmpty(token.DeliveryMode), nullIfEmpty(token.Reason),
		token.ExpiresAt, token.CreatedAt, nullIfEmpty(token.IssuedByUserID), nullIfZeroTime(token.UsedAt))
	return err
}

func (s *Store) ConsumePasswordResetToken(email, tokenHash, passwordHash string, at time.Time) (store.User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if email == "" || tokenHash == "" || passwordHash == "" {
		return store.User{}, false, errors.New("email, token_hash, and password_hash required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.User{}, false, err
	}
	defer tx.Rollback(ctx)

	var tokenID string
	var user store.User
	err = tx.QueryRow(ctx, `
		SELECT prt.token_id::text, u.user_id, u.email, COALESCE(u.display_name, ''), u.password_hash,
		       COALESCE(u.roles, '[]'::jsonb), COALESCE(u.recovery_codes, '[]'::jsonb), u.disabled,
		       COALESCE(u.auth_provider, 'local'), COALESCE(u.external_id, ''), u.created_at, u.updated_at,
		       COALESCE(u.last_login_at, '0001-01-01'::timestamptz), COALESCE(u.recovery_codes_generated_at, '0001-01-01'::timestamptz),
		       COALESCE(u.totp_secret, ''), COALESCE(u.totp_enabled, false)
		FROM password_reset_tokens prt
		JOIN users u ON u.user_id = prt.user_id
		WHERE prt.token_hash = $1
		  AND prt.used_at IS NULL
		  AND prt.expires_at > $2
		  AND u.email = $3
		FOR UPDATE OF prt, u
	`, tokenHash, at, email).Scan(&tokenID, &user.UserID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.RolesJSON,
		&user.RecoveryCodesJSON, &user.Disabled, &user.AuthProvider, &user.ExternalID, &user.CreatedAt, &user.UpdatedAt,
		&user.LastLoginAt, &user.RecoveryCodesGeneratedAt, &user.TOTPSecret, &user.TOTPEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	if user.Disabled {
		return store.User{}, false, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = $2
		WHERE token_id = $1
	`, tokenID, at); err != nil {
		return store.User{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
		    updated_at = $3
		WHERE user_id = $1
	`, user.UserID, passwordHash, at); err != nil {
		return store.User{}, false, err
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = at
	if err := tx.Commit(ctx); err != nil {
		return store.User{}, false, err
	}
	return user, true, nil
}

func (s *Store) UpdatePasswordResetTokenEmailSent(tokenID string, sentAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE password_reset_tokens SET email_sent_at = $2 WHERE token_id = $1
	`, tokenID, sentAt)
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

func (s *Store) GetServiceToken(tokenID string) (store.ServiceToken, bool, error) {
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
		WHERE token_id = $1
	`, tokenID).Scan(&token.TokenID, &token.Name, &token.TokenHash, &token.ScopesJSON,
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

func (s *Store) RevokeDeviceCertSerial(serial, deviceID string, at time.Time) error {
	if serial == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cert_revoked_serials (serial, device_id, revoked_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (serial) DO NOTHING
	`, serial, deviceID, at.UTC())
	return err
}

func (s *Store) IsCertSerialRevoked(serial string) (bool, error) {
	if serial == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM cert_revoked_serials WHERE serial = $1)
	`, serial).Scan(&exists)
	return exists, err
}

// --- Attestations ------------------------------------------------------------

func (s *Store) CreateAttestation(rec store.AttestationRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifact_attestations (
			artifact_id, predicate_type, payload_json,
			signature, signature_type, signature_key_id,
			builder_id, builder_issuer, verified_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		rec.ArtifactID, rec.PredicateType, rec.PayloadJSON,
		nullIfEmpty(rec.Signature), nullIfEmpty(rec.SignatureType), nullIfEmpty(rec.SignatureKeyID),
		nullIfEmpty(rec.BuilderID), nullIfEmpty(rec.BuilderIssuer), nullIfZeroTime(rec.VerifiedAt),
	)
	return err
}

func (s *Store) GetAttestation(attestationID string) (store.AttestationRecord, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out store.AttestationRecord
	var verifiedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT attestation_id, artifact_id, predicate_type, payload_json,
		       COALESCE(signature, ''), COALESCE(signature_type, ''), COALESCE(signature_key_id, ''),
		       COALESCE(builder_id, ''), COALESCE(builder_issuer, ''),
		       verified_at, created_at
		FROM artifact_attestations
		WHERE attestation_id = $1
	`, attestationID).Scan(
		&out.AttestationID, &out.ArtifactID, &out.PredicateType, &out.PayloadJSON,
		&out.Signature, &out.SignatureType, &out.SignatureKeyID,
		&out.BuilderID, &out.BuilderIssuer,
		&verifiedAt, &out.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AttestationRecord{}, false, nil
	}
	if err != nil {
		return store.AttestationRecord{}, false, err
	}
	if verifiedAt != nil {
		out.VerifiedAt = *verifiedAt
	}
	return out, true, nil
}

func (s *Store) ListAttestations(artifactID string) ([]store.AttestationRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT attestation_id, artifact_id, predicate_type, payload_json,
		       COALESCE(signature, ''), COALESCE(signature_type, ''), COALESCE(signature_key_id, ''),
		       COALESCE(builder_id, ''), COALESCE(builder_issuer, ''),
		       verified_at, created_at
		FROM artifact_attestations
		WHERE artifact_id = $1
		ORDER BY created_at ASC
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.AttestationRecord
	for rows.Next() {
		var rec store.AttestationRecord
		var verifiedAt *time.Time
		if err := rows.Scan(
			&rec.AttestationID, &rec.ArtifactID, &rec.PredicateType, &rec.PayloadJSON,
			&rec.Signature, &rec.SignatureType, &rec.SignatureKeyID,
			&rec.BuilderID, &rec.BuilderIssuer,
			&verifiedAt, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		if verifiedAt != nil {
			rec.VerifiedAt = *verifiedAt
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAttestationsForArtifact(artifactID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		DELETE FROM artifact_attestations WHERE artifact_id = $1
	`, artifactID)
	return err
}

// --- Vulnerability scans (artifact) ------------------------------------------

func (s *Store) CreateArtifactVulnScan(scan store.ArtifactVulnerabilityScan) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifact_vulnerability_scans (
			artifact_id, scanner_type, scanner_version, scan_status,
			findings_json, severity_counts, error_message, scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING scan_id, created_at
	`,
		scan.ArtifactID,
		scan.ScannerType,
		nullIfEmpty(scan.ScannerVersion),
		scan.ScanStatus,
		nullIfEmptyBytes(scan.FindingsJSON),
		nullIfEmptyBytes(scan.SeverityCountsJSON),
		nullIfEmpty(scan.ErrorMessage),
		nullIfZeroTime(scan.ScannedAt),
	)
	return err
}

func (s *Store) UpdateArtifactVulnScan(scan store.ArtifactVulnerabilityScan) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE artifact_vulnerability_scans SET
			scan_status     = $2,
			scanner_version = $3,
			findings_json   = $4,
			severity_counts = $5,
			error_message   = $6,
			scanned_at      = $7
		WHERE scan_id = $1
	`,
		scan.ScanID,
		scan.ScanStatus,
		nullIfEmpty(scan.ScannerVersion),
		nullIfEmptyBytes(scan.FindingsJSON),
		nullIfEmptyBytes(scan.SeverityCountsJSON),
		nullIfEmpty(scan.ErrorMessage),
		nullIfZeroTime(scan.ScannedAt),
	)
	return err
}

func (s *Store) GetLatestArtifactVulnScan(artifactID string) (store.ArtifactVulnerabilityScan, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out store.ArtifactVulnerabilityScan
	var scannerVersion, errorMessage *string
	var scannedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT scan_id, artifact_id, scanner_type,
		       COALESCE(scanner_version, ''), scan_status,
		       findings_json, severity_counts,
		       COALESCE(error_message, ''), scanned_at, created_at
		FROM artifact_vulnerability_scans
		WHERE artifact_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, artifactID).Scan(
		&out.ScanID, &out.ArtifactID, &out.ScannerType,
		&out.ScannerVersion, &out.ScanStatus,
		&out.FindingsJSON, &out.SeverityCountsJSON,
		&out.ErrorMessage, &scannedAt, &out.CreatedAt,
	)
	_ = scannerVersion
	_ = errorMessage
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ArtifactVulnerabilityScan{}, false, nil
	}
	if err != nil {
		return store.ArtifactVulnerabilityScan{}, false, err
	}
	if scannedAt != nil {
		out.ScannedAt = *scannedAt
	}
	return out, true, nil
}

func (s *Store) ListArtifactVulnScans(artifactID string) ([]store.ArtifactVulnerabilityScan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT scan_id, artifact_id, scanner_type,
		       COALESCE(scanner_version, ''), scan_status,
		       findings_json, severity_counts,
		       COALESCE(error_message, ''), scanned_at, created_at
		FROM artifact_vulnerability_scans
		WHERE artifact_id = $1
		ORDER BY created_at DESC
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.ArtifactVulnerabilityScan
	for rows.Next() {
		var rec store.ArtifactVulnerabilityScan
		var scannedAt *time.Time
		if err := rows.Scan(
			&rec.ScanID, &rec.ArtifactID, &rec.ScannerType,
			&rec.ScannerVersion, &rec.ScanStatus,
			&rec.FindingsJSON, &rec.SeverityCountsJSON,
			&rec.ErrorMessage, &scannedAt, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		if scannedAt != nil {
			rec.ScannedAt = *scannedAt
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// --- Vulnerability scans (device) --------------------------------------------

func (s *Store) UpsertDeviceVulnScan(scan store.DeviceVulnerabilityScan) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_vulnerability_scans (
			device_id, scanner_type, external_scan_id, external_host_id,
			scan_status, findings_json, severity_counts, scanned_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT DO NOTHING
	`,
		scan.DeviceID,
		scan.ScannerType,
		nullIfEmpty(scan.ExternalScanID),
		nullIfEmpty(scan.ExternalHostID),
		scan.ScanStatus,
		nullIfEmptyBytes(scan.FindingsJSON),
		nullIfEmptyBytes(scan.SeverityCountsJSON),
		nullIfZeroTime(scan.ScannedAt),
	)
	return err
}

func (s *Store) GetLatestDeviceVulnScan(deviceID string) (store.DeviceVulnerabilityScan, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out store.DeviceVulnerabilityScan
	var scannedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT scan_id, device_id, scanner_type,
		       COALESCE(external_scan_id, ''), COALESCE(external_host_id, ''),
		       scan_status, findings_json, severity_counts,
		       scanned_at, synced_at, created_at
		FROM device_vulnerability_scans
		WHERE device_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, deviceID).Scan(
		&out.ScanID, &out.DeviceID, &out.ScannerType,
		&out.ExternalScanID, &out.ExternalHostID,
		&out.ScanStatus, &out.FindingsJSON, &out.SeverityCountsJSON,
		&scannedAt, &out.SyncedAt, &out.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DeviceVulnerabilityScan{}, false, nil
	}
	if err != nil {
		return store.DeviceVulnerabilityScan{}, false, err
	}
	if scannedAt != nil {
		out.ScannedAt = *scannedAt
	}
	return out, true, nil
}

func (s *Store) ListDeviceVulnScans(deviceID string) ([]store.DeviceVulnerabilityScan, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT scan_id, device_id, scanner_type,
		       COALESCE(external_scan_id, ''), COALESCE(external_host_id, ''),
		       scan_status, findings_json, severity_counts,
		       scanned_at, synced_at, created_at
		FROM device_vulnerability_scans
		WHERE device_id = $1
		ORDER BY created_at DESC
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.DeviceVulnerabilityScan
	for rows.Next() {
		var rec store.DeviceVulnerabilityScan
		var scannedAt *time.Time
		if err := rows.Scan(
			&rec.ScanID, &rec.DeviceID, &rec.ScannerType,
			&rec.ExternalScanID, &rec.ExternalHostID,
			&rec.ScanStatus, &rec.FindingsJSON, &rec.SeverityCountsJSON,
			&scannedAt, &rec.SyncedAt, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		if scannedAt != nil {
			rec.ScannedAt = *scannedAt
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ── Group lookup ──────────────────────────────────────────────────────────────

func (s *Store) GetGroup(groupID string) (store.Group, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var g store.Group
	err := s.pool.QueryRow(ctx, `
		SELECT group_id, COALESCE(name, ''), COALESCE(selector, '{}'::jsonb), created_at
		FROM groups WHERE group_id = $1
	`, groupID).Scan(&g.GroupID, &g.Name, &g.SelectorJSON, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Group{}, false, nil
	}
	if err != nil {
		return store.Group{}, false, err
	}
	return g, true, nil
}

func (s *Store) ListDevicesForGroup(groupID string, limit int) ([]store.Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.device_id, COALESCE(d.cert_fingerprint,''), COALESCE(d.cert_serial,''),
		       COALESCE(d.status,''), d.last_seen,
		       COALESCE(d.labels,'{}'), COALESCE(d.metadata,'{}')
		FROM devices d
		INNER JOIN groups g ON g.group_id = $1
		WHERE COALESCE(d.labels, '{}'::jsonb) @> COALESCE(g.selector, '{}'::jsonb)
		LIMIT $2
	`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Device
	for rows.Next() {
		var d store.Device
		if err := rows.Scan(&d.DeviceID, &d.CertFingerprint, &d.CertSerial,
			&d.Status, &d.LastSeen, &d.LabelsJSON, &d.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

func (s *Store) CreateWebhook(webhook store.Webhook) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eventTypesJSON, _ := json.Marshal(webhook.EventTypes)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhooks (id, name, url, encrypted_secret, event_types, enabled, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, webhook.ID, webhook.Name, webhook.URL, webhook.EncryptedSecret,
		eventTypesJSON, webhook.Enabled, webhook.CreatedBy,
		coalesceTime(webhook.CreatedAt))
	return err
}

func (s *Store) GetWebhook(id string) (store.Webhook, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wh store.Webhook
	var eventTypesJSON []byte
	var lastFiredAt *time.Time
	var lastStatus sql.NullInt32
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, url, encrypted_secret, event_types, enabled,
		       COALESCE(created_by,''), created_at, last_fired_at, last_status
		FROM webhooks WHERE id = $1
	`, id).Scan(&wh.ID, &wh.Name, &wh.URL, &wh.EncryptedSecret, &eventTypesJSON,
		&wh.Enabled, &wh.CreatedBy, &wh.CreatedAt, &lastFiredAt, &lastStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Webhook{}, false, nil
	}
	if err != nil {
		return store.Webhook{}, false, err
	}
	_ = json.Unmarshal(eventTypesJSON, &wh.EventTypes)
	if lastFiredAt != nil {
		wh.LastFiredAt = *lastFiredAt
	}
	if lastStatus.Valid {
		wh.LastStatus = int(lastStatus.Int32)
	}
	return wh, true, nil
}

func (s *Store) ListWebhooks() ([]store.Webhook, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, url, encrypted_secret, event_types, enabled,
		       COALESCE(created_by,''), created_at, last_fired_at, last_status
		FROM webhooks ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.Webhook
	for rows.Next() {
		var wh store.Webhook
		var eventTypesJSON []byte
		var lastFiredAt *time.Time
		var lastStatus sql.NullInt32
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.URL, &wh.EncryptedSecret,
			&eventTypesJSON, &wh.Enabled, &wh.CreatedBy, &wh.CreatedAt,
			&lastFiredAt, &lastStatus); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(eventTypesJSON, &wh.EventTypes)
		if lastFiredAt != nil {
			wh.LastFiredAt = *lastFiredAt
		}
		if lastStatus.Valid {
			wh.LastStatus = int(lastStatus.Int32)
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (s *Store) UpdateWebhook(webhook store.Webhook) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eventTypesJSON, _ := json.Marshal(webhook.EventTypes)
	_, err := s.pool.Exec(ctx, `
		UPDATE webhooks SET name=$2, url=$3, event_types=$4, enabled=$5
		WHERE id = $1
	`, webhook.ID, webhook.Name, webhook.URL, eventTypesJSON, webhook.Enabled)
	return err
}

func (s *Store) DeleteWebhook(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateWebhookLastFired(id string, at time.Time, status int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE webhooks SET last_fired_at=$2, last_status=$3 WHERE id = $1
	`, id, at, status)
	return err
}

func (s *Store) CreateWebhookDelivery(delivery store.WebhookDelivery) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries
		(id, webhook_id, event_type, payload, status, attempts, created_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
	`, delivery.ID, delivery.WebhookID, delivery.EventType,
		nullIfEmpty(string(delivery.PayloadJSON)), delivery.Status,
		delivery.Attempts, coalesceTime(delivery.CreatedAt))
	return err
}

func (s *Store) GetWebhookDelivery(deliveryID string) (store.WebhookDelivery, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var d store.WebhookDelivery
	var lastAttemptAt *time.Time
	var responseStatus sql.NullInt32
	err := s.pool.QueryRow(ctx, `
		SELECT id, webhook_id, event_type, payload, status, attempts, last_attempt_at, response_status, created_at
		FROM webhook_deliveries WHERE id = $1
	`, deliveryID).Scan(&d.ID, &d.WebhookID, &d.EventType, &d.PayloadJSON,
		&d.Status, &d.Attempts, &lastAttemptAt, &responseStatus, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.WebhookDelivery{}, false, nil
	}
	if err != nil {
		return store.WebhookDelivery{}, false, err
	}
	if lastAttemptAt != nil {
		d.LastAttemptAt = *lastAttemptAt
	}
	if responseStatus.Valid {
		d.ResponseStatus = int(responseStatus.Int32)
	}
	return d, true, nil
}

func (s *Store) UpdateWebhookDelivery(delivery store.WebhookDelivery) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var lastAttemptAt *time.Time
	if !delivery.LastAttemptAt.IsZero() {
		lastAttemptAt = &delivery.LastAttemptAt
	}
	var responseStatus *int
	if delivery.ResponseStatus != 0 {
		responseStatus = &delivery.ResponseStatus
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status=$2, attempts=$3, last_attempt_at=$4, response_status=$5
		WHERE id = $1
	`, delivery.ID, delivery.Status, delivery.Attempts, lastAttemptAt, responseStatus)
	return err
}

func (s *Store) ListWebhookDeliveries(webhookID string, limit int) ([]store.WebhookDelivery, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_id, event_type, COALESCE(payload,'{}'), status,
		       attempts, last_attempt_at, response_status, created_at
		FROM webhook_deliveries
		WHERE webhook_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.WebhookDelivery
	for rows.Next() {
		var d store.WebhookDelivery
		var lastAttemptAt *time.Time
		var responseStatus sql.NullInt32
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventType, &d.PayloadJSON,
			&d.Status, &d.Attempts, &lastAttemptAt, &responseStatus, &d.CreatedAt); err != nil {
			return nil, err
		}
		if lastAttemptAt != nil {
			d.LastAttemptAt = *lastAttemptAt
		}
		if responseStatus.Valid {
			d.ResponseStatus = int(responseStatus.Int32)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ── Deploy triggers ───────────────────────────────────────────────────────────

func (s *Store) UpsertDeployTrigger(trigger store.DeployTrigger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	at := trigger.TriggeredAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO deploy_triggers (device_id, triggered_by, triggered_at, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (device_id) DO UPDATE SET
			triggered_by = EXCLUDED.triggered_by,
			triggered_at = EXCLUDED.triggered_at,
			reason       = EXCLUDED.reason
	`, trigger.DeviceID, trigger.TriggeredBy, at, trigger.Reason)
	return err
}

func (s *Store) ConsumeDeployTrigger(deviceID string) (store.DeployTrigger, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var t store.DeployTrigger
	err := s.pool.QueryRow(ctx, `
		DELETE FROM deploy_triggers WHERE device_id = $1
		RETURNING device_id, COALESCE(triggered_by,''), triggered_at, COALESCE(reason,'')
	`, deviceID).Scan(&t.DeviceID, &t.TriggeredBy, &t.TriggeredAt, &t.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DeployTrigger{}, false, nil
	}
	if err != nil {
		return store.DeployTrigger{}, false, err
	}
	return t, true, nil
}

func coalesceTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

// ── Deployment status ─────────────────────────────────────────────────────────

func (s *Store) GetGroupDeploymentStatus(groupID, artifactID string) ([]store.DeviceDeploymentStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT d.device_id::text,
		       COALESCE(dar.status, 'pending'),
		       COALESCE(dar.applied_version, ''),
		       COALESCE(dar.error, ''),
		       dar.last_apply_at
		FROM devices d
		INNER JOIN groups g ON g.group_id = $1
		LEFT JOIN LATERAL (
		    SELECT status, applied_version, error, created_at AS last_apply_at
		    FROM device_apply_results
		    WHERE device_id = d.device_id
		      AND artifact_id = $2
		    ORDER BY created_at DESC
		    LIMIT 1
		) dar ON true
		WHERE COALESCE(d.labels, '{}'::jsonb) @> COALESCE(g.selector, '{}'::jsonb)
		  AND d.status != 'decommissioned'
		ORDER BY d.device_id
	`, groupID, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.DeviceDeploymentStatus
	for rows.Next() {
		var s store.DeviceDeploymentStatus
		var lastApplyAt *time.Time
		if err := rows.Scan(&s.DeviceID, &s.Status, &s.AppliedVersion, &s.Error, &lastApplyAt); err != nil {
			return nil, err
		}
		if lastApplyAt != nil {
			s.LastApplyAt = *lastApplyAt
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ── Federation ingest ──────────────────────────────────────────────────────────

// ── Global policy cache ───────────────────────────────────────────────────────

func (s *Store) UpsertGlobalPolicyCache(policy store.GlobalPolicyCache) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sel := policy.SelectorJSON
	if len(sel) == 0 {
		sel = []byte("{}")
	}
	pol := policy.PolicyJSON
	if len(pol) == 0 {
		pol = []byte("{}")
	}
	comp := policy.ComponentsJSON
	if len(comp) == 0 {
		comp = []byte("{}")
	}
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO global_policy_cache
			(group_id, group_name, selector_json, artifact_id, desired_version,
			 desired_config_rev, policy_json, components_json, checkin_interval,
			 received_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		ON CONFLICT (group_id) DO UPDATE SET
			group_name        = EXCLUDED.group_name,
			selector_json     = EXCLUDED.selector_json,
			artifact_id       = EXCLUDED.artifact_id,
			desired_version   = EXCLUDED.desired_version,
			desired_config_rev = EXCLUDED.desired_config_rev,
			policy_json       = EXCLUDED.policy_json,
			components_json   = EXCLUDED.components_json,
			checkin_interval  = EXCLUDED.checkin_interval,
			updated_at        = $10
	`, policy.GroupID, policy.GroupName, sel, policy.ArtifactID, policy.DesiredVersion,
		policy.DesiredConfigRev, pol, comp, policy.CheckinInterval, now)
	return err
}

func (s *Store) ListGlobalPolicyCaches() ([]store.GlobalPolicyCache, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT cache_id, group_id, group_name, selector_json, artifact_id, desired_version,
		       desired_config_rev, policy_json, components_json, checkin_interval,
		       received_at, updated_at
		FROM global_policy_cache ORDER BY group_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.GlobalPolicyCache
	for rows.Next() {
		var p store.GlobalPolicyCache
		if err := rows.Scan(
			&p.CacheID, &p.GroupID, &p.GroupName, &p.SelectorJSON,
			&p.ArtifactID, &p.DesiredVersion, &p.DesiredConfigRev,
			&p.PolicyJSON, &p.ComponentsJSON, &p.CheckinInterval,
			&p.ReceivedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetGlobalPolicyCacheForDevice(deviceID string) (store.GlobalPolicyCache, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var p store.GlobalPolicyCache
	err := s.pool.QueryRow(ctx, `
		SELECT gpc.cache_id, gpc.group_id, gpc.group_name, gpc.selector_json,
		       gpc.artifact_id, gpc.desired_version, gpc.desired_config_rev,
		       gpc.policy_json, gpc.components_json, gpc.checkin_interval,
		       gpc.received_at, gpc.updated_at
		FROM global_policy_cache gpc
		JOIN devices d ON d.device_id = $1
		WHERE COALESCE(d.labels, '{}'::jsonb) @> COALESCE(gpc.selector_json, '{}'::jsonb)
		ORDER BY gpc.updated_at DESC
		LIMIT 1
	`, deviceID).Scan(
		&p.CacheID, &p.GroupID, &p.GroupName, &p.SelectorJSON,
		&p.ArtifactID, &p.DesiredVersion, &p.DesiredConfigRev,
		&p.PolicyJSON, &p.ComponentsJSON, &p.CheckinInterval,
		&p.ReceivedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.GlobalPolicyCache{}, false, nil
	}
	if err != nil {
		return store.GlobalPolicyCache{}, false, err
	}
	return p, true, nil
}

func (s *Store) DeleteGlobalPolicyCache(groupID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM global_policy_cache WHERE group_id = $1`, groupID)
	return err
}

func (s *Store) UpsertFederationIngest(ingest store.FederationIngest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO federation_artifact_ingest
			(ingest_id, artifact_id, global_object_key, global_presign_base_url, received_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (artifact_id) DO UPDATE SET
			global_object_key       = EXCLUDED.global_object_key,
			global_presign_base_url = EXCLUDED.global_presign_base_url,
			updated_at              = $5
	`, ingest.IngestID, ingest.ArtifactID, ingest.GlobalObjectKey, ingest.GlobalPresignBaseURL, now)
	return err
}

func (s *Store) GetFederationIngest(artifactID string) (store.FederationIngest, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var fi store.FederationIngest
	var blobConfirmedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT ingest_id, artifact_id, global_object_key, global_presign_base_url,
		       blob_confirmed, blob_confirmed_at, received_at, updated_at
		FROM federation_artifact_ingest WHERE artifact_id = $1
	`, artifactID).Scan(
		&fi.IngestID, &fi.ArtifactID, &fi.GlobalObjectKey, &fi.GlobalPresignBaseURL,
		&fi.BlobConfirmed, &blobConfirmedAt, &fi.ReceivedAt, &fi.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.FederationIngest{}, false, nil
	}
	if err != nil {
		return store.FederationIngest{}, false, err
	}
	if blobConfirmedAt != nil {
		fi.BlobConfirmedAt = *blobConfirmedAt
	}
	return fi, true, nil
}

func (s *Store) ConfirmFederationBlobLocal(artifactID string, confirmedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE federation_artifact_ingest
		SET blob_confirmed = TRUE, blob_confirmed_at = $2, updated_at = $2
		WHERE artifact_id = $1
	`, artifactID, confirmedAt)
	return err
}

func (s *Store) CreateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	_, err := s.pool.Exec(ctx, `
		INSERT INTO iec62304_change_records
			(record_id, artifact_id, safety_class, impact_summary, risk_controls, status,
			 created_by_user_id, updated_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, record.RecordID, record.ArtifactID, record.SafetyClass,
		record.ImpactSummary, record.RiskControls, record.Status,
		record.CreatedByUserID, record.UpdatedAt, record.CreatedAt)
	if err != nil {
		return store.ChangeRecord{}, err
	}
	return record, nil
}

func (s *Store) GetChangeRecord(recordID string) (store.ChangeRecord, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var r store.ChangeRecord
	var approvedAt, rejectedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT record_id, artifact_id, safety_class, impact_summary, risk_controls, status,
		       created_by_user_id, updated_at, created_at,
		       approved_by_user_id, approved_at,
		       rejected_by_user_id, rejected_at, rejected_reason
		FROM iec62304_change_records WHERE record_id = $1
	`, recordID).Scan(
		&r.RecordID, &r.ArtifactID, &r.SafetyClass, &r.ImpactSummary, &r.RiskControls, &r.Status,
		&r.CreatedByUserID, &r.UpdatedAt, &r.CreatedAt,
		&r.ApprovedByUserID, &approvedAt,
		&r.RejectedByUserID, &rejectedAt, &r.RejectedReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ChangeRecord{}, false, nil
	}
	if err != nil {
		return store.ChangeRecord{}, false, err
	}
	if approvedAt.Valid {
		r.ApprovedAt = approvedAt.Time
	}
	if rejectedAt.Valid {
		r.RejectedAt = rejectedAt.Time
	}
	return r, true, nil
}

func (s *Store) GetChangeRecordForArtifact(artifactID string) (store.ChangeRecord, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var r store.ChangeRecord
	var approvedAt, rejectedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT record_id, artifact_id, safety_class, impact_summary, risk_controls, status,
		       created_by_user_id, updated_at, created_at,
		       approved_by_user_id, approved_at,
		       rejected_by_user_id, rejected_at, rejected_reason
		FROM iec62304_change_records WHERE artifact_id = $1
	`, artifactID).Scan(
		&r.RecordID, &r.ArtifactID, &r.SafetyClass, &r.ImpactSummary, &r.RiskControls, &r.Status,
		&r.CreatedByUserID, &r.UpdatedAt, &r.CreatedAt,
		&r.ApprovedByUserID, &approvedAt,
		&r.RejectedByUserID, &rejectedAt, &r.RejectedReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ChangeRecord{}, false, nil
	}
	if err != nil {
		return store.ChangeRecord{}, false, err
	}
	if approvedAt.Valid {
		r.ApprovedAt = approvedAt.Time
	}
	if rejectedAt.Valid {
		r.RejectedAt = rejectedAt.Time
	}
	return r, true, nil
}

func (s *Store) UpdateChangeRecord(record store.ChangeRecord) (store.ChangeRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE iec62304_change_records
		SET safety_class = $2, impact_summary = $3, risk_controls = $4, status = $5,
		    approved_by_user_id = $6, approved_at = $7,
		    rejected_by_user_id = $8, rejected_at = $9, rejected_reason = $10,
		    updated_at = $11
		WHERE record_id = $1
	`, record.RecordID, record.SafetyClass, record.ImpactSummary, record.RiskControls, record.Status,
		record.ApprovedByUserID, nullTime(record.ApprovedAt),
		record.RejectedByUserID, nullTime(record.RejectedAt), record.RejectedReason,
		record.UpdatedAt)
	if err != nil {
		return store.ChangeRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return store.ChangeRecord{}, store.ErrChangeRecordNotFound
	}
	return record, nil
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func (s *Store) SetArtifactVexObjectKey(artifactID, vexObjectKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE artifacts SET vex_object_key = $2 WHERE artifact_id = $1`,
		artifactID, vexObjectKey,
	)
	return err
}

func (s *Store) UpsertVexAssertion(assertion store.VexAssertion) (store.VexAssertion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assertion.CreatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vex_assertions
		    (assertion_id, artifact_id, cve_id, component_name, assertion, justification, actor_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (artifact_id, cve_id, component_name)
		DO UPDATE SET
		    assertion_id   = EXCLUDED.assertion_id,
		    assertion      = EXCLUDED.assertion,
		    justification  = EXCLUDED.justification,
		    actor_user_id  = EXCLUDED.actor_user_id,
		    created_at     = EXCLUDED.created_at
	`, assertion.AssertionID, assertion.ArtifactID, assertion.CVEID,
		assertion.ComponentName, assertion.Assertion,
		assertion.Justification, assertion.ActorUserID, assertion.CreatedAt)
	if err != nil {
		return store.VexAssertion{}, err
	}
	return assertion, nil
}

func (s *Store) ListVexAssertions(artifactID string) ([]store.VexAssertion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT assertion_id, artifact_id, cve_id, component_name,
		       assertion, justification, actor_user_id, created_at
		FROM vex_assertions WHERE artifact_id = $1
		ORDER BY cve_id, component_name
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.VexAssertion
	for rows.Next() {
		var a store.VexAssertion
		if err := rows.Scan(&a.AssertionID, &a.ArtifactID, &a.CVEID, &a.ComponentName,
			&a.Assertion, &a.Justification, &a.ActorUserID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
