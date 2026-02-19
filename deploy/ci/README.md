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
- `HWOPS_CI_SERVICE_TOKEN` (scope: `artifact.publish`)
- Optional TLS trust:
  - `HWOPS_CA_CERT_B64` (base64-encoded CA cert)
  - or set insecure mode in non-production tests

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

- Service tokens should be short-lived and rotated.
- For pull with `credentialRef`, configure resolver sources in control-plane:
  - `ARTIFACT_PULL_CREDENTIALS_FILE` / `ARTIFACT_PULL_CREDENTIALS_JSON`
  - or AWS Secrets Manager (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`)
- Local smoke test (push + pull + resolver reload):
  - `AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me ./scripts/test-artifact-ingest.sh`
- See:
  - `docs/development/artifact-ingest.md`
  - `docs/development/push-vs-pull-workflows.md`
