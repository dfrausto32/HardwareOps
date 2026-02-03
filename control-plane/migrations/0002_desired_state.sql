CREATE TABLE IF NOT EXISTS desired_state_group (
  group_id uuid PRIMARY KEY,
  artifact_id uuid,
  desired_version text,
  desired_config_rev text,
  policy jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS desired_state_device (
  device_id uuid PRIMARY KEY,
  artifact_id uuid,
  desired_version text,
  desired_config_rev text,
  policy jsonb,
  source text NOT NULL DEFAULT 'manual',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS desired_state_device_source_idx ON desired_state_device(source);
