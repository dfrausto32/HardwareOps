package globalplane

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateGlobalArtifact(a GlobalArtifact) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO global_artifacts
			(artifact_id, name, version, artifact_type, status, object_key,
			 sha256, signature, signature_type, signature_key_id, size_bytes,
			 metadata_json, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13)
	`, a.ArtifactID, a.Name, a.Version, a.ArtifactType, a.Status, a.ObjectKey,
		a.SHA256, a.Signature, a.SignatureType, a.SignatureKeyID, a.SizeBytes,
		nullIfEmptyStr(string(a.MetadataJSON)), a.CreatedBy)
	return err
}

func (s *PostgresStore) GetGlobalArtifact(artifactID string) (GlobalArtifact, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		SELECT artifact_id, name, version, artifact_type, status, object_key,
		       sha256, signature, signature_type, signature_key_id, size_bytes,
		       metadata_json, created_by, created_at, updated_at
		FROM global_artifacts WHERE artifact_id = $1
	`, artifactID)
	a, err := scanGlobalArtifact(row)
	if err == pgx.ErrNoRows {
		return GlobalArtifact{}, false, nil
	}
	return a, err == nil, err
}

func (s *PostgresStore) ListGlobalArtifacts(nameFilter string) ([]GlobalArtifact, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := `SELECT artifact_id, name, version, artifact_type, status, object_key,
		         sha256, signature, signature_type, signature_key_id, size_bytes,
		         metadata_json, created_by, created_at, updated_at
		  FROM global_artifacts WHERE TRUE`
	args := []any{}
	if nameFilter != "" {
		q += ` AND name = $1`
		args = append(args, nameFilter)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var arts []GlobalArtifact
	for rows.Next() {
		a, err := scanGlobalArtifact(rows)
		if err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	return arts, rows.Err()
}

func (s *PostgresStore) CreateReplicationStatusRows(artifactID string, planeIDs []string) error {
	if len(planeIDs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batch := &pgx.Batch{}
	for _, pid := range planeIDs {
		batch.Queue(`
			INSERT INTO artifact_replication_status (artifact_id, plane_id)
			VALUES ($1, $2::uuid)
			ON CONFLICT (artifact_id, plane_id) DO NOTHING
		`, artifactID, pid)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range planeIDs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) UpdateReplicationStatusMetadataPush(artifactID, planeID string, pushedAt *time.Time, pushErr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE artifact_replication_status
		SET metadata_pushed_at = $3, metadata_push_error = $4, updated_at = now()
		WHERE artifact_id = $1 AND plane_id = $2::uuid
	`, artifactID, planeID, pushedAt, pushErr)
	return err
}

func (s *PostgresStore) UpdateReplicationStatusBlobConfirmed(artifactID, planeID string, confirmedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE artifact_replication_status
		SET blob_status = 'confirmed', blob_confirmed_at = $3, blob_check_error = '', updated_at = now()
		WHERE artifact_id = $1 AND plane_id = $2::uuid
	`, artifactID, planeID, confirmedAt)
	return err
}

func (s *PostgresStore) UpdateReplicationStatusBlobCheckError(artifactID, planeID, checkErr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE artifact_replication_status
		SET blob_check_error = $3, blob_status = 'replicating', updated_at = now()
		WHERE artifact_id = $1 AND plane_id = $2::uuid
	`, artifactID, planeID, checkErr)
	return err
}

func (s *PostgresStore) ListReplicationStatus(artifactID string) ([]ArtifactReplicationStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT r.replication_id, r.artifact_id, r.plane_id,
		       COALESCE(p.name, r.plane_id::text),
		       r.metadata_pushed_at, r.metadata_push_error,
		       r.blob_status, r.blob_confirmed_at, r.blob_check_error,
		       r.created_at, r.updated_at
		FROM artifact_replication_status r
		LEFT JOIN regional_planes p ON p.plane_id = r.plane_id
		WHERE r.artifact_id = $1
		ORDER BY p.name
	`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReplicationRows(rows)
}

func (s *PostgresStore) ListPendingReplicationRows() ([]ArtifactReplicationStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT r.replication_id, r.artifact_id, r.plane_id,
		       COALESCE(p.name, r.plane_id::text),
		       r.metadata_pushed_at, r.metadata_push_error,
		       r.blob_status, r.blob_confirmed_at, r.blob_check_error,
		       r.created_at, r.updated_at
		FROM artifact_replication_status r
		LEFT JOIN regional_planes p ON p.plane_id = r.plane_id
		WHERE r.blob_status IN ('pending', 'replicating')
		ORDER BY r.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReplicationRows(rows)
}

func scanReplicationRows(rows pgx.Rows) ([]ArtifactReplicationStatus, error) {
	var out []ArtifactReplicationStatus
	for rows.Next() {
		var r ArtifactReplicationStatus
		if err := rows.Scan(
			&r.ReplicationID, &r.ArtifactID, &r.PlaneID, &r.PlaneName,
			&r.MetadataPushedAt, &r.MetadataPushError,
			&r.BlobStatus, &r.BlobConfirmedAt, &r.BlobCheckError,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanGlobalArtifact(row scannable) (GlobalArtifact, error) {
	var a GlobalArtifact
	var metaJSON []byte
	err := row.Scan(
		&a.ArtifactID, &a.Name, &a.Version, &a.ArtifactType, &a.Status, &a.ObjectKey,
		&a.SHA256, &a.Signature, &a.SignatureType, &a.SignatureKeyID, &a.SizeBytes,
		&metaJSON, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return GlobalArtifact{}, err
	}
	a.MetadataJSON = metaJSON
	return a, nil
}
