# CI Workflows

HardwareOps supports two CI integration patterns:

1. **Push** - CI uploads the artifact to HardwareOps
2. **Pull** - HardwareOps pulls the artifact from a repository URL

For both workflows, CI can authenticate with:
- a static HardwareOps service token, or
- workload identity exchange using an external OIDC job token

## 1. Push workflow

Used when CI should publish the built artifact directly.

Flow:
1. package the artifact
2. optionally sign it
3. request presigned upload from HardwareOps
4. upload to object storage
5. complete artifact registration

Helper:

```bash
./ci-helpers/ci-upload-artifact.sh
```

## 2. Pull workflow

Used when the built artifact already exists in Artifactory or another repository.

Flow:
1. CI tells HardwareOps the artifact metadata and source
2. HardwareOps downloads the artifact
3. checksum and trust policy are enforced during registration

Helper:

```bash
./ci-helpers/ci-pull-artifact.sh
```

## 3. Workload identity

Recommended for connected CI systems.

Instead of storing a long-lived `HWOPS_CI_SERVICE_TOKEN`, the CI job presents its own OIDC token and exchanges it for a short-lived HardwareOps token.

Exchange endpoint:

```text
POST /api/v1/auth/workload-identity/exchange
```

Scope today:
- `artifact.publish`

## 4. GitHub Actions

Use the included template:

```text
ci-templates/github-actions-push.yml
ci-templates/github-actions-pull.yml
```

Requirements:
- workflow permission `id-token: write`
- matching workload identity provider configured in the control-plane

## 5. GitLab CI

Use the included template:

```text
ci-templates/gitlab-ci-push.yml
ci-templates/gitlab-ci-pull.yml
```

Requirements:
- GitLab job `id_tokens`
- matching workload identity provider configured in the control-plane

## 6. Jenkins

Use the included template:

```text
ci-templates/Jenkinsfile.push
ci-templates/Jenkinsfile.pull
```

Helper scripts shipped with the bundle:

```text
ci-helpers/ci-exchange-workload-identity.sh
ci-helpers/ci-exchange-gitlab-workload-identity.sh
ci-helpers/ci-exchange-jenkins-workload-identity.sh
ci-helpers/ci-upload-artifact.sh
ci-helpers/ci-pull-artifact.sh
```

Requirements:
- Jenkins must supply an external OIDC token in:
  - `JENKINS_OIDC_TOKEN`, or
  - `HWOPS_JENKINS_ID_TOKEN`

If Jenkins cannot provide an OIDC token yet, it can still use a static service token as fallback.

## 7. Artifact signing

Recommended production posture:
- sign artifacts in CI before ingest
- configure trusted signing keys in HardwareOps
- require verified artifacts for controlled environments

## 8. When to use push vs pull

Use **push** when:
- CI owns the built artifact output
- direct upload is simpler than repository integration

Use **pull** when:
- the artifact already lives in Artifactory or another approved repository
- you want HardwareOps to ingest from the repository of record
