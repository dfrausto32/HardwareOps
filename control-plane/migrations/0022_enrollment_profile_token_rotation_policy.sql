ALTER TABLE enrollment_profiles
  ADD COLUMN IF NOT EXISTS previous_token_hash text,
  ADD COLUMN IF NOT EXISTS previous_token_expires_at timestamptz,
  ADD COLUMN IF NOT EXISTS token_rotated_at timestamptz;

CREATE INDEX IF NOT EXISTS enrollment_profiles_previous_token_hash_idx
  ON enrollment_profiles (previous_token_hash)
  WHERE previous_token_hash IS NOT NULL;
