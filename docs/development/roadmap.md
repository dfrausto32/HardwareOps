# Product Development Roadmap & Progress Tracker

This document is the **authoritative internal roadmap** for the platform. It is designed to:
- Track development progress over time
- Communicate current and future capabilities to customers
- Support sales and stakeholder conversations with concrete status

The roadmap is organized by **maturity phases**, not deadlines. Phases are sequential, but work may overlap.

---

## Program Snapshot (Parallel Session Staging)
_Updated: 2026-03-10 (post D-hardening-alarms/secrets/ecs-exec batch)_

Use this section as the single source of truth for "what is done" vs "what is left."

### Phase-level status
| Phase | Status | Summary |
|---|---|---|
| Phase A — Foundation | 🟢 Complete | Core deployability, signing, upgrades, cert rotation, licensing, backup/restore are in place. |
| Phase B — Operational Maturity | 🟢 Complete | Audit/metrics/events/lifecycle/CI ingest/bulk ops shipped and operational. |
| Phase B Extension — UX + Realtime | 🟢 Complete | Bulk actions + realtime updates + auth-session UX reset shipped. |
| Operational Hardening (between B and C) | 🟢 Complete | Pull-boundary, token exposure, startup guardrails, break-glass backend, proxy trust policy, and abuse controls are all shipped. |
| Phase C — Enterprise Readiness | 🟡 In progress | Fixed RBAC, role-aware UI parity, break-glass APIs, and first-contact approval onboarding all shipped. Planned extensions (OIDC, custom RBAC, LDAP, Vault secrets) remain as future work. |
| Phase D — Scale & Cloud Optionality | 🟡 In progress | AWS reference deployment and least-privilege IAM shipped. WAF attached; ingress CIDR split in place. Acceptance runbook and gate script created. Plaintext DATABASE_URL eliminated; ECS exec off by default; CloudWatch alarms Terraform-managed. Live-deployment acceptance gate execution remains. |

### Active work queue (what is still to do)

| Task | Issue | Branch | Worktree | Status |
|---|---|---|---|---|
| C-OIDC-SSO: OIDC identity provider integration | #23 | `feat/23-c-oidc-sso` | agent-1 | 🟡 In progress |

### Prepared next tasks (agent-scoped)
2026-03-10 D-hardening batch merged (alarms, secrets, ecs-exec). Candidate items for the next sequence: OIDC SSO and custom RBAC policies.

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
- ✅ Backend RBAC matrix coverage for representative viewer/operator/admin endpoints plus `artifact.publish` service-token gating on publish flows.
- ✅ License anti-cheat hardening close-out (transactional cap checks + identity enforcement + hardened profile + auditable decommission).
- ✅ Device identity hardening v1 (`DEVICE_IDENTITY_MODE` audit/enforce + hardware identity conflict detection on enroll/check-in).
- ✅ Artifact auto-version tracking (global default + per-component override, semver `W.X.Y[.Z]`, signed/active eligibility, periodic + ingest-triggered desired-state updates).
- ✅ Artifact signature enforcement policy (control-plane ingest guardrails + check-in applyPolicy defaults for agent-side verification).
- ✅ Enrollment profile token reissue policy (rotate-time grace windows + rotation reason audit metadata).
- ✅ Pending-enrollment queue telemetry closeout (`hwops_pending_enroll_queue_age_total`, `hwops_pending_enroll_oldest_age_seconds`, throttle-by-reason docs + alert thresholds).
- ✅ Break-glass backend workflows (service-token revoke/rotate + cert rotation reason capture + audit coverage).
- ✅ Trusted proxy CIDR hardening (strict wildcard rejection, hardened profile validation, and deployment docs/templates update).
- ✅ Role-aware UI parity (centralized `buildPermissionState` permission helper; nav, logs tabs, and mutating controls gated by role; browser-vs-backend parity regression suite added).
- ✅ First-contact onboarding production closeout (packaged self-contained installer/bootstrap path; agent-host gateway routes for pending-enrollment request/claim; prod-lab deterministic compose subnet and trusted proxy CIDRs; new `prod-lab-first-contact.sh` end-to-end validation script; full first-contact approval flow validated in prod-docker-lab).
- ✅ AWS Terraform IAM/WAF/ingress hardening (least-privilege ECS task-execution and task IAM roles; WAFv2 web ACL with IP-reputation, known-bad-inputs, and CRS managed rule sets attached to ALB app ingress; ingress CIDR split between app and device trust boundaries; Terraform `fmt` + `validate` passed across dev/staging/prod environments).
- ✅ AWS hardening acceptance gate and runbook (rewritten `aws-customer-deployment-runbook.md` with pre/post-apply evidence checklist; `scripts/aws-hardening-check.sh` config and deployment gate helper; `security-hardening.md` updated with acceptance evidence requirements and residual known-debt documentation).
- ✅ Plaintext DATABASE_URL elimination (`database_url_secret_arn` variable in `customer_stack`; when set, DATABASE_URL routes via ECS `valueFrom` Secrets Manager injection instead of plaintext task env; backward-compatible when unset).
- ✅ ECS exec guardrail (`enable_execute_command` default changed to `false` in `modules/ecs`; variable threaded through `customer_stack`; opt-in documented in tfvars examples).
- ✅ CloudWatch alarm Terraform resources (SNS topic + email subscription + 8 managed alarms: ALB 5xx/unhealthy-host, ECS CPU/memory/task-count, RDS CPU/storage/connections; all thresholds parameterized; `aws-hardening-check.sh deployment` alarm checks now have resources to validate).

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
- **Status:** 🟢 Complete
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
- **Notes:** Shipped: transactional cap enforcement, clone-suspicion runtime/audit events, hardware identity audit/enforce checks (`device-identity-hardening.md`), hardened startup profile guardrails (`HARDENED_PROFILE`), trusted-proxy allowlist enforcement, and explicit admin decommission endpoint for auditable slot reclaim (`POST /api/v1/devices/{deviceId}/decommission`).

