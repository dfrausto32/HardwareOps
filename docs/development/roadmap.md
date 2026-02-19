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
- ✅ Metrics & health (Prometheus `/metrics` + health summary + UI charts).
- ✅ Certificate rotation cleanup (hybrid grace/coverage + UI button).
- ✅ Artifact lifecycle management (deprecate/restore, safe delete gating, retention policy, manual + scheduled prune).
- ✅ Bulk group management (shift-select, modal multi-edit, modal multi-group desired-state apply, batch API + advanced CSV rollback flow).
- ✅ AWS per-customer Terraform scaffold + CLI wrapper + deployment runbook + demo fleet baseline.
- ✅ Artifact ingest validation harness (push + pull local smoke test + operator test runbook).
- ✅ Artifactory pull adapter (provider plugin) + signed local demo workflow (`setup-artifactory-demo.sh` / `test-artifactory-adapter.sh`).
- ✅ License anti-cheat hardening v1 (atomic enroll cap enforcement + clone-suspicion telemetry).
- ✅ Device identity hardening v1 (`DEVICE_IDENTITY_MODE` audit/enforce + hardware identity conflict detection on enroll/check-in).

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
- **Notes:** See `artifact-signing.md`.

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
- **Notes:** CA bundle trust + active signer + UI rotate/reload + agent auto‑reenroll implemented. Server‑TLS rotation runbook documented in `../certs.md`.

#### Backup & restore runbooks
- **Status:** 🟢 Complete
- **Scope:** Postgres + object store recovery; tested restore procedure; UI-driven backup/restore + wipe.
- **Dependencies:** Backup tooling; storage policy.
- **Risks:** Incomplete restores.
- **Acceptance:** Restore tested with documented RTO/RPO.
- **Notes:** UI supports create/restore + wipe; scripts included for on‑prem and local dev. See `../backup-restore.md`.

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
- **Status:** 🟢 Complete
- **Scope:** Prometheus endpoints; core dashboards; UI graphs for system metrics.
- **Dependencies:** Metrics library + exporters.
- **Risks:** Missing or noisy signals.
- **Acceptance:** Operators can see fleet/apply health at a glance.
- **Notes:** `/metrics` endpoint + health summary API + UI metrics page + dashboard activity graph shipped.

#### Event retention
- **Status:** 🟢 Complete
- **Scope:** Realtime WS/SSE + persisted event store + retention policy.
- **Dependencies:** Storage backend; retention jobs.
- **Risks:** Storage growth.
- **Acceptance:** Events are queryable over defined retention window.
- **Notes:** Runtime events are persisted (`runtime_events`) with query API (`GET /api/v1/events/history`), retention config endpoints (`GET/PUT /api/v1/events/retention`), startup+scheduled cleanup worker, and Logs-page UI controls for filtering/history + retention updates.

#### Certificate rotation cleanup (hybrid)
- **Status:** 🟢 Complete
- **Scope:** Remove old CA from bundle once coverage reaches 100% or after max grace period (whichever comes first); warn if devices still on old CA.
- **Dependencies:** Rotation status counts; scheduled task; UI warning/banners.
- **Risks:** Offline devices stranded after grace expiry.
- **Acceptance:** Old CA pruned safely; operators warned with clear counts and timestamps.
- **Notes:** Cleanup endpoint + UI control + grace window in place (see `../certs.md`).

#### Artifact lifecycle management
- **Status:** 🟢 Complete
- **Scope:** Retain/deprecate/delete with safety guardrails and policy-based cleanup.
- **Dependencies:** Artifact metadata + policy engine.
- **Risks:** Deleting in‑use artifacts.
- **Acceptance:** 
  - Artifacts can be deprecated/restored without breaking desired state.
  - Delete is blocked for active/in-use artifacts unless explicitly overridden.
  - Lifecycle policy controls deprecation retention; prune only removes eligible, unreferenced artifacts.
- **Notes:** API + UI support status, retention policy (Settings), deprecate/restore, reference counts, manual prune (Artifacts), and scheduled auto-prune with telemetry/alerts.

