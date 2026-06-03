package globalplane

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Global enrollment profiles ────────────────────────────────────────────────

func (s *PostgresStore) CreateGlobalEnrollmentProfile(p GlobalEnrollmentProfile) (GlobalEnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	labels := p.DefaultLabelsJSON
	if len(labels) == 0 {
		labels = []byte("{}")
	}
	now := time.Now().UTC()
	err := s.pool.QueryRow(ctx, `
		INSERT INTO global_enrollment_profiles
			(name, require_approval, allow_untrusted_hw, challenge_hint,
			 approval_delay_sec, max_uses, cert_validity_days, default_labels_json,
			 disabled, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
		RETURNING profile_id, name, require_approval, allow_untrusted_hw, challenge_hint,
		          approval_delay_sec, max_uses, cert_validity_days, default_labels_json,
		          disabled, COALESCE(created_by,''), created_at, updated_at
	`, p.Name, p.RequireApproval, p.AllowUntrustedHW, p.ChallengeHint,
		p.ApprovalDelaySec, p.MaxUses, p.CertValidityDays, labels,
		p.Disabled, p.CreatedBy, now,
	).Scan(
		&p.ProfileID, &p.Name, &p.RequireApproval, &p.AllowUntrustedHW, &p.ChallengeHint,
		&p.ApprovalDelaySec, &p.MaxUses, &p.CertValidityDays, &p.DefaultLabelsJSON,
		&p.Disabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (s *PostgresStore) GetGlobalEnrollmentProfile(profileID string) (GlobalEnrollmentProfile, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var p GlobalEnrollmentProfile
	err := s.pool.QueryRow(ctx, `
		SELECT profile_id, name, require_approval, allow_untrusted_hw, challenge_hint,
		       approval_delay_sec, max_uses, cert_validity_days, default_labels_json,
		       disabled, COALESCE(created_by,''), created_at, updated_at
		FROM global_enrollment_profiles WHERE profile_id = $1
	`, profileID).Scan(
		&p.ProfileID, &p.Name, &p.RequireApproval, &p.AllowUntrustedHW, &p.ChallengeHint,
		&p.ApprovalDelaySec, &p.MaxUses, &p.CertValidityDays, &p.DefaultLabelsJSON,
		&p.Disabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GlobalEnrollmentProfile{}, false, nil
	}
	return p, err == nil, err
}

func (s *PostgresStore) ListGlobalEnrollmentProfiles() ([]GlobalEnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT profile_id, name, require_approval, allow_untrusted_hw, challenge_hint,
		       approval_delay_sec, max_uses, cert_validity_days, default_labels_json,
		       disabled, COALESCE(created_by,''), created_at, updated_at
		FROM global_enrollment_profiles ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlobalEnrollmentProfile
	for rows.Next() {
		var p GlobalEnrollmentProfile
		if err := rows.Scan(
			&p.ProfileID, &p.Name, &p.RequireApproval, &p.AllowUntrustedHW, &p.ChallengeHint,
			&p.ApprovalDelaySec, &p.MaxUses, &p.CertValidityDays, &p.DefaultLabelsJSON,
			&p.Disabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateGlobalEnrollmentProfile(profileID string, u GlobalEnrollmentProfileUpdate) (GlobalEnrollmentProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	labels := u.DefaultLabelsJSON
	if len(labels) == 0 {
		labels = []byte("{}")
	}
	var p GlobalEnrollmentProfile
	err := s.pool.QueryRow(ctx, `
		UPDATE global_enrollment_profiles SET
			name               = $2,
			require_approval   = $3,
			allow_untrusted_hw = $4,
			challenge_hint     = $5,
			approval_delay_sec = $6,
			max_uses           = $7,
			cert_validity_days = $8,
			default_labels_json = $9,
			updated_at         = NOW()
		WHERE profile_id = $1
		RETURNING profile_id, name, require_approval, allow_untrusted_hw, challenge_hint,
		          approval_delay_sec, max_uses, cert_validity_days, default_labels_json,
		          disabled, COALESCE(created_by,''), created_at, updated_at
	`, profileID, u.Name, u.RequireApproval, u.AllowUntrustedHW, u.ChallengeHint,
		u.ApprovalDelaySec, u.MaxUses, u.CertValidityDays, labels,
	).Scan(
		&p.ProfileID, &p.Name, &p.RequireApproval, &p.AllowUntrustedHW, &p.ChallengeHint,
		&p.ApprovalDelaySec, &p.MaxUses, &p.CertValidityDays, &p.DefaultLabelsJSON,
		&p.Disabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GlobalEnrollmentProfile{}, pgx.ErrNoRows
	}
	return p, err
}

func (s *PostgresStore) DeleteGlobalEnrollmentProfile(profileID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM global_enrollment_profiles WHERE profile_id = $1`, profileID)
	return err
}

// ── Pending enrollment cache ──────────────────────────────────────────────────

func (s *PostgresStore) UpsertGlobalPendingEnrollments(planeID string, items []GlobalPendingEnrollment) error {
	if len(items) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, item := range items {
		meta := item.MetadataJSON
		if len(meta) == 0 {
			meta = []byte("{}")
		}
		caps := item.CapabilitiesJSON
		if len(caps) == 0 {
			caps = []byte("{}")
		}
		batch.Queue(`
			INSERT INTO global_pending_enrollments_cache
				(plane_id, request_id, profile_id, status, source_ip, agent_version,
				 hardware_id, metadata_json, capabilities_json, denied_reason,
				 expires_at, approval_available_at, created_at, synced_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (plane_id, request_id) DO UPDATE SET
				status               = EXCLUDED.status,
				profile_id           = EXCLUDED.profile_id,
				source_ip            = EXCLUDED.source_ip,
				agent_version        = EXCLUDED.agent_version,
				hardware_id          = EXCLUDED.hardware_id,
				metadata_json        = EXCLUDED.metadata_json,
				capabilities_json    = EXCLUDED.capabilities_json,
				denied_reason        = EXCLUDED.denied_reason,
				expires_at           = EXCLUDED.expires_at,
				approval_available_at = EXCLUDED.approval_available_at,
				synced_at            = EXCLUDED.synced_at
		`, planeID, item.RequestID, item.ProfileID, item.Status, item.SourceIP,
			item.AgentVersion, item.HardwareID, meta, caps, item.DeniedReason,
			item.ExpiresAt, item.ApprovalAvailableAt, item.CreatedAt, now,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range items {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) PurgeGlobalPendingEnrollmentsForPlane(planeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`DELETE FROM global_pending_enrollments_cache WHERE plane_id = $1`,
		planeID,
	)
	return err
}

func (s *PostgresStore) ListGlobalPendingEnrollments(filter GlobalPendingEnrollmentFilter) ([]GlobalPendingEnrollment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var rows pgx.Rows
	var err error
	if filter.PlaneID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT c.cache_id, c.plane_id, COALESCE(p.name,'') AS plane_name,
			       c.request_id, COALESCE(c.profile_id,''), COALESCE(c.status,''),
			       COALESCE(c.source_ip,''), COALESCE(c.agent_version,''), COALESCE(c.hardware_id,''),
			       c.metadata_json, c.capabilities_json, COALESCE(c.denied_reason,''),
			       c.expires_at, c.approval_available_at, c.created_at, c.synced_at
			FROM global_pending_enrollments_cache c
			LEFT JOIN regional_planes p ON p.plane_id = c.plane_id
			WHERE ($1 = '' OR c.status = $1) AND c.plane_id = $2
			ORDER BY c.created_at DESC
		`, filter.Status, filter.PlaneID)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT c.cache_id, c.plane_id, COALESCE(p.name,'') AS plane_name,
			       c.request_id, COALESCE(c.profile_id,''), COALESCE(c.status,''),
			       COALESCE(c.source_ip,''), COALESCE(c.agent_version,''), COALESCE(c.hardware_id,''),
			       c.metadata_json, c.capabilities_json, COALESCE(c.denied_reason,''),
			       c.expires_at, c.approval_available_at, c.created_at, c.synced_at
			FROM global_pending_enrollments_cache c
			LEFT JOIN regional_planes p ON p.plane_id = c.plane_id
			WHERE ($1 = '' OR c.status = $1)
			ORDER BY c.created_at DESC
		`, filter.Status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlobalPendingEnrollment
	for rows.Next() {
		var e GlobalPendingEnrollment
		if err := rows.Scan(
			&e.CacheID, &e.PlaneID, &e.PlaneName,
			&e.RequestID, &e.ProfileID, &e.Status,
			&e.SourceIP, &e.AgentVersion, &e.HardwareID,
			&e.MetadataJSON, &e.CapabilitiesJSON, &e.DeniedReason,
			&e.ExpiresAt, &e.ApprovalAvailableAt, &e.CreatedAt, &e.SyncedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

