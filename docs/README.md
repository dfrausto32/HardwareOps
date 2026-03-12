# Documentation Map

Start here. Use the canonical docs first; go to deep dives only for topic-specific detail.

---

## Canonical docs (start here)

| Doc | What it covers |
|---|---|
| `deploy.md` | End-to-end deployment guide (on-prem + AWS + hardening baseline) |
| `operations.md` | Day-2 operations: backup/restore, upgrades, cert rotation, metrics |
| `local-dev-wsl.md` | Local WSL dev workflow + pre-apply demo walkthrough |
| `icd.md` | Living Integration Control Document: API contract + headless workflows |
| `customer/` | Customer-facing documentation bundle source used in installer outputs |
| `development/roadmap.md` | Implementation roadmap and phase status |

---

## Deployment deep dives

| Doc | What it covers |
|---|---|
| `installers.md` | Bundle build/layout + fresh-machine install runbook (control-plane + agent + upgrades) |
| `deployment-hardening.md` | Proxy trust allowlist, on-prem anti-tamper controls, device identity policy |
| `dns-coredns.md` | CoreDNS setup for on-prem internal DNS |
| `vm-testing.md` | VM-based testing flow |
| `development/aws-cloud-setup-plan.md` | Deployment shapes, AWS architecture baseline, implementation sequence |
| `development/aws-customer-deployment-runbook.md` | Step-by-step AWS per-customer deploy and hardening runbook |
| `../deploy/aws/terraform/README.md` | Terraform scaffold module details |

---

## Operations deep dives

| Doc | What it covers |
|---|---|
| `backup-restore.md` | Backup and restore runbook |
| `certs.md` | CA/TLS setup and CA rotation |
| `development/upgrade-strategy.md` | Staged apply and rollback strategy |
| `development/metrics-health.md` | Metrics catalog and health model |

---

## Validation labs

| Doc | What it covers |
|---|---|
| `testing/prod-docker-lab.md` | Production-like on-prem validation stack using Docker |
| `testing/prod-docker-test-plan.md` | Feature + edge + security validation checklist for that lab |

Associated scripts: `../scripts/testing/`

---

## Platform and integration references

| Doc | What it covers |
|---|---|
| `agent-systemd.md` | Systemd agent install/enrollment and approval-mode runbook |
| `development/artifact-ingest.md` | Ingest modes (manual / CI push / pull), signing model, security baseline |
| `development/artifact-ingest-test-plan.md` | Ingest test plan |
| `development/artifact-apply-roadmap.md` | Future artifact apply: firmware and container image planning |
| `development/agent-first-contact-onboarding.md` | First-contact approval flow: full design and implementation detail |
| `development/auth-secrets-v1.md` | Auth model design: local users, JWT, roles, OIDC extension points |
| `development/license.md` | Signed license and device-cap enforcement |
| `development/security-hardening.md` | AWS hardening tracker: acceptance evidence and residual known items |
| `../deploy/ci/README.md` | CI provider templates (GitHub Actions/GitLab/Jenkins) for artifact push/pull |

---

## Helpful scripts

| Script | What it does |
|---|---|
| `../scripts/multi-app-artifacts.sh` | Build/upload multi-component demo artifacts |
| `../scripts/aws-demo-image.sh` | Build/push demo-agent image to ECR |
| `../scripts/aws-demo-seed.sh` | Seed signed demo artifacts in an AWS stack |
| `../scripts/testing/prod-lab-init.sh` | Initialize hardened production-like Docker lab |
| `../scripts/testing/prod-lab-up.sh` | Start production-like Docker lab |
| `../scripts/testing/prod-lab-seed.sh` | Seed demo devices and signed artifacts in prod-like lab |
| `../scripts/testing/prod-lab-smoke.sh` | Smoke test lab health/auth/metrics |
| `../scripts/testing/prod-lab-first-contact.sh` | End-to-end first-contact approval flow validation |
| `../scripts/testing/prod-lab-down.sh` | Stop/wipe production-like Docker lab |
| `../scripts/test-artifact-ingest.sh` | Push/pull ingest smoke test |
| `../scripts/test-workload-identity.sh` | Validate configured workload identity providers and optional OIDC exchange |
| `../scripts/build-customer-docs-bundle.sh` | Assemble a zip-ready customer docs bundle |
| `../scripts/reload-pull-credentials.sh` | Reload pull credential resolver without restarting control-plane |
| `../scripts/setup-artifactory-demo.sh` | Local Artifactory setup for pull adapter tests |
| `../scripts/test-artifactory-adapter.sh` | End-to-end Artifactory pull adapter test |
| `../scripts/aws-hardening-check.sh` | AWS hardening config and deployment gate helper |
