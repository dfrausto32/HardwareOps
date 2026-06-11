-- HIPAA audit hardening: PHI touch flag and minimum-necessary access classification (Phase G4).
ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS phi_touched        boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS minimum_necessary  text    NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_events_phi_touched ON audit_events (phi_touched)
    WHERE phi_touched = true;
