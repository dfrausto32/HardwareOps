# Artifact Ingest

Three ingest paths are supported. Any combination can be enabled.

---

## Ingest Modes

### 1) Manual Upload (UI/API)

**Use when:** small teams, manual release flow.
- UI upload
- API upload: `POST /api/v1/artifacts/upload`

Pros: simple, no CI dependencies.
Cons: human-driven, limited audit automation.

### 2) CI Push (Presigned)

**Use when:** automated pipelines in CI/CD.

Flow:
1. CI requests presigned upload URL from control-plane.
2. CI uploads artifact directly to object store.
3. CI completes artifact registration with metadata/signature.

Pros:
- No long-lived object store credentials in CI.
- Strong audit trail (tokenized API calls + artifact metadata).
- Scales cleanly across customer environments.

Cons: requires pipeline wiring.

### 3) Control-Plane Pull (Artifactory / HTTP)

**Use when:** artifacts already live in an external registry or blob store.

Flow:
1. Operator/automation posts metadata + `sourceUrl` + checksum.
2. Control-plane downloads artifact from source URL.
3. Control-plane verifies checksum and ingests artifact.

Pros:
- Fast onboarding for existing repos; no need to rewrite publish steps.
- Control-plane can ingest from Artifactory and generic HTTP sources.

Cons:
- Control-plane needs egress to source repository.
- Larger SSRF surface than push; mitigate with `ARTIFACT_PULL_ALLOWED_HOSTS`.

---

## When to Use Which Path

| Scenario | Recommended path |
|---|---|
| New deployment | CI Push |
| Migrating existing repo flow | Pull first, then migrate to Push |
| Strict security / compliance | CI Push |
| Disconnected CI but reachable control-plane egress | Pull |
| Manual / import / one-off | Manual Upload |

**Operational recommendation:** use CI Push as the steady-state production default. Keep Pull enabled for migration and special cases.

---

## CI Push API

### Endpoints

1. `POST /api/v1/artifacts/presign-upload`
   - Returns `artifactId`, `objectKey`, `uploadUrl`, `expiresAt`
2. CI uploads bytes directly with `PUT <uploadUrl>`
3. `POST /api/v1/artifacts/complete`
   - Finalizes artifact metadata after server-side object validation (`size` + `sha256`)

### Auth

Create scoped service tokens via `POST /api/v1/auth/service-tokens` (admin).
- Scope for v1: `artifact.publish`
- Tokens support break-glass revoke/rotate (`POST /api/v1/auth/service-tokens/{tokenId}/revoke|rotate`) with operator reason capture.

### Helper script

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

CI provider scaffold templates (GitHub Actions, GitLab CI, Jenkins): `../../deploy/ci/README.md`

---

## Pull API

### Endpoint

`POST /api/v1/artifacts/pull`

Required fields:
- `name`
- `version`
- `sha256`

Source fields (backward compatible):
- Legacy: `sourceUrl` (http/https)
- Adapter-ready: `source.kind` + `source.uri` (+ optional `source.credentialRef`)
- If both are sent, they must refer to the same URI.

Supported source kinds: `http`, `artifactory`

Optional fields: `type`, `sizeBytes`, `signature`, `signatureKeyId`, `metadata`

Server behavior:
- Fetches from resolved source URI
- Enforces size limit and timeout
- Verifies `sha256` (and `sizeBytes` when provided)
- Stores in object store and creates artifact metadata

### Pull safety controls

| Variable | Default | Notes |
|---|---|---|
| `ARTIFACT_PULL_ALLOWED_HOSTS` | (unset) | Required in hardened profiles; comma-separated host allowlist |
| `ARTIFACT_PULL_MAX_BYTES` | `1073741824` | 1 GiB max artifact size |
| `ARTIFACT_PULL_TIMEOUT` | `15m` | Max download time |
| `ARTIFACT_PULL_ALLOW_INSECURE_HTTP` | `0` | Set to `1` only for controlled local/dev scenarios |

Adapter guardrails:
- `http://` sources are rejected unless `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=1`.
- Loopback/private/link-local targets are blocked unless the host is explicitly allowlisted.
- Hardened profile requires non-empty `ARTIFACT_PULL_ALLOWED_HOSTS` and `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0`.

### Credential resolver

