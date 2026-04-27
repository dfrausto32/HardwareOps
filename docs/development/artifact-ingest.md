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
- Alternative for CI: `POST /api/v1/auth/workload-identity/exchange` exchanges an external OIDC job token for a short-lived Parcel bearer token carrying `artifact.publish`
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
- Workload identity helper scripts:
  - `scripts/ci-exchange-workload-identity.sh`
  - `scripts/ci-exchange-gitlab-workload-identity.sh`
  - `scripts/ci-exchange-jenkins-workload-identity.sh`

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

### Current v1 (trusted key verification)

- CI/packer computes `sha256(artifact.tar.gz)` and signs it with an Ed25519 private key.
- Control-plane stores `signature`, `signatureType`, and `signatureKeyId`.
- Control-plane verifies the detached signature during create/upload/pull/complete against the active trusted key registry.
- Agents receive the active signing trust bundle during enroll/claim/check-in and verify again before apply.

**Why it works:** simple, fast, offline-friendly; no private signing key enters the control-plane or browser.

**What's missing:** no public transparency log; no keyless identity verification; no provenance/attestations in this slice.

### Verification modes

Control-plane managed policy (recommended for production):

| Variable | Effect |
|---|---|
| `ARTIFACT_TRUST_VERIFICATION_MODE=require_verified` | Bootstrap DB/global policy starts strict |
| `ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS=sha256:...,sha256:...` | Bootstrap policy pins allowed signing key IDs |
| `ARTIFACT_TRUST_ALLOWED_SIGNATURE_TYPES=ed25519,cosign` | Bootstrap policy restricts accepted signature formats |
| `ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1` | Bootstrap default becomes `require_verified` |
| `ARTIFACT_SIGNATURE_KEY_ID=sha256:...` | Bootstrap policy pins allowed signing key ID |
| `ARTIFACT_SIGNATURE_ENFORCE_INGEST=1` | Bootstrap default rejects unsigned/untrusted artifacts on create/upload/pull/complete |

Global policy modes:
- `allow_unsigned`
- `warn_unsigned`
- `require_verified`

Agent-side legacy fallback (env):
- `SIGNING_PUB_KEY_PATH=/path/to/ed25519.pub`
- `SIGNING_KEY_ID=sha256:...` — optional key-identity pin when no control-plane trust bundle exists

### Trusted signing key bootstrap sources

The control-plane seeds the trusted key registry on startup from one or more bootstrap sources:

- `TRUSTED_SIGNING_KEYS_FILE=/path/to/trusted-signing-keys.json`
- `TRUSTED_SIGNING_KEYS_JSON='{"keys":[...]}'`
- `TRUSTED_SIGNING_KEYS_AWS_SECRET_ID=parcel/customer/prod/trusted-signing-keys`
- `TRUSTED_SIGNING_KEYS_AWS_REGION=us-east-1`
- `CI_WORKLOAD_IDENTITY_PROVIDERS_FILE=/path/to/workload-identity-providers.json`
- `CI_WORKLOAD_IDENTITY_PROVIDERS_JSON='[{"name":"github-actions","issuer":"https://token.actions.githubusercontent.com","audience":"parcel-ci","claimMatches":{"repository":["my-org/my-repo"]}}]'`
- `CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_SECRET_ID=parcel/customer-a/prod/workload-identity-providers`
- `CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_REGION=us-east-1`

Operational verification:
- admin status endpoint: `GET /api/v1/auth/workload-identity/status`
- smoke helper: `scripts/test-workload-identity.sh`

Supported JSON payload:

```json
{
  "keys": [
    {
      "keyId": "sha256:...",
      "displayName": "release-ed25519",
      "algorithm": "ed25519",
      "publicKeyPem": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----",
      "notes": "primary release signer"
    }
  ]
}
```

Helper:

- `scripts/build-trusted-signing-keys.sh <out-file> <public-key-path>`

### Cosign scope

This slice supports `signatureType=cosign` for key-based offline verification with trusted PEM public keys. It does **not** include:
- Rekor transparency log verification
- Fulcio/keyless identity
- Cosign attestation/provenance workflows

Those stay in the later supply-chain policy track.

---

## Manual Upload Workflow

Manual upload supports detached signature verification and policy-driven unsigned behavior.

- **Unsigned:** Upload via UI or `POST /api/v1/artifacts/upload` with no signature. Accepted only when policy is `allow_unsigned` or `warn_unsigned`.
- **Signed:** Upload through the UI with detached signature file + signature type + signing key ID, or use `scripts/pack-upload-artifact.sh` with `SIGNING_KEY`, `SIGNING_KEY_ID`, and optional `SIGNATURE_TYPE`.

See `docs/development/artifact-trusted-upload-ui.md` for the UI contract and remaining gaps.

---

## Security Baseline

For all ingest paths:
- Always provide and verify `sha256`.
- Use `signature` + `signatureType` + `signatureKeyId` when artifacts are signed.
- Prefer `require_verified` in cloud/hardened deployments.
- Use `warn_unsigned` or `allow_unsigned` only for explicitly disconnected/on-prem environments.
- Seed the trusted key registry from file/JSON/Secrets Manager before switching to strict mode.

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
