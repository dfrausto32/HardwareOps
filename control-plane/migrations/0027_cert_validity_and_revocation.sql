-- B2: configurable cert validity per enrollment profile + cert serial blocklist for immediate revocation.

-- Add cert_validity_days to enrollment_profiles.
-- 0 = use server default (90 days). Values > 0 override the default.
ALTER TABLE enrollment_profiles ADD COLUMN IF NOT EXISTS cert_validity_days INT NOT NULL DEFAULT 0;

-- Add cert_serial to devices to enable revocation by serial on decommission.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS cert_serial TEXT NOT NULL DEFAULT '';

-- Cert serial blocklist: checked at mTLS authentication to block decommissioned devices
-- even if the TLS certificate is still cryptographically valid.
CREATE TABLE IF NOT EXISTS cert_revoked_serials (
    serial      TEXT        NOT NULL PRIMARY KEY,
    device_id   TEXT        NOT NULL,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- C8: default max_uses to 1 (single-use) for new enrollment profiles.
-- Existing unlimited profiles (max_uses = 0) are unaffected.
ALTER TABLE enrollment_profiles ALTER COLUMN max_uses SET DEFAULT 1;
