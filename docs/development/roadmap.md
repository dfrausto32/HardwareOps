# Product Development Roadmap & Progress Tracker

This document is the **authoritative internal roadmap** for the platform. It is designed to:
- Track development progress over time
- Communicate current and future capabilities to customers
- Support sales and stakeholder conversations with concrete status

The roadmap is organized by **maturity phases**, not deadlines. Phases are sequential, but work may overlap.

---

## Recently Completed (Current State)
- ✅ Artifact signing + verification (Ed25519) end‑to‑end (packer → control‑plane → agent).
- ✅ Device status model (active/stale/offline) with periodic refresh.
- ✅ Agent throttling + jitter (fleet‑friendly check‑ins).
- ✅ Control‑plane rate limits (check‑in / enroll / apply).
- ✅ Audit logging + query + CSV export + retention controls.
- ✅ Local auth (JWT) + bootstrap admin + voucher invites.
- ✅ Web UI for auth, audit logs, and retention management.
- ✅ Fleet capacity enforcement (signed on‑prem license + device cap).
- ✅ Upgrade strategy automation (preflight + staged apply + rollback health gate).
- ✅ Multi‑artifact device management (componented desired state + per‑component apply status).
- ✅ Certificate rotation (CA bundle + active signer + UI rotate/reload + auto re‑enroll).
- ✅ Backup + restore runbooks + UI workflows (Postgres + MinIO).

---

## Phase A — Foundation (Production-Ready Core)
**Goal:** Safe, deployable, and supportable for real customers.

### Definition of Done (Phase Exit Criteria)
- System can be deployed to customer environments
- Artifacts are verifiably safe to apply
- Device health and failure modes are visible
- Recovery paths are documented and tested

### Feature Templates
#### Artifact signing + verification
- **Status:** 🟢 Complete
- **Scope:** CI/packer signs artifacts (Ed25519 in v1); agents verify before apply; hard-fail on invalid signature; metadata supports future Cosign/Sigstore without breaking existing flow.
- **Dependencies:** Signing key management, CI integration, agent verify path, artifact metadata fields.
- **Risks:** Key compromise, signing flow drift.
- **Acceptance:** 
  - Ed25519 signatures stored alongside artifacts (signature + key ID).
  - Agents reject unsigned/invalid artifacts with clear error path.
  - Migration path documented for Cosign/Sigstore. 
- **Notes:** See `docs/development/artifact-signing.md`.

#### Device status model
- **Status:** 🟢 Complete
- **Scope:** active / stale / degraded / offline; heartbeat + last-seen tracking.
- **Dependencies:** Control-plane status rules; UI surfacing.
- **Risks:** False positives on stale/offline.
- **Acceptance:** Status transitions are deterministic and observable.
- **Notes:** 

#### Agent throttling & jitter
- **Status:** 🟢 Complete
- **Scope:** Staggered check-ins; fleet-size aware defaults.
- **Dependencies:** Agent config; control-plane guidance.
- **Risks:** Delayed updates at scale.
- **Acceptance:** Check-ins distribute evenly under load.
- **Notes:** 

#### Rate limiting
- **Status:** 🟢 Complete
- **Scope:** Adaptive backoff by fleet size; protect against misbehaving agents.
- **Dependencies:** Control-plane rate limiter, telemetry.
- **Risks:** Starving healthy devices.
- **Acceptance:** No sustained overload; fair device access.
- **Notes:** 

#### Upgrade strategy
- **Status:** 🟢 Complete
- **Scope:** Schema migrations; backward-compatible agent rollout; rollback safety.
- **Dependencies:** Migration tool, versioning policy.
- **Risks:** Data loss or downtime.
- **Acceptance:** Zero-data-loss upgrade with rollback plan.
- **Notes:** Preflight + UI status panels + health gate + staged apply/rollback contract implemented.

#### Fleet capacity enforcement (on‑prem licensing)
- **Status:** 🟢 Complete
- **Scope:** Hard cap on total devices (enrolled/active) that cannot be changed by operators in on‑prem installs.
- **Dependencies:** License format + verification, enforcement points in API.
- **Risks:** Accidental lockout if limit is mis‑set; failure to enforce consistently.
- **Acceptance:** 
  - Control‑plane refuses enroll/check‑in when cap exceeded (clear error).
  - Cap is loaded from a signed license file (or compiled limit) that operators cannot alter without vendor key.
  - UI shows current usage vs cap (read‑only).
- **Notes:** Recommend signed license file with embedded public key; future hosted control‑plane can validate against vendor service.

