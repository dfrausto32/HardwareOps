CREATE TABLE IF NOT EXISTS audit_events (
  event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  occurred_at timestamptz NOT NULL DEFAULT now(),
  actor_type text,
  actor_id text,
  actor_email text,
  actor_roles jsonb,
  auth_method text,
  source_ip text,
  user_agent text,
  request_id text,
  action text NOT NULL,
  target_type text,
  target_id text,
  status text NOT NULL,
  error text,
  before jsonb,
  after jsonb,
  metadata jsonb
);

CREATE INDEX IF NOT EXISTS audit_events_occurred_at_idx
  ON audit_events (occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_action_idx
  ON audit_events (action);
CREATE INDEX IF NOT EXISTS audit_events_target_idx
  ON audit_events (target_type, target_id);
CREATE INDEX IF NOT EXISTS audit_events_actor_idx
  ON audit_events (actor_id);
