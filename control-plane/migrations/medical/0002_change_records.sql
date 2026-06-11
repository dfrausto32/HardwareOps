-- Medical migration 0002: IEC 62304 change control records.
-- Applied only when DEPLOYMENT_PROFILE=medical.

CREATE TABLE IF NOT EXISTS iec62304_change_records (
    record_id            text PRIMARY KEY,
    artifact_id          text NOT NULL REFERENCES artifacts(artifact_id) ON DELETE CASCADE,
    safety_class         text NOT NULL CHECK (safety_class IN ('ClassA', 'ClassB', 'ClassC')),
    impact_summary       text NOT NULL DEFAULT '',
    risk_controls        text NOT NULL DEFAULT '',
    -- State machine: draft → pending_approval → approved | rejected
    status               text NOT NULL DEFAULT 'draft'
                             CHECK (status IN ('draft', 'pending_approval', 'approved', 'rejected')),
    created_by_user_id   text NOT NULL DEFAULT '',
    updated_at           timestamptz NOT NULL DEFAULT now(),
    created_at           timestamptz NOT NULL DEFAULT now(),
    approved_by_user_id  text NOT NULL DEFAULT '',
    approved_at          timestamptz,
    rejected_by_user_id  text NOT NULL DEFAULT '',
    rejected_at          timestamptz,
    rejected_reason      text NOT NULL DEFAULT '',
    UNIQUE (artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_iec62304_change_records_artifact_id
    ON iec62304_change_records (artifact_id);
