-- Canonical record of artifacts uploaded directly to the global plane.
CREATE TABLE IF NOT EXISTS global_artifacts (
    artifact_id      TEXT PRIMARY KEY,
    name             TEXT NOT NULL DEFAULT '',
    version          TEXT NOT NULL DEFAULT '',
    artifact_type    TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active',
    object_key       TEXT NOT NULL,
    sha256           TEXT NOT NULL DEFAULT '',
    signature        TEXT NOT NULL DEFAULT '',
    signature_type   TEXT NOT NULL DEFAULT '',
    signature_key_id TEXT NOT NULL DEFAULT '',
    size_bytes       BIGINT NOT NULL DEFAULT 0,
    metadata_json    JSONB,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_global_artifacts_name   ON global_artifacts(name);
CREATE INDEX IF NOT EXISTS idx_global_artifacts_status ON global_artifacts(status);

-- Per-artifact per-region replication tracking.
-- blob_status: pending | replicating | confirmed | failed
CREATE TABLE IF NOT EXISTS artifact_replication_status (
    replication_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id        TEXT NOT NULL REFERENCES global_artifacts(artifact_id) ON DELETE CASCADE,
    plane_id           UUID NOT NULL REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    metadata_pushed_at TIMESTAMPTZ,
    metadata_push_error TEXT NOT NULL DEFAULT '',
    blob_status        TEXT NOT NULL DEFAULT 'pending',
    blob_confirmed_at  TIMESTAMPTZ,
    blob_check_error   TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, plane_id)
);

CREATE INDEX IF NOT EXISTS idx_art_repl_artifact ON artifact_replication_status(artifact_id);
CREATE INDEX IF NOT EXISTS idx_art_repl_plane    ON artifact_replication_status(plane_id);
CREATE INDEX IF NOT EXISTS idx_art_repl_blob     ON artifact_replication_status(blob_status);
