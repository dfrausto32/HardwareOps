CREATE TABLE IF NOT EXISTS artifact_cache (
    cache_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plane_id      UUID NOT NULL REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    artifact_id   TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    version       TEXT NOT NULL DEFAULT '',
    artifact_type TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT '',
    sha256        TEXT NOT NULL DEFAULT '',
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ,
    synced_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plane_id, artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_artifact_cache_plane ON artifact_cache(plane_id);
CREATE INDEX IF NOT EXISTS idx_artifact_cache_name  ON artifact_cache(name);
