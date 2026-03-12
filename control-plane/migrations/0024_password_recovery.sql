ALTER TABLE users
  ADD COLUMN IF NOT EXISTS recovery_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS recovery_codes_generated_at timestamptz;
