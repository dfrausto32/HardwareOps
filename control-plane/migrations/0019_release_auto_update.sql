CREATE TABLE IF NOT EXISTS release_auto_update_settings (
  settings_id int PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  allow_unsigned boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by_user_id text
);
