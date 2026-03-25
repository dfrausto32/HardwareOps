# Product Development Roadmap & Progress Tracker

This document is the **authoritative internal roadmap** for the platform. It is designed to:
- Track development progress over time
- Communicate current and future capabilities to customers
- Support sales and stakeholder conversations with concrete status

The roadmap is organized by **maturity phases**, not deadlines. Phases are sequential, but work may overlap.

---

## Program Snapshot (Parallel Session Staging)
_Updated: 2026-03-12 (post CI workload identity AWS + GitHub validation)_

Use this section as the single source of truth for "what is done" vs "what is left."

### Phase-level status
| Phase | Status | Summary |
|---|---|---|
| Phase A — Foundation | 🟢 Complete | Core deployability, signing, upgrades, cert rotation, licensing, backup/restore are in place. |
| Phase B — Operational Maturity | 🟢 Complete | Audit/metrics/events/lifecycle/CI ingest/bulk ops shipped and operational. |
| Phase B Extension — UX + Realtime | 🟢 Complete | Bulk actions + realtime updates + auth-session UX reset shipped. |
| Operational Hardening (between B and C) | 🟢 Complete | Pull-boundary, token exposure, startup guardrails, break-glass backend, proxy trust policy, and abuse controls are all shipped. |
| Phase C — Enterprise Readiness | 🟢 Complete | Fixed RBAC, role-aware UI parity, break-glass APIs, first-contact approval onboarding, OIDC SSO, trusted-key artifact verification, trusted-key deployment wiring, trust-override UX, artifact tracking policies, the local-auth recovery stack (recovery codes, reset tokens, break-glass CLI), CI workload identity federation, supply-chain provenance policy (Cosign/Sigstore), LDAP/AD auth, and Vault secrets integration are all shipped. |
| Phase D — Scale & Cloud Optionality | 🟡 In progress | AWS reference deployment and least-privilege IAM shipped. WAF attached; ingress CIDR split in place. Acceptance runbook and gate script created. Plaintext DATABASE_URL eliminated; ECS exec off by default; CloudWatch alarms Terraform-managed. Connected email delivery complete. Remaining Phase D work is live-deployment acceptance gate execution (operational) and full VPC reference diagram (docs). |
| Phase E — Federated Multi-Region | 🟡 In progress | Hub-and-spoke federation layer: global management plane above regional control planes. Agents unchanged. E1 backend complete (global-plane binary, sync manager, REST API, migrations). UI and smoke test remaining. |

### Active work queue (what is still to do)

No items currently in-flight. Queue is clear.

### Prepared next tasks (agent-scoped)
- `E2-ARTIFACT-FEDERATION` — single artifact upload to global plane; metadata push to regional planes via federation endpoint; MinIO bucket replication tracking; per-artifact per-region replication status in global UI

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
- ✅ OIDC SSO (authorization code flow; `go-oidc/v3` + `oauth2`; IdP discovery; group-to-role mapping; user upsert on `auth_provider`+`external_id`; state cookie; `auth.oidc.login` audit events; UI SSO button; hardened-profile guards; per-IdP docs for Okta/Azure AD/Google Workspace).
- ✅ Trusted-key artifact verification core (trusted signing key registry; global verification policy; control-plane cryptographic verification for create/upload/pull/complete; agent trust bundle distribution and apply-time re-verification; Security-page trust controls; artifact verification state/metrics/tests).
- ✅ Trusted-key deployment wiring and AWS validation (startup seeding from `TRUSTED_SIGNING_KEYS_*` / `ARTIFACT_TRUST_*`; on-prem and AWS deployment templates updated; AWS `parcel/dev` bootstrapped from Secrets Manager; `scripts/test-artifact-trust.sh` validates unsigned reject / signed accept / wrong-key reject against the live stack).
- ✅ Trust-override UX in desired-state editors (group, device, and multi-group desired-state rows can open a component trust-override modal; operators can set verification mode, allowed signature types, and allowed key IDs with inherited/effective-policy previews; artifact pickers and version selectors enforce the effective trust policy inline).
- ✅ Password recovery layers 1–2 (self-service recovery codes plus admin-issued one-time reset tokens with audit coverage and login-screen redemption flows).
- ✅ Password recovery layer 3 (host-local break-glass CLI for emergency local-admin password reset and recovery-admin creation with explicit audit reason capture).
- ✅ CI workload identity federation (GitHub/GitLab/Jenkins helper flows, AWS/on-prem provider config wiring, live AWS provider bootstrap, and end-to-end GitHub Actions OIDC exchange plus signed artifact upload validation against the strict trusted-key policy).
- ✅ Artifact tracking policies (explicit group/device/component auto-follow targets, immediate reconcile on desired-state save, trust-aware eligibility, artifact-family visibility in the UI, device-scoped artifact retrieval for assigned artifacts, and live AWS validation with a local agent applying `trackingdemo` 1.3.0 end-to-end).
- ✅ Connected email delivery for auth recovery (SMTP mailer with STARTTLS/TLS/plain; forgot-password, admin-issued reset, and user-invite flows; `smtpEnabled` on auth status; SMTP vars in all deployment templates; SMTP password via Secrets Manager on AWS; operator runbook and full config reference docs).
- ✅ CI/CD feedback loop (outbound webhooks with HMAC-SHA256, deploy triggers with `immediateRecheckin`, deployment status polling endpoint, scoped service tokens for all CI integration patterns, ICD documentation, and end-to-end smoke test script).

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

