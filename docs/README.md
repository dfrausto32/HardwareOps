# Documentation Map

Everything in `docs/` is indexed here. Pick the section for what you're trying to do.

> **Layout:** `guides/` (deploy & operate) · `reference/` (features + API) · `product/` (vision & sales) · `compliance/` (status, policies) · `customer/` (customer-facing bundle) · `playbooks/` · `testing/` · `incidents/` · `development/` (design & internal) · `archive/` (historical).

---

## Start here (canonical)

| Doc | What it covers |
|---|---|
| `guides/deploy.md` | End-to-end deployment (on-prem + AWS + hardening baseline) — the deployment entry point |
| `guides/operations.md` | Day-2 ops: backup/restore, upgrades, cert rotation, metrics |
| `guides/local-dev-wsl.md` | Local WSL dev workflow + pre-apply demo walkthrough |
| `reference/icd.md` | Integration Control Document: the living API contract + headless workflows |
| `development/roadmap.md` | Authoritative implementation roadmap and phase status |

---

## Guides — deploy & operate

| Doc | What it covers |
|---|---|
| `guides/deploy.md` | Canonical deployment guide; routes to the docs below |
| `guides/installers.md` | Bundle build/layout + fresh-machine install runbook |
| `guides/deployment-hardening.md` | Proxy trust allowlist, on-prem anti-tamper, device identity policy |
| `guides/operations.md` | Day-2 operations runbook |
| `guides/backup-restore.md` | Backup and restore runbook (Postgres + MinIO) |
| `guides/local-dev-wsl.md` | Local dev (WSL2) |
| `guides/vm-testing.md` | VM-based install validation |
| `development/aws-cloud-setup-plan.md` | AWS deployment shapes + architecture baseline |
| `development/aws-customer-deployment-runbook.md` | Step-by-step AWS per-customer deploy + hardening |
| `../deploy/aws/terraform/README.md` | Terraform scaffold module details |
| `../deploy/compose/README.md` | Docker Compose dev/on-prem stack quickstarts |

---

## Reference — features & API

| Doc | What it covers |
|---|---|
| `reference/icd.md` | Full API contract + headless workflows |
| `reference/certs.md` | CA/TLS setup and CA rotation |
| `reference/agent-systemd.md` | Systemd agent install, enrollment, approval-mode runbook |
| `reference/ldap-auth.md` | LDAP/AD authentication provider |
| `reference/global-plane.md` | Global-plane (federation) operator runbook |
| `reference/global-desired-state.md` | Global groups + policy fan-out to regions |
| `reference/artifact-federation.md` | Global artifact upload + replication tracking |
| `reference/artifact-provenance.md` | Cosign/Sigstore signatures, attestations, provenance policy |
| `reference/cloud-pull-adapters.md` | S3/GCS pull adapters + Vault credential backend |
| `reference/vulnerability-scanning.md` | Artifact (Grype/Trivy) + device (Nessus) scanning |
| `reference/email-delivery.md` | SMTP for password reset + invites |
| `reference/dns-coredns.md` | CoreDNS for on-prem internal DNS |
| `../deploy/ci/README.md` | CI provider templates (GitHub Actions/GitLab/Jenkins) |

---

## Customer-facing (installer bundle source)

| Doc | What it covers |
|---|---|
| `customer/README.md` | Index to the customer documentation bundle |
| `customer/setup-onprem.md` / `customer/setup-vendor-hosted-aws.md` | Customer setup by deployment model |
| `customer/first-agent-onboarding.md` | Customer agent install + approval onboarding |
| `customer/operations.md` | Customer day-2 operations |
| `customer/security-and-recovery.md` | Artifact trust, password recovery, audit expectations |
| `customer/ci-workflows.md` | Customer CI integration (push/pull, workload identity) |

---

## Product & sales

| Doc | What it covers |
|---|---|
| `product/whitepaper.md` | Technical white paper |
| `product/pitch-short.md` / `product/pitch-long.md` | 1-page and 2-page pitches |

---

## Compliance & policies

| Doc | What it covers |
|---|---|
| `compliance/compliance-status.md` | Compliance landscape vs. platform capabilities |
| `compliance/export-compliance.md` | EAR self-classification + obligations |
| `compliance/risk-register.md` | Live risk register |
| `compliance/policies/` | Formal policies (access control, backup/recovery, change mgmt, incident response, ransomware, risk assessment, vendor mgmt, vuln mgmt) |