| Variable | Notes |
|---|---|
| `ARTIFACT_PULL_CREDENTIALS_FILE` | Path to JSON credentials file |
| `ARTIFACT_PULL_CREDENTIALS_JSON` | Inline JSON (used when file is unset) |
| `ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID` | AWS Secrets Manager secret containing the same JSON map |
| `ARTIFACT_PULL_CREDENTIALS_AWS_REGION` | Optional region override |

Load order: static first, AWS secret second (AWS wins on collisions).

After rotating pull credentials, reload without restart:
```bash
POST /api/v1/artifacts/pull-credentials/reload
# or
scripts/reload-pull-credentials.sh
```

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

Artifactory-specific credential aliases:
- `artifactory_token` or `jfrog_access_token` → `Authorization: Bearer <token>`
- `artifactory_api_key` or `jfrog_api_key` → `X-JFrog-Art-Api: <key>`

Pull helper script: `scripts/ci-pull-artifact.sh`

Local Artifactory smoke test:
- `./scripts/setup-artifactory-demo.sh`
- `./scripts/test-artifactory-adapter.sh`

---

## Signing Model

### Current v1 (Ed25519, offline)

- CI/packer computes `sha256(artifact.tar.gz)` and signs it with an Ed25519 private key.
- The signature and key ID are stored in artifact metadata (`signature`, `signatureKeyId`).
- The agent verifies the signature before apply.

**Why it works:** simple, fast, offline-friendly; minimal key material and no external service dependency.

**What's missing:** no public transparency log; no keyless identity verification.

### Signature enforcement

Control-plane managed policy (recommended for production):

| Variable | Effect |
|---|---|
| `ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1` | Adds `applyPolicy.requireSignature=true` to desired components when not explicitly set |
| `ARTIFACT_SIGNATURE_KEY_ID=sha256:...` | Injects pinned key ID into desired `applyPolicy` when not explicitly set |
| `ARTIFACT_SIGNATURE_ENFORCE_INGEST=1` | Rejects unsigned artifacts (and wrong key ID) on create/upload/pull/complete |

Agent-side enforcement (env):
- `REQUIRE_ARTIFACT_SIGNATURE=1` — hard-fail unsigned artifacts
- `SIGNING_PUB_KEY_PATH=/path/to/ed25519.pub`
- `SIGNING_KEY_ID=sha256:...` — optional key-identity pin

### Future path: Cosign/Sigstore

To extend to Cosign without breaking existing artifacts, add `signatureType` and optional `cosignBundle` to artifact metadata:
- `signatureType`: `ed25519` | `cosign`
- `signature`: base64 for Ed25519
- `signatureKeyId`: key identifier for Ed25519
- `cosignBundle`: JSON (optional), for Cosign verification

This keeps Ed25519 as the v1 default while letting Cosign signing be adopted incrementally in CI. Consider Cosign when you need:
- Audit-grade transparency log (Rekor)
- Keyless signing tied to enterprise identity (Fulcio)
- SLSA provenance / SBOM attestations

---

## Manual Upload Workflow

Manual upload does not change unless signature verification is enforced.

- **Unsigned (default):** Upload via UI or `POST /api/v1/artifacts/upload` with no signature.
- **Signed:** Use `scripts/pack-upload-artifact.sh` with `SIGNING_KEY` + `SIGNING_KEY_ID` to attach a signature.

---

## Security Baseline

For all ingest paths:
- Always provide and verify `sha256`.
- Use `signature` + `signatureKeyId` when artifacts are signed.

For pull:
- Set `ARTIFACT_PULL_ALLOWED_HOSTS` explicitly.
- Keep `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0`.
- Keep `ARTIFACT_PULL_MAX_BYTES` tight to expected artifact size.
- Use short `ARTIFACT_PULL_TIMEOUT`.
- Avoid loopback/private source hosts unless explicitly allowlisted.
- Prefer CI Push for steady-state production; keep Pull for migration and import.

For CI:
- Prefer presigned URLs over long-lived object store credentials.
- Store repo pull credentials in Secrets Manager or Vault.

---

## Related

- Test plan: `artifact-ingest-test-plan.md`
- CI templates: `../../deploy/ci/README.md`
- Push helper: `scripts/ci-upload-artifact.sh`
- Pull helper: `scripts/ci-pull-artifact.sh`
