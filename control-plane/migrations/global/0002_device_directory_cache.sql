CREATE TABLE IF NOT EXISTS device_directory_cache (
    cache_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plane_id  UUID NOT NULL REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    status    TEXT NOT NULL DEFAULT '',
    last_seen TIMESTAMPTZ,
    labels    JSONB,
    metadata  JSONB,
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plane_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_device_cache_plane  ON device_directory_cache(plane_id);
CREATE INDEX IF NOT EXISTS idx_device_cache_status ON device_directory_cache(status);
