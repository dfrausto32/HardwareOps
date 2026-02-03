ALTER TABLE device_state
  ADD COLUMN IF NOT EXISTS last_apply_status text,
  ADD COLUMN IF NOT EXISTS last_apply_error text,
  ADD COLUMN IF NOT EXISTS last_apply_at timestamptz;

CREATE TABLE IF NOT EXISTS device_apply_results (
  apply_id uuid PRIMARY KEY,
  device_id uuid NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
  status text NOT NULL,
  applied_version text,
  applied_config_rev text,
  error text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS device_apply_results_device_id_idx ON device_apply_results(device_id);