#### License anti-cheat hardening
- **Status:** 🟡 In progress
- **Scope:** Make license-cap bypass and device-clone abuse painful/detectable in on-prem environments.
- **Dependencies:** Enrollment transaction flow, mTLS identity metadata, audit/runtime events, deployment guardrails.
- **Risks:** False positives on clone detection and operator friction on decommission workflows.
- **Acceptance:**
  - Enrollment cap enforcement is transactional and race-safe under concurrent enroll requests.
  - Hardware identity conflicts are detected across enroll/check-in and can be enforced (`audit`/`enforce` mode).
  - Suspicious identity reuse patterns (rapid source-IP switch and capability drift) produce runtime/audit signals.
  - Production guardrails prevent accidental insecure mode (`AUTH_MODE=disabled`, `LICENSE_ENFORCE=0`) in hardened profiles.
  - Trusted-proxy allowlist is enforced for forwarded client-cert headers.
  - Device slot reclaim/decommission path is explicit and auditable (no silent quota bypass by deletes).
- **Notes:** Shipped now: transactional device cap enforcement, clone-suspicion runtime/audit events, and hardware identity audit/enforce checks (`device-identity-hardening.md`). Remaining hardening items are scheduled in Phase B.

#### Release channels
- **Status:** ⬜ Planned
- **Scope:** Stable/canary labels; gradual rollout controls.
- **Dependencies:** Grouping + desired-state policy.
- **Risks:** Mis‑targeted rollouts.
- **Acceptance:** Controlled staged rollouts with visibility.
- **Notes:** 

#### Bulk group management
- **Status:** 🟢 Complete
- **Scope:** CSV import + batch label changes.
- **Dependencies:** UI + API.
- **Risks:** Accidental broad changes.
- **Acceptance:** Bulk changes are previewable and reversible.
- **Notes:** Groups page supports row-by-row multi-select (including shift-range), modal multi-edit for selected groups, modal multi-group desired-state apply, delete-selected, and an advanced CSV workflow with preview/apply + rollback CSV. Batch API is available at `POST /api/v1/groups/batch`.

