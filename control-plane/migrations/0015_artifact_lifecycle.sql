ALTER TABLE artifacts
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS deprecated_at timestamptz,
  ADD COLUMN IF NOT EXISTS delete_after timestamptz;

CREATE INDEX IF NOT EXISTS artifacts_status_idx ON artifacts(status);
CREATE INDEX IF NOT EXISTS artifacts_delete_after_idx ON artifacts(delete_after);

CREATE TABLE IF NOT EXISTS artifact_lifecycle_policy (
  policy_id int PRIMARY KEY CHECK (policy_id = 1),
  deprecated_delete_after_days int NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO artifact_lifecycle_policy (policy_id, deprecated_delete_after_days)
VALUES (1, 30)
ON CONFLICT (policy_id) DO NOTHING;
