CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS devices (
  device_id uuid PRIMARY KEY,
  cert_fingerprint text UNIQUE,
  status text NOT NULL DEFAULT 'active',
  last_seen timestamptz,
  labels jsonb,
  metadata jsonb
);

CREATE TABLE IF NOT EXISTS device_state (
  device_id uuid PRIMARY KEY REFERENCES devices(device_id) ON DELETE CASCADE,
  current_version text,
  current_config_rev text,
  services jsonb,
  health jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS enrollment_tokens (
  token_id uuid PRIMARY KEY,
  token_hash text UNIQUE NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS devices_last_seen_idx ON devices(last_seen);
