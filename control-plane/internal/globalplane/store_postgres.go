package globalplane

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the Postgres implementation of Store.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a PostgresStore backed by the given pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateRegionalPlane(p RegionalPlane) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO regional_planes
			(plane_id, name, base_url, encrypted_token, tls_ca_pem, enabled, sync_interval_seconds, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, p.PlaneID, p.Name, p.BaseURL, p.EncryptedToken, p.TLSCAPem, p.Enabled, p.SyncIntervalSeconds, p.CreatedBy)
	return err
}

func (s *PostgresStore) GetRegionalPlane(planeID string) (RegionalPlane, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		SELECT plane_id, name, base_url, encrypted_token, tls_ca_pem, enabled,
		       sync_interval_seconds, last_sync_at, last_sync_error, created_by, created_at, updated_at
		FROM regional_planes WHERE plane_id = $1
	`, planeID)
	p, err := scanPlane(row)
	if err == pgx.ErrNoRows {
		return RegionalPlane{}, false, nil
	}
	return p, err == nil, err
}

func (s *PostgresStore) ListRegionalPlanes() ([]RegionalPlane, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT plane_id, name, base_url, encrypted_token, tls_ca_pem, enabled,
		       sync_interval_seconds, last_sync_at, last_sync_error, created_by, created_at, updated_at
		FROM regional_planes ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var planes []RegionalPlane
	for rows.Next() {
		p, err := scanPlane(rows)
		if err != nil {
			return nil, err
		}
		planes = append(planes, p)
	}
	return planes, rows.Err()
}

func (s *PostgresStore) UpdateRegionalPlane(planeID, name, baseURL string, encryptedToken []byte, tlsCAPem string, syncIntervalSeconds int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE regional_planes SET
			name = $2, base_url = $3, encrypted_token = $4, tls_ca_pem = $5,
			sync_interval_seconds = $6, updated_at = now()
		WHERE plane_id = $1
	`, planeID, name, baseURL, encryptedToken, tlsCAPem, syncIntervalSeconds)
	return err
}

func (s *PostgresStore) UpdateRegionalPlaneSyncStatus(planeID string, syncedAt time.Time, syncErr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE regional_planes SET last_sync_at = $2, last_sync_error = $3, updated_at = now()
		WHERE plane_id = $1
	`, planeID, syncedAt, syncErr)
	return err
}

func (s *PostgresStore) DeleteRegionalPlane(planeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM regional_planes WHERE plane_id = $1`, planeID)
	return err
}

