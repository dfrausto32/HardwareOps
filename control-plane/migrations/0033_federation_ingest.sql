-- Sidecar table tracking artifacts received via federation push from the global plane.
-- Used by the presign handler to decide whether to serve a local or global MinIO URL.
CREATE TABLE IF NOT EXISTS federation_artifact_ingest (
    ingest_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id            TEXT NOT NULL UNIQUE,
    global_object_key      TEXT NOT NULL DEFAULT '',
    global_presign_base_url TEXT NOT NULL DEFAULT '',
    blob_confirmed         BOOLEAN NOT NULL DEFAULT FALSE,
    blob_confirmed_at      TIMESTAMPTZ,
    received_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fed_ingest_artifact  ON federation_artifact_ingest(artifact_id);
CREATE INDEX IF NOT EXISTS idx_fed_ingest_confirmed ON federation_artifact_ingest(blob_confirmed);
