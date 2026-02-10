CREATE TABLE IF NOT EXISTS users (
  user_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text UNIQUE NOT NULL,
  display_name text,
  password_hash text NOT NULL,
  roles jsonb NOT NULL DEFAULT '[]'::jsonb,
  disabled boolean NOT NULL DEFAULT false,
  auth_provider text NOT NULL DEFAULT 'local',
  external_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz
);

CREATE INDEX IF NOT EXISTS users_email_idx ON users (email);
