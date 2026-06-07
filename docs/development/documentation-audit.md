# Documentation Audit — 2026-06-04

> **Update (2026-06-07):** the reorg recommended below was **executed** on `main` — `docs/` root reorganized into `guides/` `reference/` `product/` `compliance/` `archive/`, `docs/README.md` rebuilt as a complete audience-routed index, and `docs/development/README.md` refreshed. The inventory paths below reflect the *pre-reorg* layout and are kept as the record of what moved where.

**Scope:** every Markdown doc in the repo (~80 files). **Method:** read/skim each, cross-checked against `docs/development/roadmap.md` (phase status + Recently Completed) and the current code. **Decision basis:** historical/superseded docs are recommended for **archive (not delete)** per the owner's call.

> **Headline:** the docs are in better shape than they feel. The "disorganized" perception is mostly a **discoverability** problem — the index (`docs/README.md`) is stale and lists only ~half the docs — not widespread duplication or rot. Deployment docs look sprawling but actually form a clean tiered set (canonical → detail → customer-simplified). Only **2 files are genuinely historical**; a few need a one-line scope/status banner. **No file moves were made** (audit only).

## Do-this-first action list (small, high-leverage)

1. ✅ **Rebuild `docs/README.md`** as a *complete*, audience-routed index. Done 2026-06-07.
2. ✅ **Refresh `docs/development/README.md`** — add missing docs. Done 2026-06-07.
3. ✅ **Archive 2 historical docs** → `docs/archive/` with a "historical — superseded by X" banner:
   - `Parcel_Update_Application_Design_v1.md` (was at repo root; superseded by the implementation + roadmap)
   - `docs/scalability-security-audit-2026-03-15.md` (point-in-time audit; most findings overtaken by Phase D hardening)
4. **Add a status banner** to `docs/development/security-hardening.md` — it's a dated AWS-hardening tracker whose items largely shipped in Phase D; mark "post-implementation; residual items tracked in roadmap."
5. **Light content updates (keep in place):**
   - `docs/development/auth-secrets-v1.md` — add a note that OIDC/LDAP/Vault are now shipped (Phase C); link to current auth references.
   - `docs/development/artifact-ingest.md` — clarify push/pull terminology now that S3/GCS/Artifactory adapters shipped.

Items 4 and 5 are the only remaining open actions from this audit.

## Executed reorg (2026-06-07)

The following moves were made from the flat `docs/` root:

**→ `docs/guides/`:** `deploy.md`, `installers.md`, `deployment-hardening.md`, `operations.md`, `backup-restore.md`, `local-dev-wsl.md`, `vm-testing.md`

**→ `docs/reference/`:** `icd.md`, `certs.md`, `agent-systemd.md`, `ldap-auth.md`, `global-plane.md`, `global-desired-state.md`, `artifact-federation.md`, `artifact-provenance.md`, `cloud-pull-adapters.md`, `vulnerability-scanning.md`, `email-delivery.md`, `dns-coredns.md`

**→ `docs/product/`:** `whitepaper.md`, `pitch-short.md`, `pitch-long.md`

**→ `docs/compliance/`:** `compliance-status.md`, `export-compliance.md`, `risk-register.md`, `policies/`

**→ `docs/archive/`:** `scalability-security-audit-2026-03-15.md`, `Parcel_Update_Application_Design_v1.md` (from repo root)

All cross-references updated via `sed` across `.md` and `.sh` files in the same commit. `git mv` used throughout to preserve history.

## Proposed target structure (executed)

- `docs/reference/` — feature/platform references + API: `icd.md`, `certs.md`, `ldap-auth.md`, `global-plane.md`, `global-desired-state.md`, `artifact-federation.md`, `artifact-provenance.md`, `cloud-pull-adapters.md`, `vulnerability-scanning.md`, `email-delivery.md`, `agent-systemd.md`, `dns-coredns.md`
- `docs/product/` — `whitepaper.md`, `pitch-short.md`, `pitch-long.md`
- `docs/compliance/` — `compliance-status.md`, `export-compliance.md`, `risk-register.md` (+ `policies/`)
- `docs/guides/` — `deploy.md`, `installers.md`, `deployment-hardening.md`, `operations.md`, `backup-restore.md`, `local-dev-wsl.md`, `vm-testing.md`
- `docs/archive/` — historical (see action 3)

---

## Full inventory (pre-reorg paths)

**Status legend:** `current` = accurate, keep · `update` = keep but edit · `archive` = historical → `docs/archive/` · `dup` = overlaps, consider merge.

