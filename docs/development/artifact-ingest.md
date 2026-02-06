# Artifact Ingest Modes

We support **three ingest paths** and allow customers to enable any combination.
The control‑plane should be configured with which modes are allowed.

## Modes

### 1) Manual Upload (UI/API)
**Use when:** small teams, manual release flow.
- UI upload (current behavior)
- API upload (`POST /api/v1/artifacts/upload`)

Pros: simple, no CI dependencies.  
Cons: human‑driven, limited audit automation.

### 2) CI Push (Presigned)
**Use when:** automated pipelines in CI/CD.
- Control‑plane issues **presigned upload** URL
- CI uploads artifact, then registers metadata

Pros: secure automation, no direct S3 credentials needed.  
Cons: requires pipeline wiring.

### 3) Repo/Blob Pull (Artifactory/S3)
**Use when:** artifacts already live in a registry or blob.
- Control‑plane **pulls** artifact from configured repo
- Validates checksum + metadata
- Stores in object store (optional caching)

Pros: integrates with existing artifact systems.  
Cons: needs credentials + network access.

## Config (future)
We’ll expose a config to enable any subset:
- `ARTIFACT_INGEST_MODES=manual,push,pull`
- `DEFAULT_INGEST_MODE=manual`

### Repo Pull Config (examples)
- `ARTIFACT_REPO_TYPE=artifactory|s3|gcs|http`
- `ARTIFACT_REPO_URL=...`
- `ARTIFACT_REPO_CREDENTIALS=...` (stored in secrets manager)

## Security Notes
- Prefer **presigned URLs** for CI.
- Store repo credentials in **Secrets Manager/Vault**.
- Support artifact **signing + verification**.
  - **Signature format:** Ed25519 signature over the artifact SHA256.
  - **Storage:** signature stored in `artifacts.signature`; key ID stored in `metadata.signatureKeyId`.
  - **Pipeline:** packer/CI generates signature and passes it on upload.

## Manual Upload Workflow (Signed vs Unsigned)
Manual upload **does not change** unless signature verification is enforced on agents.

- **Unsigned (default):** Upload via UI or `POST /api/v1/artifacts/upload` with no signature.
- **Signed:** Use `scripts/pack-upload-artifact.sh` with `SIGNING_KEY` + `SIGNING_KEY_ID` to attach a signature.

**Agent enforcement (optional):**
- Set `REQUIRE_ARTIFACT_SIGNATURE=1` to hard‑fail unsigned artifacts.
- Set `SIGNING_PUB_KEY_PATH=/path/to/ed25519.pub` so the agent can verify.
- Optionally pin `SIGNING_KEY_ID=sha256:...` to enforce key identity.
