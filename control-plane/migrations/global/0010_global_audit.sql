CREATE TABLE IF NOT EXISTS audit_events (
  event_id      TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor_type    TEXT NOT NULL DEFAULT '',
  actor_id      TEXT NOT NULL DEFAULT '',
  actor_email   TEXT NOT NULL DEFAULT '',
  actor_roles   JSONB NOT NULL DEFAULT '[]',
  auth_method   TEXT NOT NULL DEFAULT '',
  source_ip     TEXT NOT NULL DEFAULT '',
  user_agent    TEXT NOT NULL DEFAULT '',
  request_id    TEXT NOT NULL DEFAULT '',
  action        TEXT NOT NULL,
  target_type   TEXT NOT NULL DEFAULT '',
  target_id     TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'success',
  error         TEXT NOT NULL DEFAULT '',
  before_json   JSONB,
  after_json    JSONB,
  metadata_json JSONB
);

CREATE INDEX IF NOT EXISTS audit_events_occurred_at_idx ON audit_events (occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_action_idx      ON audit_events (action);
CREATE INDEX IF NOT EXISTS audit_events_target_idx      ON audit_events (target_type, target_id);
CREATE INDEX IF NOT EXISTS audit_events_actor_idx       ON audit_events (actor_id);
