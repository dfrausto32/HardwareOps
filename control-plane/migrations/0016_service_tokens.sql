CREATE TABLE IF NOT EXISTS service_tokens (
  token_id uuid PRIMARY KEY,
  name text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by text,
  last_used_at timestamptz,
  revoked_at timestamptz,
  revoked_by text
);

CREATE INDEX IF NOT EXISTS service_tokens_token_hash_idx ON service_tokens (token_hash);
CREATE INDEX IF NOT EXISTS service_tokens_expires_idx ON service_tokens (expires_at);