---

## Testing & validation

| Doc | What it covers |
|---|---|
| `testing/prod-docker-lab.md` | Production-like on-prem validation stack (Docker) |
| `testing/prod-docker-test-plan.md` | P0/P1/P2 release-gate checklist for that lab |
| `testing/aws-e2e-test-plan.md` | Manual AWS end-to-end validation runbook |
| `testing/firmware-ota-hardware-test-plan.md` | Hardware procurement + bare-metal OTA test cases (Phase F) |
| `development/handoff-e2e-testing.md` | Automated full-stack E2E suite status + how to run/extend (`../scripts/e2e-suite.sh`) |
| `incidents/` | Incident notes + live validation failures needing follow-up |

> Manual labs above are complementary to the **automated** CI E2E gate (`../scripts/e2e-suite.sh`); see the handoff doc. Associated lab scripts: `../scripts/testing/`.

---

## Playbooks (operational scenarios)

| Doc | What it covers |
|---|---|
| `playbooks/managed-aws-hosted.md` | Vendor-managed AWS deployment runbook |
| `playbooks/customer-self-hosted-aws.md` | Customer-owned AWS deployment runbook |
| `playbooks/iso-fleet-provisioning.md` | Bulk agent provisioning from a golden image |

---

## Development (design & internal)

| Doc | What it covers |
|---|---|
| `development/roadmap.md` | Source of truth for status + what's next |
| `development/documentation-audit.md` | 2026-06-04 docs audit + 2026-06-07 reorg execution |
| `development/bare-metal-firmware-ota.md` | Phase F engineering design: gateway/BLE/MCU firmware OTA |
| `development/agent-launch-runbook.md` | Multi-agent worktree runbook (interactive + headless modes) |
| `development/handoff-e2e-testing.md` | E2E pipeline + testing initiative hand-off |
| `development/phase-c-internals.md` | LDAP, Vault, provenance, cloud-adapter internals |
| `development/auth-secrets-v1.md` | Local-user auth design (OIDC/LDAP/Vault now shipped) |
| `development/license.md` | Signed license + device-cap enforcement |
| `development/artifact-ingest.md` / `development/artifact-ingest-test-plan.md` | Ingest model + test plan |
| `development/artifact-apply-roadmap.md` | Forward plan: firmware + container image apply |
| `development/artifact-trusted-upload-ui.md` | Trusted artifact upload UI model |
| `development/agent-first-contact-onboarding.md` | First-contact approval flow design |
| `development/metrics-health.md` | Metrics catalog + health model |
| `development/upgrade-strategy.md` | Staged apply + rollback contract |
| `development/security-hardening.md` | AWS hardening tracker (post-implementation) |

---

## Archive (historical — not current)

| Doc | Why it's here |
|---|---|
| `archive/Parcel_Update_Application_Design_v1.md` | Original v1 design concept; superseded by the implementation |
| `archive/scalability-security-audit-2026-03-15.md` | Point-in-time audit; findings largely addressed in Phase D |

---

## Helpful scripts

| Script | What it does |
|---|---|
| `../scripts/e2e-suite.sh` | Full-stack automated E2E suite (control-plane + global-plane + agent over TLS/mTLS) |
| `../scripts/multi-app-artifacts.sh` | Build/upload multi-component demo artifacts |
| `../scripts/aws-demo-image.sh` / `../scripts/aws-demo-seed.sh` | Demo-agent image to ECR / seed signed demo artifacts |
| `../scripts/testing/prod-lab-*.sh` | Init/up/seed/smoke/first-contact/down for the prod-like Docker lab |
| `../scripts/test-artifact-ingest.sh` | Push/pull ingest smoke test |
| `../scripts/test-workload-identity.sh` | Validate workload identity providers + optional OIDC exchange |
| `../scripts/assemble-customer-docs.sh` | Assemble the customer docs bundle |
| `../scripts/reload-pull-credentials.sh` | Reload pull credential resolver without restart |
| `../scripts/setup-artifactory-demo.sh` / `../scripts/test-artifactory-adapter.sh` | Artifactory pull adapter setup + test |
| `../scripts/aws-hardening-check.sh` | AWS hardening config + deployment gate helper |
