CREATE TABLE IF NOT EXISTS auth_vouchers (
  voucher_id uuid PRIMARY KEY,
  token_hash text NOT NULL UNIQUE,
  email text,
  roles jsonb NOT NULL DEFAULT '["viewer"]'::jsonb,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by text,
  used_at timestamptz,
  used_by text,
  revoked boolean NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS auth_vouchers_token_hash_idx ON auth_vouchers (token_hash);
CREATE INDEX IF NOT EXISTS auth_vouchers_expires_idx ON auth_vouchers (expires_at);
