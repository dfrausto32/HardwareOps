CREATE TABLE IF NOT EXISTS enrollment_profiles (
  profile_id uuid PRIMARY KEY,
  name text NOT NULL,
  token_hash text UNIQUE NOT NULL,
  require_approval boolean NOT NULL DEFAULT true,
  allow_untrusted_hw boolean NOT NULL DEFAULT false,
  max_uses integer NOT NULL DEFAULT 0,
  uses integer NOT NULL DEFAULT 0,
  expires_at timestamptz,
  default_labels jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by text,
  disabled boolean NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS enrollment_profiles_expires_idx
  ON enrollment_profiles (expires_at);

CREATE TABLE IF NOT EXISTS pending_enrollments (
  request_id uuid PRIMARY KEY,
  profile_id uuid NOT NULL REFERENCES enrollment_profiles(profile_id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'pending',
  csr text NOT NULL,
  claim_token_hash text UNIQUE NOT NULL,
  capabilities jsonb,
  metadata jsonb,
  source_ip text,
  user_agent text,
  agent_version text,
  hardware_id text,
  denied_reason text,
  expires_at timestamptz NOT NULL,
  approved_at timestamptz,
  approved_by_user_id uuid,
  denied_at timestamptz,
  denied_by_user_id uuid,
  issued_at timestamptz,
  issued_device_id uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pending_enrollments_status_created_idx
  ON pending_enrollments (status, created_at DESC);

CREATE INDEX IF NOT EXISTS pending_enrollments_hardware_pending_idx
  ON pending_enrollments (hardware_id)
  WHERE status = 'pending' AND hardware_id IS NOT NULL;
