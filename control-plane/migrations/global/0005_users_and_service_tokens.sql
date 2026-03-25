-- Minimal user table for global-plane local auth.
CREATE TABLE IF NOT EXISTS users (
    user_id       TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL DEFAULT '',
    roles         TEXT NOT NULL DEFAULT 'viewer',
    disabled      BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Minimal service token table for global-plane token auth.
CREATE TABLE IF NOT EXISTS service_tokens (
    token_id       TEXT PRIMARY KEY,
    name           TEXT NOT NULL DEFAULT '',
    token_hash     TEXT NOT NULL UNIQUE,
    scopes         TEXT NOT NULL DEFAULT '',
    disabled       BOOLEAN NOT NULL DEFAULT FALSE,
    last_used_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
