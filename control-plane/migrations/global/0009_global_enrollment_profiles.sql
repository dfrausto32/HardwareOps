-- Global enrollment profiles and pending enrollment cache for E5.
-- Profiles are stored on the global plane and eventually pushed to regional planes.
-- Pending enrollments are polled from regional planes and cached here for unified approval.

CREATE TABLE IF NOT EXISTS global_enrollment_profiles (
    profile_id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 TEXT        NOT NULL,
    require_approval     BOOLEAN     NOT NULL DEFAULT TRUE,
    allow_untrusted_hw   BOOLEAN     NOT NULL DEFAULT FALSE,
    challenge_hint       TEXT        NOT NULL DEFAULT '',
    approval_delay_sec   INT         NOT NULL DEFAULT 0,
    max_uses             INT         NOT NULL DEFAULT 0,
    cert_validity_days   INT         NOT NULL DEFAULT 365,
    default_labels_json  JSONB       NOT NULL DEFAULT '{}',
    disabled             BOOLEAN     NOT NULL DEFAULT FALSE,
    created_by           TEXT        NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- global_pending_enrollments_cache stores pending enrollments polled from all
-- regional planes. Rows are replaced wholesale per plane on each reconciler tick.
CREATE TABLE IF NOT EXISTS global_pending_enrollments_cache (
    cache_id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    plane_id              UUID        NOT NULL REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    request_id            TEXT        NOT NULL,
    profile_id            TEXT        NOT NULL DEFAULT '',
    status                TEXT        NOT NULL DEFAULT 'pending',
    source_ip             TEXT        NOT NULL DEFAULT '',
    agent_version         TEXT        NOT NULL DEFAULT '',
    hardware_id           TEXT        NOT NULL DEFAULT '',
    metadata_json         JSONB       NOT NULL DEFAULT '{}',
    capabilities_json     JSONB       NOT NULL DEFAULT '{}',
    denied_reason         TEXT        NOT NULL DEFAULT '',
    expires_at            TIMESTAMPTZ,
    approval_available_at TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (plane_id, request_id)
);

CREATE INDEX IF NOT EXISTS idx_gpec_plane_id ON global_pending_enrollments_cache(plane_id);
CREATE INDEX IF NOT EXISTS idx_gpec_status   ON global_pending_enrollments_cache(status);