#### Artifact auto-version tracking
- **Status:** 🟢 Complete
- **Scope:** Auto-advance desired state to the newest eligible artifact when `name + type` match and version is semver (`W.X.Y` or `W.X.Y.Z`).
- **Dependencies:** Artifact metadata hygiene, desired-state components, audit trail.
- **Risks:** Unexpected upgrades if defaults are too broad.
- **Acceptance:** 
  - Global default toggle in Settings with optional `allowUnsigned` override.
  - Per-component `autoVersion.mode` override (`inherit` / `enabled` / `disabled`) in group/device desired-state editors.
  - Only `status=active` artifacts are considered; signature required unless unsigned override is enabled.
  - Runs on schedule and immediately after artifact ingest (`create` / `upload` / `pull` / `complete`).
  - Updates components independently and emits audit events for entity updates + run summaries.
- **Notes:** Supports immediate desired-state updates; agent-side apply rollback behavior remains unchanged.

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

### Recommended Next Sequence (Pending Prioritization)
The 2026-03-10 D-hardening batch merged. Items 1–3 below are now complete. Remaining candidates:
1. ~~**Plaintext secret elimination**~~ — ✅ Done (#18)
2. ~~**ECS exec guardrails**~~ — ✅ Done (#19)
3. ~~**CloudWatch alarm Terraform resources**~~ — ✅ Done (#17)
4. **OIDC SSO:** Begin Phase C OIDC identity provider integration now that RBAC layer and role-aware UI are in place.
5. **Custom RBAC policies:** Per-resource permission engine and admin UI.

Confirm priorities with the team before mapping to agents.

---

## Phase B Extension — Operator UX + Realtime Workflow (Post-Phase B, Pre-Phase C)
**Goal:** Close production operator workflow gaps found during Docker prod-lab testing.

### Definition of Done
- Operators can perform high-volume cleanup actions directly in UI without repetitive per-row work.
- Device/artifact lifecycle changes are visible near-real-time without manual refresh loops.
- Expired/invalid auth sessions fail cleanly back to login instead of broad fetch-failure states.

### Backlog Tracks
#### Device bulk delete actions
- **Status:** 🟢 Complete
- **Scope:** Multi-select devices in Devices page and delete selected in one operation.
- **Notes:** Devices page now supports row/shift multi-select with explicit decommission confirmation + reason and bulk execution summary.

#### Group delete with optional device cascade
- **Status:** 🟢 Complete
- **Scope:** Group delete flow can optionally remove devices assigned to that group.
- **Notes:** Single and selected-group delete flows now support optional "Delete + Devices" cascade with impact counts and required decommission reason.

#### Device logs navigation improvements
- **Status:** 🟢 Complete
- **Scope:** Add device selector dropdown on Logs page for fast switching.
- **Notes:** Preserve current filter state when switching devices.

#### Artifact bulk delete
- **Status:** 🟢 Complete
- **Scope:** Multi-select artifacts for delete/prune actions (still honoring in-use safety checks).
- **Notes:** Artifacts table now supports row/shift multi-select and bulk delete; only deprecated + unreferenced artifacts are deleted and skipped counts are surfaced.

#### Realtime upload + registration updates
- **Status:** 🟢 Complete
- **Scope:** WebSocket/SSE-driven near-real-time UI updates for artifact uploads and new device registration.
- **Notes:** Runtime events now include `artifact.registered` and `device.enroll`; dashboard clients auto-refresh artifacts/devices on relevant event types via websocket feed with throttled refresh.

#### Auth/session failure UX reset
- **Status:** 🟢 Complete
- **Scope:** On auth expiry/invalid session, route to login with clear session-expired message.
- **Notes:** Avoid global "everything failed to fetch" dead state.

---

## Operational Hardening (Post-Phase B, Pre-Phase C)
**Goal:** Close immediate production security gaps before enterprise identity/policy expansion.

### Definition of Done
- Production bundles fail fast on insecure defaults.
- Browser clients are not shipped privileged operational secrets.
- Pull ingest and proxy trust are tightly bounded to explicit network policy.
- Auth, artifact, and upgrade paths have abuse-resistant controls.

### Hardening Tracks
#### Pull ingest boundary hardening
- **Status:** 🟢 Complete
- **Scope:** Require explicit `ARTIFACT_PULL_ALLOWED_HOSTS` in hardened deployments; default to HTTPS-only pull adapters; block internal/loopback targets unless explicitly allowed.
- **Dependencies:** Pull adapter policy checks, startup config validation.
- **Risks:** Breaking existing permissive pull workflows if migration is not staged.
- **Acceptance:** Pull ingest cannot reach arbitrary/internal endpoints by default.
- **Notes:** Implemented with adapter-level policy enforcement (`http`/`artifactory`), `ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0` production defaults, internal/loopback block unless allowlisted, and hardened-profile startup checks requiring explicit pull allowlist.

#### Maintenance/upgrade token exposure removal
- **Status:** 🟢 Complete
- **Scope:** Remove `VITE_MAINTENANCE_TOKEN` from shipped UI; perform maintenance/upgrade authorization server-side via admin auth and/or operator-only backend workflows.
- **Dependencies:** UI/API maintenance flows, deployment templates.
- **Risks:** Temporary operator friction during migration.
- **Acceptance:** No privileged maintenance token embedded in browser-delivered assets.
- **Notes:** UI now uses admin-authenticated API calls for maintenance toggle and upgrade apply; browser build/token wiring removed from compose, installer, and upgrade package templates.

#### Secure defaults and startup guardrails
- **Status:** 🟢 Complete
- **Scope:** Eliminate permissive fallback secrets/tokens for production profiles; enforce `LICENSE_ENFORCE=1`, non-placeholder auth/bootstrap/maintenance secrets, and strict proxy CIDR validation.
- **Dependencies:** Config validation, installer/compose templates.
- **Risks:** Install failures for environments still using dev defaults.
- **Acceptance:** Hardened/prod startup refuses weak or placeholder configuration.
- **Notes:** Hardened startup now enforces non-placeholder/length guardrails for auth + bootstrap secrets, rejects overly long auth token TTL, validates proxy CIDRs, and rejects wildcard proxy trust ranges.

#### Privilege boundary for upgrade/backup execution
- **Status:** 🟢 Complete
- **Scope:** Reduce control-plane host privilege by moving docker-socket-dependent actions into constrained runner workflows.
- **Dependencies:** Upgrade/backup runner architecture, container/runtime policy.
- **Risks:** Operational complexity if migration is incomplete.
- **Acceptance:** Always-on control-plane no longer requires broad host-level container control.
- **Notes:** Control-plane now supports `remote` runner mode (`UPGRADE_RUNNER_MODE=remote`, `BACKUP_RUNNER_MODE=remote`) and calls a dedicated `maintenance-runner` API for upgrade/backup/restore execution. On-prem compose/templates now move `/var/run/docker.sock` to `maintenance-runner` only, removing docker-socket privilege from the always-on control-plane service.

#### Auth brute-force and abuse controls
- **Status:** 🟢 Complete
- **Scope:** Add login rate limiting and progressive lock/backoff for repeated failures.
- **Dependencies:** Auth middleware and audit integration.
- **Risks:** False positives impacting legitimate users.
- **Acceptance:** Repeated credential attacks are throttled and visible in audit/metrics.
- **Notes:** Implemented for local auth with per-IP login RPM limiter (`AUTH_LOGIN_RPM`) plus progressive per-identity backoff (`AUTH_LOGIN_BACKOFF_*`) and `Retry-After` responses; failed/blocked login attempts are audited.

#### Artifact signature enforcement by policy
- **Status:** 🟢 Complete
- **Scope:** Make signature verification default-required for production agent profiles, with explicit emergency override only.
- **Dependencies:** Agent config profiles, deployment docs.
- **Risks:** Older unsigned pipelines break until signing is adopted.
- **Acceptance:** Production agents reject unsigned artifacts by default.
- **Notes:** Control-plane now supports signature policy defaults (`ARTIFACT_SIGNATURE_REQUIRE_DEFAULT`, `ARTIFACT_SIGNATURE_KEY_ID`) that are merged into component `applyPolicy` at check-in time, so agents enforce signature verification without per-device env tuning. Optional ingest-time enforcement (`ARTIFACT_SIGNATURE_ENFORCE_INGEST=1`) blocks unsigned/wrong-key artifact registration in control-plane paths (`create`, `upload`, `pull`, `complete`).

#### Trusted proxy allowlist tightening
- **Status:** 🟢 Complete
- **Scope:** Validate and constrain `TRUST_PROXY_CIDRS`; block wildcard/overbroad trust in hardened deployments.
- **Dependencies:** Config validation, deployment runbooks.
- **Risks:** Incorrect CIDRs can break real client IP/cert forwarding.
- **Acceptance:** Forwarded headers are only honored from explicitly trusted proxy ranges.
- **Notes:** Hardened startup now rejects insecure/wildcard proxy trust ranges, deploy templates default to explicit loopback/private CIDRs, and deployment docs now call out the on-prem/cloud trust model.

---

## Phase C — Enterprise Readiness
**Goal:** Pass security and identity reviews.

### Definition of Done
- Identity is externally managed
- Privileged access is auditable
- Access control is least-privilege

### Feature Templates
#### Fixed RBAC roles
- **Status:** 🟢 Complete
- **Scope:** Admin / Operator / Viewer.
- **Dependencies:** Auth middleware + policy checks.
- **Risks:** Role creep.
- **Acceptance:** Endpoints and UI gated correctly.
- **Notes:** API role enforcement is in place and backend matrix tests cover representative viewer/operator/admin routes plus `artifact.publish` service-token publish paths. Browser parity shipped via centralized `buildPermissionState` helper and role-gated nav/action controls. Regression suite validates parity end-to-end.

#### Role-aware UI
- **Status:** 🟢 Complete
- **Scope:** Action gating; read‑only views.
- **Dependencies:** RBAC roles.
- **Risks:** Inconsistent UI behavior.
- **Acceptance:** UI hides actions for non‑authorized roles.
- **Notes:** Browser parity shipped. Centralized `buildPermissionState` drives `visibleNav`, `visibleLogsTabs`, and mutating action gating across all views. `firstAllowedKey` fallback ensures unauthorized roles land on a permitted view. Regression coverage added in `rbac.test.js`.

#### Custom RBAC policies
- **Status:** ⬜ Planned
- **Scope:** Per‑resource permissions.
- **Dependencies:** Policy engine; admin UI.
- **Risks:** Complexity / misconfig.
- **Acceptance:** Custom policies enforce least privilege.
- **Notes:** 

#### Release channels (customer-defined canary/stable)
- **Status:** ⬜ Planned
- **Scope:** Customer-owned channel labels and promotion workflow (for environments that want staged channel operations).
- **Dependencies:** Desired-state/channel model, policy/audit controls, optional health/soak integration.
- **Risks:** Mis-targeted promotions and policy drift across customers.
- **Acceptance:** Controlled staged promotions with explicit visibility and rollback path.
- **Notes:** Deferred from Phase B to Phase C because rollout policy is customer-specific.

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
- **Status:** 🟢 Complete
- **Scope:** Bootstrap admin + forced rotation + audit trail.
- **Dependencies:** Audit logging, auth.
- **Risks:** Misuse.
- **Acceptance:** Emergency access is possible and auditable.
- **Notes:** Bootstrap admin + voucher onboarding are in place. Break-glass service-token revoke/rotate and cert reload/rotate/cleanup APIs require operator-authenticated reason capture and emit success/denied/error audit records.

#### Agent first-contact approval onboarding (no pre-shipped client cert)
- **Status:** 🟢 Complete
- **Scope:** Let agents start without a device client cert, request enrollment, wait in a pending queue, and only receive signed device certs after explicit operator approval.
- **Dependencies:** Enrollment token/profile model, pending-enrollment API/UI, CSR signing pipeline, rate limits/abuse controls, audit events.
- **Risks:** Enrollment spam, spoofed first-contact metadata, and insecure bootstrap if server trust is not established.
- **Acceptance:**
  - Agent can bootstrap with only control-plane URL + enrollment profile/token (no preloaded client cert/key).
  - Agent generates keypair locally and submits CSR; private key never leaves device.
  - Control-plane creates a pending enrollment record; operator can approve/deny in UI/API.
  - On approval, control-plane signs CSR and returns cert chain; agent transitions to normal mTLS check-in flow.
  - All actions (request/approve/deny/issue) are audited and subject to license/device-cap checks.
- **Notes:** Full API + agent bootstrap state machine shipped and validated. Enrollment profile CRUD, pending-enrollment request/approve/deny/reset/claim flows, anti-spam controls, and Security UI are all in place. Go agent implements `AGENT_ENROLL_MODE=approval` with persisted `bootstrap-state.json`, CSR request/claim polling, identity materialization, reenroll CA persistence, and profile-label-driven desired-state resolution. Production closeout complete: packaged installer (`scripts/agent-install.sh`) is now self-contained with auto-discovery of template paths, `upsert_env`/`remove_env` helpers, and approval-mode configuration; agent-host gateway routes expose pending-enrollment request and claim endpoints; prod-docker-lab uses deterministic compose subnet with matching trusted proxy CIDRs; `scripts/testing/prod-lab-first-contact.sh` automates the full create-profile → install-agent → approve-request → verify-identity → active-check-in flow. Operator runbook and install docs updated. Full prod-docker-lab first-contact approval validation passed end-to-end.

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
- **Notes:** Terraform scaffold + CLI/runbook shipped (`deploy/aws/terraform`, `scripts/aws-customer.sh`). Hardened Terraform path now includes least-privilege IAM task roles, WAF web ACL on app ALB, and split ingress CIDRs. Acceptance gate helper `scripts/aws-hardening-check.sh` (config + deployment modes) and rewritten `aws-customer-deployment-runbook.md` shipped. Remaining: CloudWatch alarm Terraform resources, plaintext secret elimination in `customer_stack`, and live-deployment acceptance gate execution against a real environment.

#### IAM integration
- **Status:** 🟢 Complete
- **Scope:** S3 + KMS access control.
- **Dependencies:** AWS IAM design.
- **Risks:** Over‑permissioned roles.
- **Acceptance:** Least‑privilege IAM policies validated.
- **Notes:** Least-privilege IAM shipped in Terraform. ECS task-execution role scoped to specific Secrets Manager ARNs and optional KMS key ARNs. Task role scoped to specific CloudWatch log group ARNs, S3 artifact bucket prefixes, and parameterized ECS Exec (SSM/logs). IAM resource ARNs computed via `aws_partition` data source for GovCloud compatibility. Terraform `validate` passed across dev/staging/prod environments.

#### Network isolation guidance
- **Status:** 🟡 In progress
- **Scope:** VPC / private subnet patterns.
- **Dependencies:** AWS architecture.
- **Risks:** Misconfigured routes.
- **Acceptance:** Reference diagram + sample config.
- **Notes:** Private subnet model documented. Ingress CIDR split now in place: separate app-facing (operator browser) and device-facing (agent mTLS) ingress trust boundaries in Terraform variable model. WAFv2 web ACL attached to app ALB only (device ingress intentionally kept separate). Remaining: full reference VPC diagram and finalized sample variable configurations.

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
- **Notes:** IAM roles, WAFv2, ingress CIDR split, acceptance gate, and operator runbook shipped in the March 2026 IAM/WAF batch. Plaintext `DATABASE_URL` eliminated (#18); `enable_execute_command` default enforced to `false` (#19); CloudWatch alarm resources added to Terraform (#17). Remaining residual: live-deployment acceptance gate execution against a real AWS environment.

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
