CREATE TABLE IF NOT EXISTS artifacts (
  artifact_id uuid PRIMARY KEY,
  name text NOT NULL,
  version text NOT NULL,
  object_key text NOT NULL,
  sha256 text NOT NULL,
  signature text,
  size_bytes bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS artifacts_name_version_idx ON artifacts(name, version);
