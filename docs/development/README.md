# Development Reference

Internal design docs, architecture decisions, and implementation deep dives.
These docs complement the canonical operator-facing docs in `../` (deploy, operations, icd).

---

## Product Direction

| Doc | What it covers |
|---|---|
| `roadmap.md` | Authoritative implementation roadmap and phase status |

---

## Auth and Access Control

| Doc | What it covers |
|---|---|
| `auth-secrets-v1.md` | Auth model design: local users, JWT structure, roles, voucher workflow, bootstrap admin, OIDC extension points |
| `license.md` | Signed license format, device-cap enforcement, anti-cheat controls |

---

## Artifact Ingest and Signing

| Doc | What it covers |
|---|---|
| `artifact-ingest.md` | Ingest modes (manual / CI push / pull), when to use each, API details, signing model (Ed25519 → Cosign path), security baseline |
| `artifact-ingest-test-plan.md` | End-to-end test plan for push + pull ingest paths |
| `artifact-apply-roadmap.md` | Future artifact apply: firmware flash and container image apply planning |

---

## Enrollment and Device Identity

| Doc | What it covers |
|---|---|
| `agent-first-contact-onboarding.md` | First-contact approval flow: full state machine, API, UI, anti-spam controls, agent implementation |

---

## AWS Deployment

| Doc | What it covers |
|---|---|
| `aws-cloud-setup-plan.md` | Deployment shapes (on-prem vs AWS), AWS architecture baseline, production readiness controls, implementation sequence |
| `aws-customer-deployment-runbook.md` | Step-by-step operator runbook for deploying and hardening a customer AWS environment |
| `security-hardening.md` | AWS security hardening tracker: acceptance evidence requirements and known residual items |

---

## Observability and Operations

| Doc | What it covers |
|---|---|
| `metrics-health.md` | Prometheus metrics catalog, health summary API, alert thresholds |
| `upgrade-strategy.md` | Staged apply strategy, rollback contract, deployment safety |

---

## Rule of thumb

- Start in `../deploy.md` (deployment) or `../operations.md` (day-2) for runbook-style guidance.
- Come here for design rationale, architecture decisions, and deep implementation details.
- `roadmap.md` is the single source of truth for what is done and what comes next.
