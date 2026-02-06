ALTER TABLE device_state
  ADD COLUMN IF NOT EXISTS last_apply_artifact_id uuid;

ALTER TABLE device_apply_results
  ADD COLUMN IF NOT EXISTS artifact_id uuid;
