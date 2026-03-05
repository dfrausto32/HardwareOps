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

## Ingest Selection Guide
- See `development/push-vs-pull-workflows.md` for operator guidance on when to choose push vs pull.
- See `development/artifact-ingest-test-plan.md` for an end-to-end test plan.

## CI Push API (implemented)
1) `POST /api/v1/artifacts/presign-upload`
   - Returns `artifactId`, `objectKey`, `uploadUrl`, `expiresAt`
2) CI uploads bytes directly with `PUT <uploadUrl>`
3) `POST /api/v1/artifacts/complete`
   - Finalizes artifact metadata after server-side object validation (`size` + `sha256`)

### CI auth (implemented)
- Create scoped service tokens via:
  - `POST /api/v1/auth/service-tokens` (admin)
- Scope for v1:
  - `artifact.publish`
- Service tokens are expiring and support break-glass revoke/rotate workflows (`POST /api/v1/auth/service-tokens/{tokenId}/revoke|rotate`) with operator-authenticated reason capture.

### CI helper script (implemented)
Use:
`scripts/ci-upload-artifact.sh`

Provider scaffolds:
`../../deploy/ci/README.md`

Example:
```bash
ARTIFACT_NAME=agent \
ARTIFACT_VERSION=1.2.3 \
ARTIFACT_TYPE=agent_bundle \
INPUT_DIR=./dist/agent \
CI_SERVICE_TOKEN='<token>' \
BASE_URL=https://control-plane.example.com \
CA_CERT_PATH=./ca.crt \
./scripts/ci-upload-artifact.sh
```

## Pull API (implemented)
`POST /api/v1/artifacts/pull`

Required fields:
- `name`
- `version`
- `sha256`

Source fields (backward compatible):
- Legacy: `sourceUrl` (http/https)
- Adapter-ready: `source.kind` + `source.uri` (+ optional `source.credentialRef`)
- If both are sent, they must refer to the same URI.

Supported source kinds (current):
- `http`
- `artifactory`

Optional fields:
- `type`
- `sizeBytes`
- `signature`
- `signatureKeyId`
- `metadata`

Server behavior:
- Fetches from resolved source URI (`source.uri` or `sourceUrl`)
- Enforces size limit and timeout
- Verifies `sha256` (and `sizeBytes` when provided)
- Stores in object store and creates artifact metadata

Pull safety controls:
- `ARTIFACT_PULL_ALLOWED_HOSTS` (comma-separated host allowlist; use explicit hosts in production)
- `ARTIFACT_PULL_MAX_BYTES` (default `1073741824`, 1 GiB)
- `ARTIFACT_PULL_TIMEOUT` (default `15m`)
- `ARTIFACT_PULL_ALLOW_INSECURE_HTTP` (default `0`; blocks `http://` sources unless explicitly enabled)

Adapter guardrails:
- `http://` pull sources are rejected unless `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=1`.
- Loopback/private/link-local targets are blocked unless the host is explicitly allowlisted.
- Hardened profile requires a non-empty `ARTIFACT_PULL_ALLOWED_HOSTS` and `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0`.

Pull helper script:
`scripts/ci-pull-artifact.sh`

Credential resolver (adapter-ready):
- `ARTIFACT_PULL_CREDENTIALS_FILE` (path to JSON file)
- `ARTIFACT_PULL_CREDENTIALS_JSON` (inline JSON; used when file is unset)
- `ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID` (AWS Secrets Manager secret containing the same JSON map)
- `ARTIFACT_PULL_CREDENTIALS_AWS_REGION` (optional region override for the AWS secret lookup)
- AWS resolver execution requirement: control-plane runtime must have AWS credentials/role permission for `secretsmanager:GetSecretValue` on the configured secret.

Resolver behavior:
- Static credentials (`FILE`/`JSON`) and AWS secret credentials are both loaded when configured.
- If the same `credentialRef` exists in multiple sources, later-loaded sources override earlier values.
- Current load order: static first, AWS secret second (AWS wins on collisions).

Operational reload workflow (admin, no control-plane restart):
- `GET /api/v1/artifacts/pull-credentials` (current resolver status)
- `POST /api/v1/artifacts/pull-credentials/reload` (reload from configured sources)
- Helper script: `scripts/reload-pull-credentials.sh`

Static credential JSON format:
```json
{
  "repo-a": {
    "authorization": "Bearer <token>"
  },
  "repo-basic": {
    "username": "user",
    "password": "pass"
  },
  "repo-headers": {
    "header:X-Api-Key": "abc123"
  }
}
```

Artifactory-specific credential aliases (same static credential map):
- `artifactory_token` or `jfrog_access_token` → `Authorization: Bearer <token>`
- `artifactory_api_key` or `jfrog_api_key` → `X-JFrog-Art-Api: <key>`

Local Artifactory adapter smoke test:
- `./scripts/setup-artifactory-demo.sh`
- `./scripts/test-artifactory-adapter.sh`

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

**Control-plane managed policy (recommended for production):**
- `ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1` adds `applyPolicy.requireSignature=true` to desired components when not explicitly set.
- `ARTIFACT_SIGNATURE_KEY_ID=sha256:...` injects pinned key ID into desired `applyPolicy` when not explicitly set.
- `ARTIFACT_SIGNATURE_ENFORCE_INGEST=1` rejects unsigned artifacts (and wrong key ID when pinned) on create/upload/pull/complete APIs.
