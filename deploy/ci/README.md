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
- GitLab helper built into `scripts/ci-exchange-gitlab-workload-identity.sh`
- Jenkins helper built into `scripts/ci-exchange-jenkins-workload-identity.sh`
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

Example GitLab provider:

```json
[
  {
    "name": "gitlab-ci",
    "issuer": "https://gitlab.com",
    "audience": "hardwareops-ci",
    "allowedScopes": ["artifact.publish"],
    "defaultScopes": ["artifact.publish"],
    "ttl": "15m",
    "claimMatches": {
      "project_path": ["my-group/my-project"],
      "ref": ["main"],
      "ref_type": ["branch"]
    }
  }
]
```

GitLab usage:
- use job `id_tokens` to mint an OIDC token into `HWOPS_GITLAB_ID_TOKEN`
- set `HWOPS_WORKLOAD_IDENTITY_AUDIENCE` to the configured audience
- the template falls back to `HWOPS_CI_SERVICE_TOKEN` only if you still provide one

Jenkins usage:
- Jenkins has no single built-in OIDC token shape across installations
- the template/helper expect an external JWT in either:
  - `JENKINS_OIDC_TOKEN`, or
  - `HWOPS_JENKINS_ID_TOKEN`
- this token can come from an OIDC-capable Jenkins plugin or a wrapper stage that requests an upstream identity token
- define a matching provider in `CI_WORKLOAD_IDENTITY_PROVIDERS_JSON/FILE` for that issuer and its claims

Other CI systems can call the same exchange endpoint directly by passing an OIDC token in `CI_WORKLOAD_IDENTITY_TOKEN`.

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
- Optional exchange helpers:
  - `scripts/ci-exchange-workload-identity.sh`
  - `scripts/ci-exchange-gitlab-workload-identity.sh`
  - `scripts/ci-exchange-jenkins-workload-identity.sh`

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
- GitHub/GitLab templates prefer workload identity and only use static CI tokens as fallback.
- Jenkins templates support workload identity if your Jenkins environment injects an OIDC token; otherwise they can still use a static service token.
- For pull with `credentialRef`, configure resolver sources in control-plane:
  - `ARTIFACT_PULL_CREDENTIALS_FILE` / `ARTIFACT_PULL_CREDENTIALS_JSON`
  - or AWS Secrets Manager (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`)
- Local smoke test (push + pull + resolver reload):
  - `AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me ./scripts/test-artifact-ingest.sh`
- See:
  - `docs/development/artifact-ingest.md`
  - `docs/development/push-vs-pull-workflows.md`
