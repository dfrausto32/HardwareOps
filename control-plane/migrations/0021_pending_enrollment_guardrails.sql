ALTER TABLE enrollment_profiles
  ADD COLUMN IF NOT EXISTS challenge_hash text,
  ADD COLUMN IF NOT EXISTS challenge_hint text,
  ADD COLUMN IF NOT EXISTS approval_delay_seconds integer NOT NULL DEFAULT 0;

ALTER TABLE pending_enrollments
  ADD COLUMN IF NOT EXISTS approval_available_at timestamptz;

UPDATE pending_enrollments
SET approval_available_at = created_at
WHERE approval_available_at IS NULL;

CREATE INDEX IF NOT EXISTS pending_enrollments_active_profile_idx
  ON pending_enrollments (profile_id, created_at DESC)
  WHERE status IN ('pending', 'approved');

CREATE INDEX IF NOT EXISTS pending_enrollments_active_source_idx
  ON pending_enrollments (source_ip, created_at DESC)
  WHERE status IN ('pending', 'approved') AND source_ip IS NOT NULL;