#### Multi‑artifact device management (componented desired state)
- **Status:** 🟢 Complete
- **Priority:** High (required for agent self‑updates + app/firmware parity)
- **Scope:** Desired state supports multiple components (agent / app / firmware / container). Track and display per‑component current version + last apply status. App bundle no longer default; agent component is always present.
- **Dependencies:** Desired‑state schema, apply‑result payload updates, UI rendering.
- **Risks:** Backward compatibility with legacy single‑artifact devices.
- **Acceptance:**
  - Desired state supports multiple components in one payload.
  - Agents can independently apply and report per‑component status.
  - UI shows per‑component current/desired version + last apply status.
  - Agent component is always reported; app_bundle is no longer default.
- **Notes:** Enables agent self‑update and multi‑app deployments.

#### Certificate rotation
**Status:** 🟢 Complete
- **Scope:** CA rotation process; device re-enrollment guidance; admin UI rotate/reload.
- **Dependencies:** CA tooling; device enrollment flow; writable cert paths.
- **Risks:** Bricking agents during rotation if bundle missing or certs read-only.
- **Acceptance:** Rotation can be executed without full fleet outage.
- **Notes:** CA bundle trust + active signer + UI rotate/reload + agent auto‑reenroll implemented. Server‑TLS rotation runbook documented in `docs/certs.md`.

#### Backup & restore runbooks
- **Status:** 🟢 Complete
- **Scope:** Postgres + object store recovery; tested restore procedure; UI-driven backup/restore + wipe.
- **Dependencies:** Backup tooling; storage policy.
- **Risks:** Incomplete restores.
- **Acceptance:** Restore tested with documented RTO/RPO.
- **Notes:** UI supports create/restore + wipe; scripts included for on‑prem and local dev. See `docs/backup-restore.md`.

---

## Phase B — Operational Maturity
**Goal:** Make the system observable, auditable, and operable at scale.

### Definition of Done
- Operators can see what happened and why
- Rollouts are controlled and repeatable
- Artifacts and events have lifecycle policies

### Feature Templates
#### Audit logging
- **Status:** 🟢 Complete
- **Scope:** Who did what/when; immutable entries; export CSV/JSON.
- **Dependencies:** Auth/RBAC, storage for audit trail.
- **Risks:** Gaps in coverage.
- **Acceptance:** All state‑changing actions are logged and queryable.
- **Notes:** Includes retention configuration + CSV export.

#### Metrics & health
- **Status:** ⬜ Planned
- **Scope:** Prometheus endpoints; core dashboards.
- **Dependencies:** Metrics library + exporters.
- **Risks:** Missing or noisy signals.
- **Acceptance:** Operators can see fleet/apply health at a glance.
- **Notes:** 

#### Event retention
- **Status:** ⬜ Planned
- **Scope:** Realtime WS/SSE + persisted event store + retention policy.
- **Dependencies:** Storage backend; retention jobs.
- **Risks:** Storage growth.
- **Acceptance:** Events are queryable over defined retention window.
- **Notes:** 

#### Certificate rotation cleanup (hybrid)
- **Status:** ⬜ Planned
- **Scope:** Remove old CA from bundle once coverage reaches 100% or after max grace period (whichever comes first); warn if devices still on old CA.
- **Dependencies:** Rotation status counts; scheduled task; UI warning/banners.
- **Risks:** Offline devices stranded after grace expiry.
- **Acceptance:** Old CA pruned safely; operators warned with clear counts and timestamps.
- **Notes:** Hybrid approach added; not required for Phase A.

#### Artifact lifecycle management
- **Status:** ⬜ Planned
- **Scope:** Retain/deprecate/delete; policy-based cleanup.
- **Dependencies:** Artifact metadata + policy engine.
- **Risks:** Deleting in‑use artifacts.
- **Acceptance:** Safe cleanup with explicit policies.
- **Notes:** 

#### Release channels
- **Status:** ⬜ Planned
- **Scope:** Stable/canary labels; gradual rollout controls.
- **Dependencies:** Grouping + desired-state policy.
- **Risks:** Mis‑targeted rollouts.
- **Acceptance:** Controlled staged rollouts with visibility.
- **Notes:** 

#### Bulk group management
- **Status:** ⬜ Planned
- **Scope:** CSV import + batch label changes.
- **Dependencies:** UI + API.
- **Risks:** Accidental broad changes.
- **Acceptance:** Bulk changes are previewable and reversible.
- **Notes:** 

