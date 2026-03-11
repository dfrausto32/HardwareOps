ALTER TABLE artifacts
  ADD COLUMN IF NOT EXISTS signature_type text,
  ADD COLUMN IF NOT EXISTS signature_key_id text,
  ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'legacy',
  ADD COLUMN IF NOT EXISTS verification_error text,
  ADD COLUMN IF NOT EXISTS verified_at timestamptz;

UPDATE artifacts
SET verification_status = 'legacy'
WHERE verification_status IS NULL OR verification_status = '';

CREATE INDEX IF NOT EXISTS artifacts_verification_status_idx ON artifacts(verification_status);
CREATE INDEX IF NOT EXISTS artifacts_signature_key_id_idx ON artifacts(signature_key_id);

CREATE TABLE IF NOT EXISTS trusted_signing_keys (
  key_id text PRIMARY KEY,
  display_name text NOT NULL,
  algorithm text NOT NULL,
  public_key_pem text NOT NULL,
  state text NOT NULL DEFAULT 'active',
  created_at timestamptz NOT NULL DEFAULT now(),
  retired_at timestamptz,
  notes text
);

CREATE INDEX IF NOT EXISTS trusted_signing_keys_state_idx ON trusted_signing_keys(state);

CREATE TABLE IF NOT EXISTS artifact_trust_policy (
  policy_id int PRIMARY KEY CHECK (policy_id = 1),
  verification_mode text NOT NULL,
  allowed_signing_key_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
  allowed_signature_types jsonb NOT NULL DEFAULT '[]'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by_user_id text
);
