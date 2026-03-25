CREATE TABLE IF NOT EXISTS regional_planes (
    plane_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT NOT NULL UNIQUE,
    base_url              TEXT NOT NULL,
    encrypted_token       BYTEA NOT NULL,
    tls_ca_pem            TEXT NOT NULL DEFAULT '',
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    sync_interval_seconds INT NOT NULL DEFAULT 60,
    last_sync_at          TIMESTAMPTZ,
    last_sync_error       TEXT NOT NULL DEFAULT '',
    created_by            TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
