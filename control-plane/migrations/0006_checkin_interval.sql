ALTER TABLE desired_state_group
  ADD COLUMN IF NOT EXISTS checkin_interval_sec integer;

ALTER TABLE desired_state_device
  ADD COLUMN IF NOT EXISTS checkin_interval_sec integer;
