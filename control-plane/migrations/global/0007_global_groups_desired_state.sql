-- Global groups and desired state policies.
-- Operators create groups with label selectors on the global plane and
-- assign desired artifact versions to them. The global plane pushes these
-- policies to all registered regional planes via the federation API.

CREATE TABLE IF NOT EXISTS global_groups (
    group_id    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    selector_json JSONB     NOT NULL DEFAULT '{}',
    created_by  TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS global_desired_state (
    group_id          UUID        PRIMARY KEY REFERENCES global_groups(group_id) ON DELETE CASCADE,
    artifact_id       TEXT        NOT NULL DEFAULT '',
    desired_version   TEXT        NOT NULL DEFAULT '',
    desired_config_rev TEXT       NOT NULL DEFAULT '',
    policy_json       JSONB       NOT NULL DEFAULT '{}',
    components_json   JSONB       NOT NULL DEFAULT '{}',
    checkin_interval  INT         NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by        TEXT        NOT NULL DEFAULT ''
);
