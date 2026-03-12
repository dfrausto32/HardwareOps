# CI Templates (Scaffold)

These templates provide a starting point for CI-driven artifact ingest:

- **Push workflow** (recommended default): presigned upload + complete
- **Pull workflow** (repo-backed): control-plane fetches source URI

They are scaffolds, not final production pipelines. Adjust build/sign stages, branch rules, and approval gates for your environment.

## Files

- `templates/github-actions-push.yml`
- `templates/github-actions-pull.yml`
- `templates/gitlab-ci-push.yml`
- `templates/gitlab-ci-pull.yml`
- `templates/Jenkinsfile.push`
- `templates/Jenkinsfile.pull`

## Shared required inputs

- `HWOPS_BASE_URL` (example: `https://app.customer.example.com`)
- Either:
  - `HWOPS_CI_SERVICE_TOKEN` (scope: `artifact.publish`), or
  - workload identity exchange via `scripts/ci-exchange-workload-identity.sh`
- Optional TLS trust:
  - `HWOPS_CA_CERT_B64` (base64-encoded CA cert)
  - or set insecure mode in non-production tests

## Workload identity (Phase C)

The control-plane can exchange an external OIDC job token for a short-lived HardwareOps bearer token.

Current shipped path:
- GitHub Actions helper built into `scripts/ci-exchange-workload-identity.sh`
- generic OIDC provider config on control-plane
- exchanged token carries short-lived `artifact.publish` scope only

Control-plane configuration:
- `CI_WORKLOAD_IDENTITY_PROVIDERS_JSON`
- or `CI_WORKLOAD_IDENTITY_PROVIDERS_FILE`

Example GitHub Actions provider:

```json
[
  {
    "name": "github-actions",
    "issuer": "https://token.actions.githubusercontent.com",
    "audience": "hardwareops-ci",
    "allowedScopes": ["artifact.publish"],
    "defaultScopes": ["artifact.publish"],
    "ttl": "15m",
    "claimMatches": {
      "repository": ["my-org/my-repo"],
      "ref": ["refs/heads/main"],
      "workflow_ref": ["my-org/my-repo/.github/workflows/release.yml@refs/heads/main"]
    }
  }
]
```

GitHub Actions usage:
- grant workflow permission: `id-token: write`
- set `CI_WORKLOAD_IDENTITY_AUDIENCE` to the configured audience
- do not pass a static CI token unless you want fallback behavior

Other CI systems can call the same exchange endpoint by passing an OIDC token in `CI_WORKLOAD_IDENTITY_TOKEN`.

## Push workflow inputs

- `ARTIFACT_NAME`
- `ARTIFACT_VERSION`
- `ARTIFACT_TYPE` (default `app_bundle`)
- `INPUT_DIR` (directory to package/upload)
- Optional:
  - `METADATA_JSON`
  - `SIGNING_KEY` + `SIGNING_KEY_ID` (if signing in pipeline)

Implementation helper:
- `scripts/ci-upload-artifact.sh`

## Pull workflow inputs

- `ARTIFACT_NAME`
- `ARTIFACT_VERSION`
- `ARTIFACT_SHA256`
- Source:
  - legacy: `SOURCE_URL`
  - adapter-ready: `SOURCE_KIND` + `SOURCE_URI` (+ optional `SOURCE_CREDENTIAL_REF`)
- Optional:
  - `ARTIFACT_TYPE`
  - `ARTIFACT_SIZE_BYTES`
  - `METADATA_JSON`

Implementation helper:
- `scripts/ci-pull-artifact.sh`

## Notes

- Service tokens should be short-lived and rotated when used.
- Workload identity tokens are exchanged on demand and do not require long-lived secret files in CI.
- For pull with `credentialRef`, configure resolver sources in control-plane:
  - `ARTIFACT_PULL_CREDENTIALS_FILE` / `ARTIFACT_PULL_CREDENTIALS_JSON`
  - or AWS Secrets Manager (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`)
- Local smoke test (push + pull + resolver reload):
  - `AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me ./scripts/test-artifact-ingest.sh`
- See:
  - `docs/development/artifact-ingest.md`
  - `docs/development/push-vs-pull-workflows.md`
