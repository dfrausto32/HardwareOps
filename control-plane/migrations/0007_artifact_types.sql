ALTER TABLE artifacts
  ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT 'app_bundle',
  ADD COLUMN IF NOT EXISTS metadata jsonb;

CREATE INDEX IF NOT EXISTS artifacts_type_idx ON artifacts(type);
