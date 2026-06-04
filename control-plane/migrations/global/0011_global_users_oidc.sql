-- Add SSO identity fields to the global-plane users table.
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_id   TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name  TEXT NOT NULL DEFAULT '';

-- Unique index for external SSO identities.
CREATE UNIQUE INDEX IF NOT EXISTS users_external_id_uniq
    ON users (auth_provider, external_id)
    WHERE external_id IS NOT NULL;