#### Artifact auto-version tracking (v1)
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
- **Notes:** This is the shipped core. The next follow-on item is to productize this as an explicit "artifact tracking policy" model with clearer UX, stronger visibility on the artifacts page, and tighter trust/eligibility controls.

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
- **Notes:** Presigned CI push path implemented (`/artifacts/presign-upload` + `/artifacts/complete`) with scoped expiring service tokens (`artifact.publish`) and `scripts/ci-upload-artifact.sh`. Pull ingest API implemented (`/artifacts/pull`) with checksum validation + host/size/timeout guardrails plus CI helper `scripts/ci-pull-artifact.sh`. Adapter framework and backward-compatible source shape are in place (`sourceUrl` or `source.kind` + `source.uri`; current kinds=`http`,`artifactory`). Credential resolver abstraction is in place (`source.credentialRef`) with static map (`ARTIFACT_PULL_CREDENTIALS_FILE`/`ARTIFACT_PULL_CREDENTIALS_JSON`) plus AWS Secrets Manager-backed loading (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`). Pull resolver operational workflow is implemented with admin status/reload endpoints (`GET/POST /api/v1/artifacts/pull-credentials*`) and helper script (`scripts/reload-pull-credentials.sh`). Artifactory adapter shipped with local docker smoke scripts (`scripts/setup-artifactory-demo.sh`, `scripts/test-artifactory-adapter.sh`) and signed artifact demo flow. Smoke coverage now validates push + pull + pull-credential reload audit path in `scripts/test-artifact-ingest.sh` (see `artifact-ingest-test-plan.md`). CI provider scaffold templates added for GitHub Actions/GitLab/Jenkins (`../../deploy/ci/README.md`). Phase C adds workload identity exchange for OIDC job tokens (`/api/v1/auth/workload-identity/exchange`) plus the GitHub Actions helper flow in `scripts/ci-exchange-workload-identity.sh`. Vault resolver backend is deferred to Phase C.

### Recommended Next Sequence (Pending Prioritization)
The 2026-03-10 D-hardening batch merged. Items 1–3 below are now complete. Remaining candidates:
1. ~~**Plaintext secret elimination**~~ — ✅ Done (#18)
2. ~~**ECS exec guardrails**~~ — ✅ Done (#19)
3. ~~**CloudWatch alarm Terraform resources**~~ — ✅ Done (#17)
4. ~~**OIDC SSO**~~ — ✅ Done (#23)
5. **Custom RBAC policies (Phase D):** Per-resource permission engine and admin UI.
6. ~~**Trusted-key management deployment wiring**~~ — ✅ Done
7. ~~**Trust override UI**~~ — ✅ Done
8. ~~**CI workload identity federation**~~ — ✅ Done

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

#### Trusted-key artifact verification
- **Status:** 🟢 Complete
- **Scope:** Trusted signing key registry, global verification policy (`allow_unsigned` / `warn_unsigned` / `require_verified`), control-plane cryptographic verification on ingest, agent trust-bundle distribution, and apply-time verification.
- **Dependencies:** Artifact ingest/update paths, desired-state policy merge, agent signature verification, admin Security UI.
- **Risks:** Policy drift between deployments, operator confusion around key rotation, and rollout friction for legacy unsigned artifacts.
- **Acceptance:** 
  - Control-plane verifies signed artifacts against active trusted public keys before registration.
  - Agents persist the distributed trust bundle and re-verify before apply.
  - Desired-state enforcement blocks non-compliant artifacts under strict policy.
  - UI exposes verification state, trusted keys, and global trust policy.
- **Notes:** Core slice is shipped for `ed25519` and key-based `cosign`. Existing artifacts are marked `legacy`. Follow-on work is trust-override UX and later provenance/keyless policy.

#### Trusted-key management deployment wiring
- **Status:** 🟢 Complete
- **Scope:** Wire trusted key registry and trust-policy defaults into on-prem compose/env examples, AWS Terraform/runtime config, seed scripts, and operator runbooks.
- **Dependencies:** Trusted-key verification core, deployment templates, installer/bootstrap scripts, AWS/on-prem docs.
- **Risks:** Drift between local/dev/on-prem/cloud defaults and brittle first-run setup.
- **Acceptance:** 
  - On-prem and AWS reference deployments can bootstrap trusted keys and initial trust policy without manual DB surgery.
  - Seed/runbook path exists for adding initial trusted keys and rotating them safely.
  - Smoke tests cover strict and permissive deployment profiles.
- **Notes:** Shipped: control-plane startup seeds DB policy/registry from `ARTIFACT_TRUST_*` + `TRUSTED_SIGNING_KEYS_*` bootstrap config; prod-lab generates `trusted-signing-keys.json`; on-prem compose/examples, installer/upgrade templates, and AWS Terraform expose trusted-key bootstrap wiring; AWS `parcel/dev` is validated end-to-end with a Secrets Manager-backed trusted key and `scripts/test-artifact-trust.sh`.

#### Trust override UI (group/device policy overrides)
- **Status:** 🟢 Complete
- **Scope:** Expose group/device/component trust-policy overrides in desired-state editors with clear precedence over the global default.
- **Dependencies:** Trusted-key verification core, desired-state policy model, UI editor components.
- **Risks:** Operators weakening policy unintentionally or not understanding precedence.
- **Acceptance:** 
  - Group/device/component editors can set verification mode and allowed key/type overrides.
  - UI clearly shows inherited vs overridden trust policy.
  - Non-compliant selections are blocked or explained inline.
- **Notes:** Shipped: device, group, and multi-group desired-state editors expose a `Trust` action per component row; the modal supports verification mode, allowed signature types, and allowed signing key IDs; inline trust summaries show inherited/effective policy; artifact selection and version pickers filter against the effective trust policy.

#### Artifact tracking policies (auto-follow latest eligible artifact)
- **Status:** 🟢 Complete
- **Scope:** Promote the shipped auto-version resolver into a first-class tracking policy model for groups, devices, and individual desired-state components.
- **Dependencies:** Existing `autoVersion` resolver, desired-state editors, trust policy model, artifact list visibility, and audit trail.
- **Risks:** Operator confusion if tracking state and pinned state are not clearly separated; unexpected upgrades if eligibility rules are too broad.
- **Acceptance:**
  - Group/device/component desired-state editors expose a first-class tracking control, not just a low-level `autoVersion.mode`.
  - Tracking policy is explicitly based on `name + type + semver + eligibility` and only moves forward to the latest eligible artifact.
  - Eligibility integrates with trusted-key policy (`active`, verification mode, allowed signing keys/types).
  - Artifacts page shows whether artifacts are tracked, eligible, or excluded and why.
  - Reconciliation still runs on ingest and on schedule, with auditable desired-state updates.
- **Notes:** Shipped: explicit tracking controls in group/device/multi-group desired-state editors; exact `name + type` targeting; immediate reconcile on save; trust-aware eligibility; artifact-family tracking detail panel in the UI; and device-scoped artifact metadata/presign routes so assigned agents can apply tracked artifacts end-to-end. Classical canary/stable promotion remains deferred because it is customer-policy specific.

#### OIDC SSO
- **Status:** 🟢 Complete
- **Scope:** Authorization code flow; group-to-role mapping; user upsert; audit events; UI SSO button.
- **Dependencies:** IdP config; callback endpoints.
- **Risks:** IdP misconfiguration.
- **Acceptance:** Users can log in via OIDC and get correct roles.
- **Notes:** `go-oidc/v3` + `golang.org/x/oauth2` for IdP discovery and token exchange. `GET /api/v1/auth/oidc/login` → IdP redirect; `GET /api/v1/auth/oidc/callback` → code exchange → internal JWT issued. Group-to-role mapping via `AUTH_OIDC_ROLE_MAP` JSON; unmapped users get `AUTH_OIDC_DEFAULT_ROLE` (default `viewer`). State cookie (`hwops_oidc_state`, HttpOnly, 600s TTL). Users upserted on `auth_provider`+`external_id` (DB columns already existed). `auth.oidc.login` and `auth.oidc.login.failed` audit events. UI renders SSO button when `authStatus.oidcEnabled`. Hardened profile rejects `AUTH_OIDC_DEFAULT_ROLE=admin` and empty role map. Per-IdP docs for Okta, Azure AD (Entra), and Google Workspace in `auth-secrets-v1.md`.

#### Password recovery / admin reset
- **Status:** 🟢 Complete (local / airgapped)
- **Scope:** Local-auth password recovery path for locked-out operators in dev, on-prem, and airgapped deployments, including recovery codes, operator-assisted reset, emergency break-glass reset, and audit coverage.
- **Dependencies:** Local auth mode, user management APIs, break-glass/admin recovery policy, deployment docs.
- **Risks:** Weak recovery flow becoming an account-takeover path; undocumented operator steps causing outage during lockout.
- **Acceptance:** 
  - Recovery codes work without SMTP/email and do not require direct DB edits.
  - Operators can issue a one-time reset token for a local user in airgapped/on-prem deployments.
  - Emergency break-glass local reset path is explicit, gated, and fully audited.
  - Recovery path is auditable end-to-end across UI/API/CLI.
- **Notes:** Implemented for local/airgapped deployments first. Recovery codes, operator-issued reset tokens, and host-local break-glass CLI recovery are shipped. Connected email delivery is explicitly deferred to Phase D so it does not block enterprise identity and policy work.
- **Phase C design:**
  - **Layer 1 — Recovery codes:** self-service one-time codes generated per user, stored only as hashes, downloadable once, usable from the login screen.
  - **Layer 2 — Operator reset token:** local admin can generate a short-lived one-time reset token for a target user and hand it to them out of band.
  - **Layer 3 — Break-glass local reset:** host-local CLI can reset an admin password or create a recovery admin when normal auth paths are unavailable.
  - **Layer 4 — Email delivery:** deferred to Phase D for connected deployments; will reuse the same backend token semantics as operator-issued reset.
- **Subtasks:**
  - **C-PASSWORD-RECOVERY-1:** ✅ Recovery codes backend + UI (`POST /api/v1/auth/recovery-codes/generate`, `POST /api/v1/auth/recovery-codes/reset`, download/copy UX, audit events).
  - **C-PASSWORD-RECOVERY-2:** ✅ Operator-issued reset token backend + admin UI (`POST /api/v1/users/{userId}/password-reset-token`, `POST /api/v1/auth/password-reset/complete`, TTL, reason, one-time use, audit events).
  - **C-PASSWORD-RECOVERY-3:** ✅ Break-glass local CLI reset/create-admin path for total lockout recovery (`control-plane auth breakglass reset-password ...`, `control-plane auth breakglass create-admin ...`) with required reason and audit event.
  - **C-PASSWORD-RECOVERY-4:** Deferred to Phase D as `D-PASSWORD-RECOVERY-EMAIL`.

#### CI workload identity federation
- **Status:** 🟢 Complete
- **Scope:** OIDC-based CI auth (GitHub/GitLab/Jenkins) for publish/pull without static secrets.
- **Dependencies:** OIDC trust config, service-token exchange endpoint/policy.
- **Risks:** Misconfigured trust policies.
- **Acceptance:** CI jobs can obtain short-lived publish credentials by identity, no long-lived credential files required.
- **Notes:** Generic workload identity exchange is shipped for OIDC job tokens via `POST /api/v1/auth/workload-identity/exchange`, with config-driven issuer/audience/claim matching and short-lived internal bearer tokens carrying `artifact.publish`. First-class helper flows are shipped for GitHub Actions (`scripts/ci-exchange-workload-identity.sh`), GitLab (`scripts/ci-exchange-gitlab-workload-identity.sh`), and Jenkins (`scripts/ci-exchange-jenkins-workload-identity.sh`) with updated scaffold templates. Deployment wiring supports file/JSON or AWS Secrets Manager-backed provider config plus admin status visibility (`GET /api/v1/auth/workload-identity/status`) and smoke validation (`scripts/test-workload-identity.sh`). Live acceptance is now validated on AWS `parcel/dev` with a real GitHub Actions OIDC token exchange and signed artifact upload under strict trusted-key policy.

#### Supply-chain provenance policy (Cosign/Sigstore)
- **Status:** 🟢 Complete
- **Scope:** Optional attestations/provenance verification policy in addition to Ed25519 signature checks.
- **Dependencies:** CI provenance generation, policy model, verification pipeline.
- **Risks:** Operational complexity and false rejects.
- **Acceptance:** Policy can enforce trusted builder/provenance on selected artifact classes.
- **Notes:** Implemented in migration `0028_artifact_attestations.sql` and packages `artifacttrust/provenance.go` + `trust.go`. Keyless cosign verification uses Fulcio-issued ECDSA certs (OID extensions + URI SANs for builder identity) and optional Rekor transparency log. In-toto attestations stored in `artifact_attestations` table via `POST /api/v1/artifacts/{id}/attestations` (keyless bundle auto-verified on upload). Provenance policy enforced at desired-state set time via `CheckProvenancePolicy`. Global trust policy extended with `provenance` JSON block configurable via `PUT /api/v1/artifact-trust/policy`. Configurable via `ARTIFACT_FULCIO_ROOT_CERT`, `ARTIFACT_REKOR_URL`, `ARTIFACT_REQUIRE_REKOR_LOG`.

#### Vulnerability scanning integration (optional, Tenable Nessus)
- **Status:** 🟢 Complete
- **Scope:** Two-track integration: (1) artifact-level scanning via Grype or Trivy CLI at ingest time; (2) device-level scanning via Nessus/Tenable API sync. Both are read-only visibility with full UI integration.
- **Dependencies:** Grype/Trivy binary in PATH (artifact scanning) or Tenable API credentials (device scanning). Both are independently optional; set `VULN_ARTIFACT_SCANNER=disabled` and leave `VULN_NESSUS_URL` blank to disable entirely.
- **Implementation:** `internal/vulnscan/` package (scanner interface, GrypeScanner, TrivyScanner, ArtifactScanJob, NessusClient, NessusSyncJob); migration 0029; 7 new API endpoints; artifact + device UI views; settings page sync controls.
- **Notes:** Non-blocking for deployments (read-only). Device matching via `hwops.network.hostname` / `hwops.network.ip` metadata fields. See `docs/vulnerability-scanning.md`.

#### Cloud-native pull adapters (S3/GCS)
- **Status:** 🟢 Complete
- **Scope:** Native `source.kind = "s3"` and `source.kind = "gcs"` adapters for pull ingest alongside the existing `http` and `artifactory` adapters. Both adapters integrate with the `CredentialResolver` system so credentials are resolved by ref (static file, AWS Secrets Manager, Vault) rather than being inlined in the pull request.
- **Dependencies:** AWS SDK v2 (`aws-sdk-go-v2/service/s3`); `golang.org/x/oauth2/google` for GCS OAuth2. Both already present in go.mod. IAM role / IRSA / GKE Workload Identity credential chain supported without any credential store entry.
- **Risks:** Credential misconfiguration could grant broad bucket access — mitigated by the existing `source.credentialRef` abstraction and audit trail. S3 bucket-policy ACLs and GCS IAM bindings remain the operator's responsibility.
- **Acceptance:**
  - `POST /api/v1/artifacts/pull` with `source.kind = "s3"` and `source.uri = "s3://bucket/key"` pulls the artifact bytes, verifies SHA-256 and signature, stores in MinIO, and emits audit events identically to the HTTP adapter.
  - `POST /api/v1/artifacts/pull` with `source.kind = "gcs"` and `source.uri = "gs://bucket/object"` (or `gcs://`) behaves identically.
  - Static AWS credentials (`access_key_id` / `secret_access_key` / `session_token`) and region override supplied via `credentialRef` entries; empty credential map falls back to the SDK default chain (IRSA / ECS task role / instance profile).
  - GCS supports `oauth_token` (pre-obtained bearer), `service_account_json` (full SA key file), or Application Default Credentials (GKE Workload Identity / GCE instance service account / `GOOGLE_APPLICATION_CREDENTIALS`).
  - Checksum mismatch, oversized artifacts, and S3/GCS request errors are surfaced as 400/502 with audit events.
- **Notes:** Implemented in `internal/artifactingest/s3_pull_adapter.go` and `gcs_pull_adapter.go` following the `PullAdapter` interface pattern. Both adapters use injectable client factories for unit-testable mocking without live cloud credentials. GCS uses the XML storage API (`GET https://storage.googleapis.com/{bucket}/{object}`) with OAuth2 Bearer token, avoiding the heavyweight Google Cloud Storage SDK. S3 uses `aws-sdk-go-v2/service/s3.GetObject` with the standard credential chain. Registered alongside HTTP/Artifactory in `handlers.pullArtifact`. URI parsing is strict: non-`s3://` URIs are rejected by the S3 adapter; non-`gs://`/`gcs://` URIs are rejected by the GCS adapter.

#### LDAP / AD support
- **Status:** 🟢 Complete
- **Scope:** LDAP/AD authentication provider. Users authenticate with their directory username and password via a new `POST /api/v1/auth/ldap/login` endpoint. Roles mapped from LDAP group membership using a configurable group→role JSON map. User records upserted in the local store with `auth_provider = "ldap"` and `external_id = <user DN>`, enabling account linking and audit traceability. No SCIM or directory sync in this phase — auth-time upsert only.
- **Dependencies:** Enterprise customer LDAP/AD environment. `go-ldap/ldap/v3` library (MIT).
- **Risks:** Directory inconsistency (deprovisioned users still have cached store records); LDAP connection pooling and latency under load; AD vs. OpenLDAP filter differences; TLS cert validation for LDAPS.
- **Acceptance:**
  - Users can authenticate with `POST /api/v1/auth/ldap/login` using their directory credentials and receive a JWT identical to local/OIDC tokens.
  - LDAP group membership is mapped to HardwareOps roles via `AUTH_LDAP_ROLE_MAP` (same JSON map pattern as OIDC).
  - Failed login emits an audit event; successful login updates `last_login_at`.
  - Disabled LDAP users (store `disabled = true`) are rejected regardless of directory state.
  - TLS/LDAPS connections are supported; plain LDAP is supported for dev.
  - UI shows LDAP login option when `AUTH_LDAP_URL` is configured.
- **Notes:** Follows the OIDC provider pattern (`auth/oidc.go`): separate `LDAPProvider` struct, initialized in `main.go` when `AUTH_LDAP_URL` is set, wired into `httpapi.Dependencies`. User lookup uses `store.GetUserByExternalID("ldap", userDN)` → fall back to email → create. JWT issuance via `manager.IssueToken()`. No recovery codes issued for LDAP accounts. Directory deprovisioning not enforced at check-in time (out of scope for initial cut — covered by disabling the store record manually or via admin API).

#### Secrets management integration
- **Status:** 🟢 Complete
- **Scope:** HashiCorp Vault KV resolver backend for artifact pull credentials, extending the existing `CredentialResolver` interface. Operators configure Vault address, auth method (token or AWS IAM), and KV path; control-plane fetches pull credentials from Vault at startup and on operator-triggered reload. Reload surfaced in existing `GET /api/v1/pull-credentials/status` and `POST /api/v1/pull-credentials/reload` endpoints.
- **Dependencies:** Vault server accessible from control-plane; `hashicorp/vault/api` Go SDK. AWS Secrets Manager resolver already in place as the pattern reference.
- **Risks:** Vault unavailability blocks credential reload (but does not break in-flight operations already holding resolved credentials); token expiry if Vault token auth is used without renewal; secret path misconfiguration causes silent empty credential set.
- **Acceptance:**
  - Setting `ARTIFACT_PULL_CREDENTIALS_VAULT_ADDR` + `ARTIFACT_PULL_CREDENTIALS_VAULT_PATH` + auth method vars causes control-plane to fetch pull credentials from Vault KV on startup.
  - `GET /api/v1/pull-credentials/status` reports `vault_backed: true` and Vault connection health.
  - `POST /api/v1/pull-credentials/reload` triggers Vault re-fetch and returns updated status.
  - Vault backend is mutually exclusive with AWS Secrets Manager backend (config validation enforces this).
  - No long-lived static credentials required in environment when Vault is configured.
- **Notes:** AWS Secrets Manager-backed pull credential resolver is in place for CI pull ingest (`ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`) from Phase B. Phase C adds `credentials_vault.go` implementing `CredentialResolver` using the same pattern as `credentials_aws_sm.go`. Vault token auth is the initial auth method; AWS IAM auth (Vault AWS auth backend) is a natural follow-on. KV v2 API only. Rotation runbook: operators rotate Vault secret, then call `POST /api/v1/pull-credentials/reload` — no control-plane restart required.

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

#### Connected email delivery for auth recovery/setup
- **Status:** 🟢 Complete
- **Scope:** Optional SMTP/provider-backed delivery for password reset and account setup in connected deployments.
- **Dependencies:** Password reset token model, deployment-specific mail configuration, public app URL, audit coverage.
- **Risks:** User-existence leakage, mail delivery drift across environments, and secret handling for SMTP/provider credentials.
- **Acceptance:**
  - Connected deployments can send password-reset/account-setup emails without changing the core reset-token model.
  - Email request flow does not reveal whether a user exists.
  - Delivery failures and successful sends are auditable.
  - On-prem/airgapped deployments remain fully functional with email disabled.
- **Notes:** SMTP mailer package shipped with STARTTLS/TLS/plain modes, stdlib-only (no third-party mail library). `POST /api/v1/auth/forgot-password` (always-200, no user-existence leak), `POST /api/v1/users/{userId}/invite` (72-hour setup link), and `sendEmail: true` on `POST /api/v1/users/{userId}/password-reset-token` all wired. `smtpEnabled` field on `GET /api/v1/auth/status` gates UI forgot-password tab. SMTP vars in all deployment templates (on-prem compose, control-plane env example, AWS Terraform customer_stack and tfvars examples). `SMTP_PASSWORD` injected via ECS Secrets Manager `valueFrom` on AWS. `SMTP_SKIP_VERIFY` blocked by hardened profile. Audit events record `emailFound` and `emailSent` without user ID. Operator runbook in `docs/customer/security-and-recovery.md`; full config reference in `docs/email-delivery.md`.

#### Multi-tenant controls (optional)
- **Status:** ⬜ Backlog
- **Scope:** Tenant isolation model.
- **Dependencies:** Auth/RBAC.
- **Risks:** Data leakage between tenants.
- **Acceptance:** Tenant boundary enforcement + tests.
- **Notes:** 

---

## Phase E — Federated Multi-Region
**Goal:** Enable operators to manage devices across many geographic regions from a single global control plane, while keeping each regional plane fully autonomous.

### Definition of Done
- Operators can view device health, artifact inventory, and audit events across all registered regional planes from a single UI
- Artifacts uploaded once to the global plane are replicated to and served from regional MinIO instances
- Operators can push desired state policies globally; regional planes apply them with local-override capability
- Regional planes continue operating fully (agent check-ins, desired state, cert rotation, enrollment) when the global plane is unreachable
- Agents require zero changes

### Architecture: Peer Coordinator Model

Regional planes are first-class standalone control planes. The global plane is a coordinator and aggregator — additive, not load-bearing.

```
┌───────────────────────────────────────────────┐
│              Global Control Plane              │
│  - Artifact registry (canonical source)        │
│  - Global group policies + desired state       │
│  - Cross-region device directory (read cache)  │
│  - Aggregated audit log, health metrics        │
│  - User management (global operators)          │
└──────────┬───────────────────┬─────────────────┘
           │  service tokens   │  (async sync)
    ┌──────▼──────┐     ┌──────▼──────┐     ...
    │  Regional   │     │  Regional   │
    │  Plane A    │     │  Plane B    │
    │  (full CP)  │     │  (full CP)  │
    └──────┬──────┘     └──────┬──────┘
       mTLS poll           mTLS poll
    devices/agents       devices/agents
```

Inter-plane authentication uses the existing service token mechanism (`federation.push` scope for regional→global telemetry; `federation.manage` scope for global→regional policy push). No new auth protocol required.

### Feature Templates

#### E1 — Global aggregation plane (read-only foundation)
- **Status:** 🟢 Complete
- **Scope:** New `global-plane` binary that registers regional control planes and aggregates their data via existing read-only API endpoints. Unified UI showing cross-region device list, regional health cards, artifact inventory, and aggregated audit feed.
- **Dependencies:** Service tokens on regional planes; new `regional_planes` and `device_directory_cache` tables on global DB.
- **Risks:** Regional plane API version skew; stale cache if sync goroutine falls behind.
- **Acceptance:** Operator can register N regional planes and see a unified device list and health summary without opening N browser tabs. Zero changes to agents or regional control planes.
- **Notes:** `cmd/global-plane` binary, `internal/globalplane` package (store, config, AES-256-GCM token encryption), `internal/globalplane/sync` (Manager + PlaneWorker with exponential backoff), `internal/globalplane/httpapi` (router, planes/aggregated handlers), 5 SQL migrations (`migrations/global/`). `auth.AuthStore` interface extracted so the global-plane store satisfies it without importing the full regional store. Two new service token scopes: `federation.push` and `federation.manage`. UI: `GlobalPage.jsx` with Planes health cards, Devices table, Artifacts table, Register Plane modal, 30s auto-refresh; admin-only nav entry with RBAC test coverage. Operator runbook in `docs/global-plane.md`.

#### E2 — Artifact federation (single upload, N regions)
- **Status:** ⬜ Planned
- **Scope:** Artifact uploaded once to global plane; metadata pushed to all regional planes; blobs replicated via MinIO bucket replication. Regional planes serve artifacts from local MinIO to agents (presigned URLs unchanged). Per-artifact, per-region replication status tracked and surfaced in global UI.
- **Dependencies:** E1 (global plane registered); MinIO bucket replication configured per region; new `artifact_replication_status` table; new `POST /api/v1/federation/artifacts` endpoint on regional planes.
- **Risks:** Replication lag — agents may be assigned an artifact on a regional plane before the blob has arrived. Fallback: presigned URL points to global MinIO if regional copy not yet available.
- **Acceptance:** Operator uploads artifact once to global plane; artifact becomes available on all registered regional planes once replication completes. Global UI artifact detail shows "Available in X/N regions." Agents download from regional MinIO with zero changes.
- **Notes:** MinIO bucket replication is a built-in MinIO/S3 feature. The custom work is tracking replication state in PostgreSQL and exposing it in the UI. Regional planes should not serve agents a presigned URL for an artifact until the blob is confirmed locally.

#### E3 — Global desired state / policy push
- **Status:** ⬜ Planned
- **Scope:** Operators define global group policies from the global plane. Regional planes receive policies, cache them locally, and merge with local overrides (local override always wins). Global plane aggregates execution status across regions. If global plane is unreachable, regional planes continue applying last-cached global policies.
- **Dependencies:** E1 (global plane); new `global_policy_cache` table and `POST /api/v1/federation/policies` endpoint on regional planes; `global_groups` + `global_desired_state` tables on global DB.
- **Risks:** Conflict resolution confusion — operators must be able to see whether a device is running under global policy or a local override. Eventual consistency means global execution status view is delayed.
- **Acceptance:** Operator sets a global desired state for a group; regional planes apply it to matching devices within one check-in cycle; global UI shows per-region execution status (devices updated, pending, overridden). Regional operator can set a local override that takes precedence; global UI surfaces "local override active" for affected devices.
- **Notes:** Two-tier precedence mirrors the existing device-overrides-group model within a single control plane. Regional `desired_state_device` overrides always win over global policy. Conflict resolution must be visible in both global and regional UIs.

#### E4 — Global enrollment profiles
- **Status:** ⬜ Planned
- **Scope:** Enrollment profiles created on the global plane and pushed to regional planes. Enrollment still happens locally (regional CA issues cert; agent connects to regional plane). Global UI shows aggregated pending enrollment queues across all regions; operators can approve/deny from the global UI via proxied API call.
- **Dependencies:** E1 (global plane); E3 (federation push mechanism); regional planes accept enrollment profile push from global.
- **Risks:** Profile sync lag if regional plane is offline when profile is created/rotated. Regional planes must fall back to locally cached profiles.
- **Acceptance:** Operator creates one enrollment profile globally; it appears on all registered regional planes. Agent enrolls with regional plane (no change). Operator can see and approve pending enrollments from all regions in a single queue.
- **Notes:** Device certs are always issued by the regional plane CA. The global plane never touches the PKI layer in this phase. Profile IDs must be globally unique (use UUID generation at global plane).

#### E5 — Global PKI hierarchy (optional, advanced)
- **Status:** ⬜ Backlog
- **Scope:** Global root CA → regional intermediate CAs. Enables cross-region device identity verification without trusting all regional CAs independently.
- **Dependencies:** E1–E4; existing enrolled devices require reenrollment for new cert chain.
- **Risks:** CA migration is a breaking change for existing enrolled devices. Certificate chain complexity increases.
- **Acceptance:** New devices receive certs chained to global root CA. Cross-region device identity verifiable from global plane.
- **Notes:** Not required for E1–E4. Defer until there is a concrete operational need for cross-region device identity trust.

#### E6 — Global aggregate licensing
- **Status:** ⬜ Backlog
- **Scope:** Global license covering total device count across all registered regional planes, with per-region sub-cap allocation managed by global admin.
- **Dependencies:** E1 (global plane); license model redesign.
- **Risks:** Regional planes need connectivity to global plane for enrollment enforcement if global license is the authority. Conflicts with full regional autonomy requirement.
- **Acceptance:** TBD — requires product/commercial decision on license model for federated deployments.
- **Notes:** Per-instance device caps remain in effect for regional planes operating standalone. The global license layer is additive. One option: regional planes enforce their own local cap; global admin sets per-region caps that regional planes download and cache (same pattern as global policies).

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
