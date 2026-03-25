CREATE TABLE IF NOT EXISTS health_cache (
    plane_id         UUID PRIMARY KEY REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    total_devices    INT NOT NULL DEFAULT 0,
    active_devices   INT NOT NULL DEFAULT 0,
    stale_devices    INT NOT NULL DEFAULT 0,
    offline_devices  INT NOT NULL DEFAULT 0,
    degraded_devices INT NOT NULL DEFAULT 0,
    last_device_seen TIMESTAMPTZ,
    synced_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
