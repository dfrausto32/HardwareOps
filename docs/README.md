# Documentation Map

Use these canonical docs first:

- `deploy.md` - end-to-end deployment guide (on-prem + AWS + hardening baseline).
- `operations.md` - day-2 operations (backup/restore, upgrades, cert rotation, metrics checks).
- `local-dev-wsl.md` - local WSL dev workflow.
- `development/roadmap.md` - implementation roadmap and phase status.

Rule of thumb:
- Start in the canonical docs.
- Use deep dives only for topic-specific details.
- Prefer linking to canonical docs instead of duplicating step-by-step flow.

---

## Deployment deep dives

- `installer-flow.md` - fresh machine install via installer bundles.
- `installers.md` - installer bundle contents and build details.
- `deployment-hardening.md` - proxy trust allowlist and on-prem anti-tamper controls.
- `development/aws-customer-deployment-runbook.md` - expanded AWS per-customer runbook.
- `development/aws-cloud-setup-plan.md` - AWS strategy and architecture decisions.
- `../deploy/aws/terraform/README.md` - Terraform scaffold details.
- `vm-testing.md` - VM testing flow.

## Operations deep dives

- `backup-restore.md` - backup and restore runbook.
- `certs.md` - CA/TLS setup and CA rotation details.
- `development/upgrade-strategy.md` - staged apply and rollback strategy.
- `development/metrics-health.md` - metrics catalog and health model.

## Product + platform references

- `agent-systemd.md` - systemd agent install/enrollment.
- `artifact-apply-roadmap.md` - artifact apply scope by type.
- `preapply-demo.md` - demo apply behavior and verification.
- `development/artifact-ingest.md` - ingest model (push + pull).
- `development/push-vs-pull-workflows.md` - workflow choice guidance.
- `development/artifact-ingest-test-plan.md` - ingest test plan.
- `../deploy/ci/README.md` - CI provider templates (GitHub/GitLab/Jenkins) for push/pull ingest.
- `development/artifact-signing.md` - signing model and migration path.
- `development/auth-secrets-v1.md` - local auth/secrets baseline.
- `development/license.md` - signed license and device cap enforcement.
- `development/device-identity-hardening.md` - anti-clone device identity policy.
- `development/security-hardening.md` - security hardening plan.

## Helpful scripts

- `../scripts/multi-app-artifacts.sh` - build/upload multi-component demo artifacts.
- `../scripts/aws-demo-image.sh` - build/push demo-agent image to ECR.
- `../scripts/aws-demo-seed.sh` - seed signed demo artifacts in AWS stack.
- `../scripts/test-artifact-ingest.sh` - push/pull ingest smoke test.
- `../scripts/reload-pull-credentials.sh` - reload pull credential resolver from configured sources without restarting control-plane.
- `../scripts/setup-artifactory-demo.sh` - local Artifactory setup for pull adapter tests.
- `../scripts/test-artifactory-adapter.sh` - end-to-end Artifactory pull adapter test.
