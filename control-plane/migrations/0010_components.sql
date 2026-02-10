ALTER TABLE device_state
  ADD COLUMN IF NOT EXISTS components jsonb;

ALTER TABLE desired_state_group
  ADD COLUMN IF NOT EXISTS components jsonb;

ALTER TABLE desired_state_device
  ADD COLUMN IF NOT EXISTS components jsonb;

ALTER TABLE device_apply_results
  ADD COLUMN IF NOT EXISTS component text;
