ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled boolean NOT NULL DEFAULT false;
