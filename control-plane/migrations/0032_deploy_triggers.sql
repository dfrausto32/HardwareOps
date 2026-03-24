CREATE TABLE IF NOT EXISTS deploy_triggers (
    device_id    TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    triggered_by TEXT NOT NULL DEFAULT '',
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (device_id)
);
