-- SBOM (Software Bill of Materials) object key for each artifact.
-- Populated asynchronously after artifact ingest by the SBOM generator.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS sbom_object_key text;