#### CI integrations
- **Status:** ⬜ Planned
- **Scope:** Presigned uploads + repo/blob pulls (Artifactory/S3/GCS).
- **Dependencies:** Secrets management; ingest modes.
- **Risks:** Credential leakage.
- **Acceptance:** CI can publish artifacts without long‑lived creds.
- **Notes:** 

---

## Phase C — Enterprise Readiness
**Goal:** Pass security and identity reviews.

### Definition of Done
- Identity is externally managed
- Privileged access is auditable
- Access control is least-privilege

### Feature Templates
#### Fixed RBAC roles
- **Status:** 🟡 In progress
- **Scope:** Admin / Operator / Viewer.
- **Dependencies:** Auth middleware + policy checks.
- **Risks:** Role creep.
- **Acceptance:** Endpoints and UI gated correctly.
- **Notes:** API role enforcement added; UI gating partially implemented.

#### Role-aware UI
- **Status:** 🟡 In progress
- **Scope:** Action gating; read‑only views.
- **Dependencies:** RBAC roles.
- **Risks:** Inconsistent UI behavior.
- **Acceptance:** UI hides actions for non‑authorized roles.
- **Notes:** Auth UI exists; remaining pages need role-aware gating.

#### Custom RBAC policies
- **Status:** ⬜ Planned
- **Scope:** Per‑resource permissions.
- **Dependencies:** Policy engine; admin UI.
- **Risks:** Complexity / misconfig.
- **Acceptance:** Custom policies enforce least privilege.
- **Notes:** 

#### OIDC SSO
- **Status:** ⬜ Planned
- **Scope:** Group‑to‑role mapping.
- **Dependencies:** IdP config; callback endpoints.
- **Risks:** IdP misconfiguration.
- **Acceptance:** Users can log in via OIDC and get correct roles.
- **Notes:** 

#### LDAP / AD support
- **Status:** ⬜ Planned
- **Scope:** LDAP/AD auth or sync.
- **Dependencies:** Enterprise customer requirements.
- **Risks:** Directory inconsistency.
- **Acceptance:** Authentication works with AD/LDAP.
- **Notes:** 

#### Secrets management integration
- **Status:** ⬜ Planned
- **Scope:** Vault / AWS Secrets Manager.
- **Dependencies:** Secrets provider.
- **Risks:** Secret sprawl.
- **Acceptance:** No long‑lived secrets in config files.
- **Notes:** 

#### Break-glass workflows
- **Status:** 🟡 In progress
- **Scope:** Bootstrap admin + forced rotation + audit trail.
- **Dependencies:** Audit logging, auth.
- **Risks:** Misuse.
- **Acceptance:** Emergency access is possible and auditable.
- **Notes:** Bootstrap admin + voucher onboarding implemented; rotation + revocation workflows remain.

---

## Phase D — Scale & Cloud Optionality
**Goal:** Meet customers where they run.

### Definition of Done
- Platform runs in customer-preferred infra
- Network and tenant boundaries are clear

### Feature Templates
#### AWS reference deployment
- **Status:** ⬜ Backlog
- **Scope:** ECS Fargate + RDS + S3 + ALB + ACM.
- **Dependencies:** Container build pipeline.
- **Risks:** Operational cost creep.
- **Acceptance:** AWS deployment runbook + reference infra.
- **Notes:** 

#### IAM integration
- **Status:** ⬜ Backlog
- **Scope:** S3 + KMS access control.
- **Dependencies:** AWS IAM design.
- **Risks:** Over‑permissioned roles.
- **Acceptance:** Least‑privilege IAM policies validated.
- **Notes:** 

#### Network isolation guidance
- **Status:** ⬜ Backlog
- **Scope:** VPC / private subnet patterns.
- **Dependencies:** AWS architecture.
- **Risks:** Misconfigured routes.
- **Acceptance:** Reference diagram + sample config.
- **Notes:** 

#### Multi-tenant controls (optional)
- **Status:** ⬜ Backlog
- **Scope:** Tenant isolation model.
- **Dependencies:** Auth/RBAC.
- **Risks:** Data leakage between tenants.
- **Acceptance:** Tenant boundary enforcement + tests.
- **Notes:** 

---

## Status Legend
- ⬜ Planned
- 🟡 In progress
- 🟢 Complete
- 🔴 Blocked

---

> **Design principle:** Start simple, scale safely, integrate where customers already operate.

---

This roadmap is reviewed continuously and updated as customer needs are validated.
