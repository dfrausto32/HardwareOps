-- Per-(group, plane) policy push tracking for E4 sync reconciler.
-- Lets the reconciler detect missed fan-outs and re-push with backoff.

CREATE TABLE IF NOT EXISTS global_policy_sync_status (
    sync_id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id        UUID        NOT NULL REFERENCES global_groups(group_id) ON DELETE CASCADE,
    plane_id        UUID        NOT NULL REFERENCES regional_planes(plane_id) ON DELETE CASCADE,
    pushed_at       TIMESTAMPTZ,
    push_error      TEXT        NOT NULL DEFAULT '',
    retry_count     INT         NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, plane_id)
);

CREATE INDEX IF NOT EXISTS idx_gpss_plane_id ON global_policy_sync_status(plane_id);
CREATE INDEX IF NOT EXISTS idx_gpss_group_id ON global_policy_sync_status(group_id);