#### CI integrations
- **Status:** 🟢 Complete
- **Scope:** Presigned uploads + repo/blob pulls (HTTP/Artifactory) with signed artifact ingestion.
- **Dependencies:** Secrets management; ingest modes.
- **Risks:** Credential leakage.
- **Acceptance:** CI can publish artifacts without long‑lived creds.
- **Notes:** Presigned CI push path implemented (`/artifacts/presign-upload` + `/artifacts/complete`) with scoped expiring service tokens (`artifact.publish`) and `scripts/ci-upload-artifact.sh`. Pull ingest API implemented (`/artifacts/pull`) with checksum validation + host/size/timeout guardrails plus CI helper `scripts/ci-pull-artifact.sh`. Adapter framework and backward-compatible source shape are in place (`sourceUrl` or `source.kind` + `source.uri`; current kinds=`http`,`artifactory`). Credential resolver abstraction is in place (`source.credentialRef`) with static map (`ARTIFACT_PULL_CREDENTIALS_FILE`/`ARTIFACT_PULL_CREDENTIALS_JSON`) plus AWS Secrets Manager-backed loading (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`). Pull resolver operational workflow is implemented with admin status/reload endpoints (`GET/POST /api/v1/artifacts/pull-credentials*`) and helper script (`scripts/reload-pull-credentials.sh`). Artifactory adapter shipped with local docker smoke scripts (`scripts/setup-artifactory-demo.sh`, `scripts/test-artifactory-adapter.sh`) and signed artifact demo flow. Smoke coverage now validates push + pull + pull-credential reload audit path in `scripts/test-artifact-ingest.sh` (see `artifact-ingest-test-plan.md`). CI provider scaffold templates added for GitHub Actions/GitLab/Jenkins (`../../deploy/ci/README.md`). Vault resolver backend is deferred to Phase C.

### Recommended Next Sequence (Current)
1. **Release channels:** Add stable/canary promotion and staged rollout targeting.
2. **License anti-cheat hardening:** Complete remaining guardrails and decommission/reclaim workflow validation.
3. **Phase B closeout:** Run end-to-end validation + docs cleanup for CI/release workflow handoff.

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

#### CI workload identity federation
- **Status:** ⬜ Planned
- **Scope:** OIDC-based CI auth (GitHub/GitLab/Jenkins) for publish/pull without static secrets.
- **Dependencies:** OIDC trust config, service-token exchange endpoint/policy.
- **Risks:** Misconfigured trust policies.
- **Acceptance:** CI jobs can obtain short-lived publish credentials by identity, no long-lived credential files required.
- **Notes:** Candidate for Phase C once Phase B static-secret resolver and adapters are complete.

#### Supply-chain provenance policy (Cosign/Sigstore)
- **Status:** ⬜ Planned
- **Scope:** Optional attestations/provenance verification policy in addition to Ed25519 signature checks.
- **Dependencies:** CI provenance generation, policy model, verification pipeline.
- **Risks:** Operational complexity and false rejects.
- **Acceptance:** Policy can enforce trusted builder/provenance on selected artifact classes.
- **Notes:** Phase C hardening item; current v1 remains Ed25519-compatible.

#### Vulnerability scanning integration (optional, Tenable Nessus)
- **Status:** ⬜ Planned
- **Scope:** Optional integration to ingest Nessus scan results and surface vulnerability posture for deployed artifacts/devices.
- **Dependencies:** Tenable API credentials, scan-to-asset mapping model, ingestion/sync job, UI views.
- **Risks:** Asset identity mismatch, stale scan data, noisy findings without normalization.
- **Acceptance:** Operators can see per-device/per-artifact vulnerability status (last scan time, severity counts, top CVEs), with no impact when feature is disabled.
- **Notes:** Keep non-blocking for deployments; customers can enable/disable per environment. Initial cut should be read-only visibility first, policy gating later.

#### Cloud-native pull adapters (S3/GCS)
- **Status:** ⬜ Planned
- **Scope:** Native `source.kind` adapters for S3 and GCS pull ingest.
- **Dependencies:** IAM/Workload Identity, credential resolver policy, host/path allowlists.
- **Risks:** Credential misconfiguration and broad bucket permissions.
- **Acceptance:** Control-plane can ingest artifacts directly from S3/GCS sources with checksum/signature verification and audit parity.
- **Notes:** Deferred from Phase B to Phase C; HTTP/Artifactory remain the v1 pull adapters.

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
- **Notes:** AWS Secrets Manager-backed pull credential resolver is in place for CI pull ingest (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`) from Phase B. Phase C expands this with a HashiCorp Vault resolver backend (plus auth/rotation runbook) so customers can use Vault as the source of pull credentials and related control-plane secrets.

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
- **Status:** 🟡 In progress
- **Scope:** ECS Fargate + RDS + S3 + ALB + ACM.
- **Dependencies:** Container build pipeline.
- **Risks:** Operational cost creep.
- **Acceptance:** AWS deployment runbook + reference infra.
- **Notes:** Terraform scaffold + CLI/runbook shipped (`deploy/aws/terraform`, `scripts/aws-customer.sh`); production hardening pass in progress.

#### IAM integration
- **Status:** 🟡 In progress
- **Scope:** S3 + KMS access control.
- **Dependencies:** AWS IAM design.
- **Risks:** Over‑permissioned roles.
- **Acceptance:** Least‑privilege IAM policies validated.
- **Notes:** Baseline task roles exist; least-privilege tightening is next.

#### Network isolation guidance
- **Status:** 🟡 In progress
- **Scope:** VPC / private subnet patterns.
- **Dependencies:** AWS architecture.
- **Risks:** Misconfigured routes.
- **Acceptance:** Reference diagram + sample config.
- **Notes:** Private subnet model documented; ingress hardening and WAF policy rollout pending.

#### AWS security hardening pack
- **Status:** 🟡 In progress
- **Scope:** Secrets Manager-first production path, least-privilege IAM, WAF on app ingress, ingress CIDR split, ECS exec guardrails, and security alarms.
- **Dependencies:** Terraform modules, runbook updates, incident response wiring.
- **Risks:** Misconfigured hardening controls can block traffic or operations.
- **Acceptance:** 
  - No plaintext secrets in production task definitions.
  - WAF attached and tested.
  - IAM policy review passed with scoped permissions.
  - ECS exec disabled by default for production.
  - Security alarms routed to on-call channel.
- **Notes:** Execution details tracked in `security-hardening.md`.

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
