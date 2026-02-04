ALTER TABLE device_state
  ADD COLUMN IF NOT EXISTS last_preapply_status text,
  ADD COLUMN IF NOT EXISTS last_preapply_error text,
  ADD COLUMN IF NOT EXISTS last_preapply_at timestamptz;

ALTER TABLE device_apply_results
  ADD COLUMN IF NOT EXISTS preapply_status text,
  ADD COLUMN IF NOT EXISTS preapply_error text;
