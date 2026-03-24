-- Artifact attestations: in-toto / SLSA provenance records attached to artifacts.
CREATE TABLE IF NOT EXISTS artifact_attestations (
    attestation_id   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id      uuid        NOT NULL
                                 REFERENCES artifacts(artifact_id)
                                 ON DELETE CASCADE,
    predicate_type   text        NOT NULL,
    payload_json     jsonb       NOT NULL,
    signature        text,
    signature_type   text,
    signature_key_id text,
    builder_id       text,
    builder_issuer   text,
    verified_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS artifact_attestations_artifact_id_idx
    ON artifact_attestations(artifact_id);

CREATE INDEX IF NOT EXISTS artifact_attestations_builder_id_idx
    ON artifact_attestations(builder_id)
    WHERE builder_id IS NOT NULL;

-- Provenance policy column on the global trust policy table.
ALTER TABLE artifact_trust_policy
    ADD COLUMN IF NOT EXISTS provenance_policy jsonb;
