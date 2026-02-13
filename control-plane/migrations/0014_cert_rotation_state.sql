CREATE TABLE IF NOT EXISTS cert_rotation_state (
  id SMALLINT PRIMARY KEY DEFAULT 1,
  active_fingerprint TEXT NOT NULL,
  previous_fingerprint TEXT,
  rotated_at TIMESTAMPTZ NOT NULL,
  grace_period_seconds BIGINT NOT NULL DEFAULT 0,
  cleaned_at TIMESTAMPTZ,
  cleaned_reason TEXT
);
