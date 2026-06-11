-- VEX object key and FDA lifecycle end-of-support date on artifacts (Phase G3).
ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS vex_object_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS eos_date       date;

-- Manual exploitability assertions per CVE/component pair.
CREATE TABLE IF NOT EXISTS vex_assertions (
    assertion_id    text PRIMARY KEY,
    artifact_id     uuid NOT NULL REFERENCES artifacts(artifact_id) ON DELETE CASCADE,
    cve_id          text NOT NULL,
    component_name  text NOT NULL DEFAULT '',
    assertion       text NOT NULL
                        CHECK (assertion IN ('affected', 'not_affected', 'under_investigation', 'fixed')),
    justification   text NOT NULL DEFAULT '',
    actor_user_id   text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, cve_id, component_name)
);

CREATE INDEX IF NOT EXISTS idx_vex_assertions_artifact_id ON vex_assertions (artifact_id);
