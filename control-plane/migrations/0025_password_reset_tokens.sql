CREATE TABLE IF NOT EXISTS password_reset_tokens (
  token_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users (user_id) ON DELETE CASCADE,
  token_hash text UNIQUE NOT NULL,
  delivery_mode text NOT NULL DEFAULT 'operator',
  reason text,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  issued_by_user_id uuid REFERENCES users (user_id) ON DELETE SET NULL,
  used_at timestamptz
);

CREATE INDEX IF NOT EXISTS password_reset_tokens_user_id_idx
  ON password_reset_tokens (user_id);