func (s *PostgresStore) UpsertDeviceCacheEntries(planeID string, devices []CachedDevice) error {
	if len(devices) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	batch := &pgx.Batch{}
	for _, d := range devices {
		labelsJSON, _ := json.Marshal(d.Labels)
		metaJSON, _ := json.Marshal(d.Metadata)
		batch.Queue(`
			INSERT INTO device_directory_cache (plane_id, device_id, status, last_seen, labels, metadata, synced_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, now())
			ON CONFLICT (plane_id, device_id) DO UPDATE SET
				status    = EXCLUDED.status,
				last_seen = EXCLUDED.last_seen,
				labels    = EXCLUDED.labels,
				metadata  = EXCLUDED.metadata,
				synced_at = now()
		`, planeID, d.DeviceID, d.Status, d.LastSeen,
			nullIfEmptyStr(string(labelsJSON)), nullIfEmptyStr(string(metaJSON)))
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(devices); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) ListDeviceCache(filter DeviceCacheFilter) ([]CachedDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := `SELECT cache_id, plane_id, device_id, status, last_seen, labels, metadata, synced_at
	      FROM device_directory_cache WHERE TRUE`
	args := []any{}
	idx := 1
	if filter.PlaneID != "" {
		q += ` AND plane_id = $` + itoa(idx)
		args = append(args, filter.PlaneID)
		idx++
	}
	if filter.Status != "" {
		q += ` AND status = $` + itoa(idx)
		args = append(args, filter.Status)
		idx++
	}
	q += ` ORDER BY plane_id, device_id`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []CachedDevice
	for rows.Next() {
		var d CachedDevice
		var labelsJSON, metaJSON []byte
		if err := rows.Scan(&d.CacheID, &d.PlaneID, &d.DeviceID, &d.Status,
			&d.LastSeen, &labelsJSON, &metaJSON, &d.SyncedAt); err != nil {
			return nil, err
		}
		if len(labelsJSON) > 0 {
			_ = json.Unmarshal(labelsJSON, &d.Labels)
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &d.Metadata)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (s *PostgresStore) PurgeDeviceCacheForPlane(planeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM device_directory_cache WHERE plane_id = $1`, planeID)
	return err
}

func (s *PostgresStore) UpsertArtifactCacheEntries(planeID string, artifacts []CachedArtifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	batch := &pgx.Batch{}
	for _, a := range artifacts {
		batch.Queue(`
			INSERT INTO artifact_cache
				(plane_id, artifact_id, name, version, artifact_type, status, sha256, size_bytes, created_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
			ON CONFLICT (plane_id, artifact_id) DO UPDATE SET
				name          = EXCLUDED.name,
				version       = EXCLUDED.version,
				artifact_type = EXCLUDED.artifact_type,
				status        = EXCLUDED.status,
				sha256        = EXCLUDED.sha256,
				size_bytes    = EXCLUDED.size_bytes,
				created_at    = EXCLUDED.created_at,
				synced_at     = now()
		`, planeID, a.ArtifactID, a.Name, a.Version, a.ArtifactType,
			a.Status, a.SHA256, a.SizeBytes, a.CreatedAt)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(artifacts); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) ListArtifactCache(filter ArtifactCacheFilter) ([]CachedArtifact, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := `SELECT cache_id, plane_id, artifact_id, name, version, artifact_type, status, sha256, size_bytes, created_at, synced_at
	      FROM artifact_cache WHERE TRUE`
	args := []any{}
	idx := 1
	if filter.PlaneID != "" {
		q += ` AND plane_id = $` + itoa(idx)
		args = append(args, filter.PlaneID)
		idx++
	}
	if filter.Name != "" {
		q += ` AND name = $` + itoa(idx)
		args = append(args, filter.Name)
		idx++
	}
	q += ` ORDER BY plane_id, name, version`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var arts []CachedArtifact
	for rows.Next() {
		var a CachedArtifact
		if err := rows.Scan(&a.CacheID, &a.PlaneID, &a.ArtifactID, &a.Name,
			&a.Version, &a.ArtifactType, &a.Status, &a.SHA256, &a.SizeBytes,
			&a.CreatedAt, &a.SyncedAt); err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	return arts, rows.Err()
}

func (s *PostgresStore) UpsertHealthCache(snap HealthSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO health_cache
			(plane_id, total_devices, active_devices, stale_devices, offline_devices, degraded_devices, last_device_seen, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (plane_id) DO UPDATE SET
			total_devices    = EXCLUDED.total_devices,
			active_devices   = EXCLUDED.active_devices,
			stale_devices    = EXCLUDED.stale_devices,
			offline_devices  = EXCLUDED.offline_devices,
			degraded_devices = EXCLUDED.degraded_devices,
			last_device_seen = EXCLUDED.last_device_seen,
			synced_at        = now()
	`, snap.PlaneID, snap.TotalDevices, snap.ActiveDevices, snap.StaleDevices,
		snap.OfflineDevices, snap.DegradedDevices, snap.LastDeviceSeen)
	return err
}

func (s *PostgresStore) ListHealthCache() ([]HealthSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT plane_id, total_devices, active_devices, stale_devices, offline_devices, degraded_devices, last_device_seen, synced_at
		FROM health_cache ORDER BY plane_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snaps []HealthSnapshot
	for rows.Next() {
		var h HealthSnapshot
		if err := rows.Scan(&h.PlaneID, &h.TotalDevices, &h.ActiveDevices, &h.StaleDevices,
			&h.OfflineDevices, &h.DegradedDevices, &h.LastDeviceSeen, &h.SyncedAt); err != nil {
			return nil, err
		}
		snaps = append(snaps, h)
	}
	return snaps, rows.Err()
}

// scanPlane scans a regional_planes row. Works for both *pgx.Row and pgx.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanPlane(row scannable) (RegionalPlane, error) {
	var p RegionalPlane
	err := row.Scan(
		&p.PlaneID, &p.Name, &p.BaseURL, &p.EncryptedToken, &p.TLSCAPem, &p.Enabled,
		&p.SyncIntervalSeconds, &p.LastSyncAt, &p.LastSyncError, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func nullIfEmptyStr(s string) interface{} {
	if s == "" || s == "null" {
		return nil
	}
	return s
}

func itoa(n int) string {
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	// For our use (1-9 parameters) this is sufficient.
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
