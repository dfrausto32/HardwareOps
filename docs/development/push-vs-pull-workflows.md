# Push vs Pull Artifact Workflows

This guide explains when to use **CI Push** vs **Control-Plane Pull** for artifact ingest.

## Summary
- Use **Push** when you own CI and want the most secure, auditable default.
- Use **Pull** when artifacts already exist in an external repo and duplicating upload logic is not practical.

## CI Push (Recommended Default)
Flow:
1. CI requests presigned upload URL from control-plane.
2. CI uploads artifact directly to object store.
3. CI completes artifact registration with metadata/signature.

Endpoints:
- `POST /api/v1/artifacts/presign-upload`
- `POST /api/v1/artifacts/complete`

Best for:
- GitHub Actions / Bitbucket Pipelines / GitLab CI
- Signed release pipelines
- Strict least-privilege access

Pros:
- No long-lived object store credentials in CI.
- Strong audit trail (tokenized API calls + artifact metadata).
- Scales cleanly across customer environments.

Cons:
- Requires CI job updates.

## Control-Plane Pull
Flow:
1. Operator/automation posts metadata + `sourceUrl` + checksum.
2. Control-plane downloads artifact from source URL.
3. Control-plane verifies checksum and ingests artifact.

Endpoint:
- `POST /api/v1/artifacts/pull`
- Source shape:
  - Legacy: `sourceUrl`
  - Adapter-ready: `source.kind` + `source.uri` (+ optional `source.credentialRef`)
  - Current supported kinds: `http`, `artifactory`
- Optional static credential resolver:
  - `ARTIFACT_PULL_CREDENTIALS_FILE` or `ARTIFACT_PULL_CREDENTIALS_JSON`

Best for:
- Existing artifact repositories (Artifactory/object storage) where artifact already exists
- Manual migration/import scenarios
- Environments where CI cannot reach control-plane directly

Pros:
- Fast onboarding for existing repos.
- No need to rewrite existing build publish steps immediately.

Cons:
- Control-plane needs egress to source repository.
- Larger SSRF/external-dependency surface than push (mitigate with allowlist).

## Decision Matrix
- **New deployment**: Start with **Push**.
- **Migrating existing repo flow**: Start with **Pull**, then move to Push over time.
- **Strict security/compliance**: Prefer **Push**.
- **Disconnected CI but reachable control-plane egress**: **Pull** can be acceptable.

## Security Baseline
For both workflows:
- Always provide and verify `sha256`.
- Use artifact signatures (`signature` + `signatureKeyId`) when available.

For pull workflow:
- Set `ARTIFACT_PULL_ALLOWED_HOSTS`.
- Keep `ARTIFACT_PULL_MAX_BYTES` tight to expected artifact size.
- Use short `ARTIFACT_PULL_TIMEOUT`.
- After rotating pull credentials, run:
  - `POST /api/v1/artifacts/pull-credentials/reload`
  - or `scripts/reload-pull-credentials.sh`

## Operational Recommendation
- Use **Push** as the production default for steady-state releases.
- Keep **Pull** enabled for migration/import and special cases.

## CI templates
- Provider scaffold templates (GitHub Actions, GitLab CI, Jenkins):
  `../../deploy/ci/README.md`
- Push helper script:
  `scripts/ci-upload-artifact.sh`
- Pull helper script:
  `scripts/ci-pull-artifact.sh`
