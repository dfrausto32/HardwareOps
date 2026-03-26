-- Regional plane cache of global desired state policies.
-- Written by POST /api/v1/federation/policies (called by the global plane).
-- Read during device checkin as lowest-priority fallback when no local group matches.

CREATE TABLE IF NOT EXISTS global_policy_cache (
    cache_id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id          UUID        NOT NULL,
    group_name        TEXT        NOT NULL DEFAULT '',
    selector_json     JSONB       NOT NULL DEFAULT '{}',
    artifact_id       TEXT        NOT NULL DEFAULT '',
    desired_version   TEXT        NOT NULL DEFAULT '',
    desired_config_rev TEXT       NOT NULL DEFAULT '',
    policy_json       JSONB       NOT NULL DEFAULT '{}',
    components_json   JSONB       NOT NULL DEFAULT '{}',
    checkin_interval  INT         NOT NULL DEFAULT 0,
    received_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id)
);