### Root & meta
| Doc | Audience | Status | Action |
|---|---|---|---|
| `README.md` | developer | current | keep |
| `CLAUDE.md` | developer/AI | current | keep (depends on `security@parcel.io` inbox — roadmap queue) |
| `SECURITY.md` | security/customer | current | keep |
| `Parcel_Update_Application_Design_v1.md` | architect | **archive** | original v1 design; moved to `docs/archive/` |
| `agent/README.md`, `cli/README.md`, `control-plane/README.md` | developer | current | keep (stubs) |

### docs/ — feature & platform references + API
| Doc | Audience | Status | Action |
|---|---|---|---|
| `icd.md` | developer | current | keep (authoritative API contract) |
| `certs.md` | operator | current | keep |
| `ldap-auth.md` | operator | current | keep |
| `global-plane.md` | operator/dev | current | keep |
| `global-desired-state.md` | operator/dev | current | keep |
| `artifact-federation.md` | operator/dev | current | keep |
| `artifact-provenance.md` | dev/operator | current | keep |
| `cloud-pull-adapters.md` | operator/dev | current | keep |
| `vulnerability-scanning.md` | operator/dev | current | keep |
| `email-delivery.md` | operator | current | keep |
| `agent-systemd.md` | operator | current | keep |
| `dns-coredns.md` | operator | current | keep |

### docs/ — deployment & operations
| Doc | Audience | Status | Action |
|---|---|---|---|
| `deploy.md` | operator/dev | current | keep (**canonical** deployment entry point) |
| `installers.md` | operator/dev | current | keep |
| `deployment-hardening.md` | operator/compliance | current | keep |
| `operations.md` | operator | current | keep |
| `backup-restore.md` | operator | current | keep |
| `vm-testing.md` | developer | current | keep |
| `local-dev-wsl.md` | developer | current | keep |

### docs/ — sales & compliance
| Doc | Audience | Status | Action |
|---|---|---|---|
| `whitepaper.md`, `pitch-short.md`, `pitch-long.md` | sales/customer | current | keep |
| `compliance-status.md`, `export-compliance.md`, `risk-register.md` | compliance | current | keep |
| `scalability-security-audit-2026-03-15.md` | dev/architect | **archive** | point-in-time; moved to `docs/archive/` |

### docs/development/
| Doc | Audience | Status | Action |
|---|---|---|---|
| `roadmap.md` | internal | current | keep (source of truth) |
| `handoff-e2e-testing.md` | internal | current | keep (added to dev index) |
| `phase-c-internals.md` | developer | current | keep (added to dev index) |
| `artifact-trusted-upload-ui.md` | dev/operator | current | keep (added to dev index) |
| `metrics-health.md`, `upgrade-strategy.md`, `license.md` | operator/dev | current | keep |
| `agent-first-contact-onboarding.md` | dev/operator | current | keep |
| `aws-cloud-setup-plan.md`, `aws-customer-deployment-runbook.md` | operator | current | keep |
| `artifact-ingest.md` | dev/operator | **update** | clarify multi-adapter push/pull terminology |
| `artifact-ingest-test-plan.md` | operator/dev | current | keep |
| `artifact-apply-roadmap.md` | developer | current | keep |
| `auth-secrets-v1.md` | developer | **update** | note OIDC/LDAP/Vault now shipped; link current auth refs |
| `security-hardening.md` | operator | **update** | add "post-implementation" status banner |
| `README.md` | internal | **update** | refreshed 2026-06-07 |

### docs/customer/ (intentional simplifications — keep all)
`README.md`, `operations.md`, `security-and-recovery.md`, `first-agent-onboarding.md`, `setup-vendor-hosted-aws.md`, `setup-onprem.md`, `ci-workflows.md` — all **current**.

### docs/policies/ → docs/compliance/policies/ (keep all)
All 8 policy files — **current** (v1.0, 2026-04). Moved to `compliance/policies/` in 2026-06-07 reorg.

### docs/playbooks/ (keep all)
`README.md`, `managed-aws-hosted.md`, `customer-self-hosted-aws.md`, `iso-fleet-provisioning.md` — all **current**.

### docs/testing/ (keep all)
`prod-docker-lab.md`, `prod-docker-test-plan.md`, `aws-e2e-test-plan.md` — all **current**. `firmware-ota-hardware-test-plan.md` added 2026-06-07 (Phase F hardware test plan).

### docs/incidents/ (keep all)
`README.md`, `ir-runbook.md` (escalation contacts TBD — roadmap queue), dated incident notes.

---

## Summary
~74 current / keep, 2 archive, 3 light updates pending (items 4 and 5 above + the `security-hardening.md` banner). No deletions, no major merges. Fastest win was rebuilding the index (done).
