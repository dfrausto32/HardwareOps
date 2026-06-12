# Ransomware Protection Policy
**Version:** 1.0  
**Effective Date:** 2026-04-13  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-13)  
**Applies To:** All production Parcel platform environments (AWS-hosted and on-premises).

---

## 1. Purpose

This policy establishes the controls, detection mechanisms, and response procedures that protect the Parcel platform against ransomware attacks. Because Parcel manages artifact distribution to autonomous device fleets, a ransomware event carries two distinct blast radii: **control-plane data loss** (database, artifact store) and **fleet-wide operational impact** (devices unable to receive updates or configuration). Both must be addressed.

---

## 2. Threat Model

### 2.1 Attack Surfaces

| Surface | Ransomware Vector | Consequence |
|---------|------------------|-------------|
| Artifact store (MinIO / S3) | Compromised operator credentials used to overwrite or delete artifact objects | Devices unable to apply updates; rollback artifacts destroyed |
| PostgreSQL database | Credential theft → bulk table destruction or encryption | Complete loss of device state, desired state, audit records, user accounts |
| Control-plane process | Exploited RCE vulnerability gives attacker write access to both stores simultaneously | Combined data + artifact loss |
| Backup destination | Attacker reaches backup with same credentials used for primary store | Backup encrypted alongside primary — recovery path eliminated |
| Device fleet | Malicious artifact pushed via compromised `artifact.publish` service token | Ransomware binary deployed to all enrolled devices |

### 2.2 High-Value Targets in This Platform

- **Artifact binaries** — firmware, configuration bundles, and app packages stored in MinIO/S3
- **Desired state records** — which artifact version each device or group should run
- **Apply result history** — proof of what ran on each device and when
- **Enrollment credentials** — device certificates and bootstrap tokens enabling fleet re-enrollment
- **Audit logs** — evidence trail that a ransomware actor would prefer to destroy

---

## 3. Preventive Controls

### 3.1 Existing Controls (Baseline)

The following controls are already in place and contribute to ransomware resistance:

| Control | Location | Ransomware Relevance |
|---------|----------|----------------------|
| Ed25519 artifact signing (`require_verified` in hardened profile) | `artifacttrust/trust.go` | Prevents unsigned ransomware payload from being deployed to devices |
| Two-phase soft-delete with 30-day deprecation window | `migrations/0015_artifact_lifecycle.sql` | Provides recovery buffer before permanent deletion |
| Reference counting blocks deletion of active artifacts | `httpapi/handlers/artifacts.go:1383` | Prevents deletion of in-use artifacts without explicit force |
| RBAC: delete requires `operator`, prune requires `admin` | `httpapi/router.go:204-206` | Limits blast radius of a compromised lower-privilege account |
| Full structured audit log (actor, IP, before/after JSON) | `migrations/0010_audit_events.sql` | Forensic trail for post-incident scope determination |
| Login rate limiting with exponential backoff | `config/hardening.go` | Slows credential brute-force attacks |
| TOTP MFA for operator/admin accounts (hardened profile) | `config/hardening.go` | Reduces risk of credential-only account takeover |
| Remote backup runner with strong token (hardened profile) | `config/hardening.go:178` | Ensures backup destination is not the same credential as primary |

### 3.2 Required Controls (Gaps to Close)

The following controls are required and must be implemented per the timeline in Section 3.3:

#### R-01: Object Store Immutability (WORM)

**Requirement:** The MinIO / S3 artifact bucket must be configured with object-level retention locks so that stored artifact objects cannot be overwritten or deleted for a minimum retention period, even by the application service account.

- **AWS deployments:** Enable S3 Object Lock in `GOVERNANCE` mode on the artifact bucket. Set a default retention of 35 days (matching the artifact deprecation-to-deletion window plus a 5-day buffer). The IAM role used by ECS must not have `s3:BypassGovernanceRetention` or `s3:DeleteObjectVersion` on the artifact bucket.
- **On-premises deployments:** Enable MinIO Object Lock on the artifact bucket at bucket creation time (`mc mb --with-lock`). Configure a default retention rule of 35 days in `GOVERNANCE` mode.
- **Acceptance gate:** Verify that `PUT` of the same object key does not silently overwrite an existing locked version; verify that `DELETE` of a locked object returns an error before the retention period expires.

#### R-02: Rate Limiting on Artifact Write and Delete Operations

**Requirement:** The artifact upload and artifact delete API endpoints must be covered by the existing rate-limiting middleware.

- Introduce `ARTIFACT_UPLOAD_RPM` and `ARTIFACT_DELETE_RPM` configuration variables (default: 60 and 20 respectively).
- Apply the rate limiter in `httpapi/router.go` to the `POST /artifacts` upload route and the `DELETE /artifacts/{id}` delete route.
- Return HTTP 429 with `Retry-After` header on breach, consistent with other rate-limited endpoints.
- **Acceptance gate:** Confirm that sending 100 delete requests in under 60 seconds from a single IP returns 429 before the limit is exhausted.

#### R-03: Bulk Deletion Anomaly Detection

**Requirement:** The audit event path must detect and alert when an actor deletes or deprecates artifacts at an anomalous rate.

- Define a threshold: more than 10 artifact deletions or deprecations by a single actor within any 5-minute window triggers an alert.
- Alert must be emitted as a structured log event (level `ERROR`) and as a runtime event to `POST /api/v1/events` (type `security.anomaly.bulk_artifact_deletion`).
- On threshold breach, the actor's current session token must be flagged for review (logged); automatic suspension is optional but recommended for hardened deployments.
- **Acceptance gate:** Unit test confirming that 11 deletions in a 5-minute window emits the alert event; 9 do not.

#### R-04: Secondary Approval for Force Delete

**Requirement:** The `force=true` query parameter on `DELETE /artifacts/{id}` (which bypasses the deprecation requirement) must require `admin` role rather than `operator` role, and must produce an audit event with `action: artifact.force_delete` distinct from standard `artifact.delete`.

- Update the RBAC guard on the force-delete path from `operator` to `admin`.
- Log the force flag explicitly in the audit event `metadata` field.
- **Acceptance gate:** Confirm that an `operator`-role token receives HTTP 403 when `force=true` is set.

#### R-05: Isolated Backup Credentials

**Requirement:** Backup destination credentials must be separate from the credentials used to read and write the primary artifact store.

- **AWS deployments:** The backup IAM role or S3 bucket policy for the backup destination must not permit the ECS task role to read or delete backup objects — only write (`s3:PutObject`). A separate administrator role with no ECS attachment is used for restore operations.
- **On-premises deployments:** The `BACKUP_RUNNER_TOKEN` and backup destination URL must point to an endpoint that is not reachable by the MinIO primary credential. The backup system must reject connections from the primary MinIO service account.
- **Acceptance gate:** Confirm that the ECS task role cannot call `s3:GetObject` or `s3:DeleteObject` on the backup bucket.

### 3.3 Implementation Status

| Control | Priority | Target Date | Status |
|---------|----------|-------------|--------|
| R-04: Force-delete elevated to admin | High | 2026-04-27 | ✅ Implemented 2026-06-12 — force-aware RBAC guard in `httpapi/router.go` (operator → 403 on `force=true`); distinct `artifact.force_delete` audit action |
| R-02: Rate limiting on upload/delete | High | 2026-04-27 | ✅ Implemented 2026-06-12 — `ARTIFACT_UPLOAD_RPM` (60) / `ARTIFACT_DELETE_RPM` (20); applied to all ingest routes (`/artifacts`, `/upload`, `/pull`, `/complete`) and to delete + deprecate; 429 with `Retry-After` |
| R-05: Isolated backup credentials | High | 2026-05-11 | ✅ Implemented 2026-06-12 (AWS) — `modules/backup_store`: write-only bucket policy for the ECS task role (PutObject allowed; Get/Delete/policy actions denied), separate encryption key, ≥30-day retention validation. On-prem remains operational guidance (§3.1 remote backup runner) |
| R-01: Object store WORM locks | High | 2026-05-11 | ✅ Implemented 2026-06-12 — AWS: `enable_object_lock` on `modules/artifact_store` (GOVERNANCE, 35-day default retention, bucket policy denies `s3:BypassGovernanceRetention`/`s3:DeleteObjectVersion` except break-glass ARNs). On-prem/MinIO: `S3_OBJECT_LOCK=1` + `S3_OBJECT_LOCK_RETENTION_DAYS` — control-plane creates the bucket with locking and applies the default GOVERNANCE retention; fails closed if pointed at a pre-existing unlocked bucket |
| R-03: Bulk deletion anomaly detection | Medium | 2026-06-08 | ✅ Implemented 2026-06-12 — `handlers.BulkDeletionDetector`: >10 deletions/deprecations per actor per 5-minute window emits ERROR log + `security.anomaly.bulk_artifact_deletion` runtime event (streamed + persisted); re-alerts at most once per window; 7 unit tests |

**Deployment notes:** R-01/R-05 Terraform controls are opt-in/default-on respectively (`artifact_store_enable_object_lock`, `enable_backup_store` in `customer_stack`) and take effect on the next `terraform apply`; enabling Object Lock on an existing artifact bucket forces bucket replacement — migrate objects first. R-02/R-03/R-04 are active in the control-plane binary as of this date with no configuration required.

---

## 4. Detection

### 4.1 Indicators of a Ransomware Event

| Indicator | Detection Source |
|-----------|----------------|
| Bulk artifact deletions or deprecations (>10 in 5 minutes from one actor) | Anomaly detection (R-03); audit log |
| Artifact objects returning HTTP 404 that were previously accessible | Agent checkin failure spike; CloudWatch 4xx metrics |
| Database tables emptied or dropped | RDS CloudWatch storage drop; application startup failure |
| Backup runner `lastSuccessAt` stale by >26 hours | Backup status API; CloudWatch alarm |
| Service token used from unexpected IP or at unusual hours | Audit log actor + source IP fields |
| Agent fleet reporting mass plan execution failures | Runtime event stream; CloudWatch error rate alarm |
| Unexpected S3 `DeleteObject` volume in CloudWatch S3 metrics | CloudWatch S3 request metrics |

### 4.2 Monitoring Requirements

The following CloudWatch alarms must be configured for AWS deployments:

| Alarm | Metric / Source | Threshold | Action |
|-------|----------------|-----------|--------|
| Artifact delete spike | Custom metric from anomaly detection (R-03) | 1 occurrence | SNS → Engineering Lead |
| S3 DeleteObject volume | `AWS/S3 NumberOfObjects` (delta) | >50 objects deleted in 1 hour | SNS → Engineering Lead |
| RDS storage drop | `AWS/RDS FreeStorageSpace` | Drop >20% in 1 hour | SNS → Engineering Lead |
| Backup stale | Custom metric from backup runner | `lastSuccessAt` age >26 hours | SNS → Engineering Lead |
| Control-plane error rate spike | ALB `HTTPCode_Target_5XX_Count` | >50 errors/min | SNS → on-call |

---

## 5. Response Procedures

Ransomware events are declared P0 incidents and handled per `docs/compliance/policies/incident-response-policy.md`. The tactical ransomware-specific playbook is in `docs/incidents/ir-runbook.md` Section 9.

### 5.1 Severity Determination

| Observation | Severity |
|-------------|----------|
| Confirmed artifact objects destroyed or encrypted; devices cannot apply updates | P0 |
| Bulk deletion in progress; audit log shows active exfiltration | P0 |
| Anomalous deletion rate detected but artifacts not yet confirmed lost | P1 |
| Single compromised token used to deprecate artifacts; no confirmed deletion | P1 |

### 5.2 Containment Actions (first 15 minutes)

1. Enable maintenance mode (`MAINTENANCE_MODE=1`) to halt inbound agent traffic and stop artifact delivery.
2. Revoke the compromised service token or user account immediately.
3. Snapshot the RDS instance before any recovery action to preserve forensic state.
4. Export audit log for the past 24 hours to a secure, offline location (`GET /api/v1/audit`).
5. Do not delete any objects from MinIO/S3 until scope is determined — object metadata and access logs may be needed for forensics.

### 5.3 Recovery Path

| Data Store | Recovery Mechanism | Expected RTO |
|------------|-------------------|-------------|
| S3 artifact objects (with Object Lock, R-01) | Artifacts within retention window are not deleted; restore is not needed if lock held | < 1 hour to confirm integrity |
| S3 artifact objects (without Object Lock) | Restore from isolated backup (R-05); re-ingest from source CI if backup unavailable | 2–4 hours |
| RDS PostgreSQL | RDS PITR to a point before the event; see `docs/compliance/policies/backup-and-recovery-policy.md` §4 | < 2 hours |
| On-premises MinIO | Restore from `mc mirror` backup; verify object count and hashes | Per operator procedure; target < 4 hours |

After recovery:
- Verify artifact count and SHA256 checksums match pre-event state.
- Run `./scripts/artifact-e2e.sh` against the recovered environment.
- Confirm agent checkin and plan execution resume normally for a representative sample of devices.
- Monitor for recurrence for 48 hours before declaring recovery complete.

---

## 6. Backup Integrity Requirements

Ransomware-resistant backups must satisfy all of the following:

1. **Air-gap or write-only access** — the primary application credential cannot read or delete backup objects (see R-05).
2. **Tested restorability** — backup restore drills are conducted per `docs/compliance/policies/backup-and-recovery-policy.md` §4; a backup that has never been tested is not a recovery asset.
3. **Separate encryption keys** — backup encryption keys are stored separately from application encryption keys and are not accessible from the ECS task role.
4. **Retention overlap** — backup retention period (≥ 30 days) must overlap with the artifact soft-delete deprecation window (30 days) to ensure that any ransomware event detectable within the deprecation window has a corresponding backup restore point.

---

## 7. Device Fleet Considerations

Because Parcel agents are designed to operate autonomously in offline or bandwidth-constrained environments, the fleet is partially resilient to control-plane ransomware by design:

- Agents continue running their last successfully applied artifact state when the control plane is unreachable.
- The agent's local state file (`state.json`) and currently active artifact symlink are on-device and not directly reachable from the control plane.

However:
- If a ransomware actor pushes a malicious artifact to desired state before the event is detected, agents will attempt to apply it on next checkin.
- **Mitigation:** Enable `MAINTENANCE_MODE=1` as the first containment step to halt desired state pushes to the fleet; this prevents in-flight ransomware artifact delivery.
- **Mitigation:** Artifact signing (`require_verified`) ensures a ransomware payload must be signed with a trusted key to be applied — an attacker who has only compromised the control-plane API but not the signing key cannot deliver an executable ransomware artifact to devices.

---

## 8. Compliance Mapping

| Requirement | Framework | This Policy Section |
|-------------|-----------|-------------------|
| Data backup and recovery | NIST SP 800-171 §3.8.9 | §5.3, §6 |
| Audit log protection | NIST SP 800-171 §3.3.1 | §3.1 (audit log), §5.2 |
| Malicious code protection | NIST SP 800-171 §3.14.2 | §3.1 (signing), §7 |
| Incident response capability | SOC 2 CC7.3 | §5 |
| Logical access controls | SOC 2 CC6.1 | §3.2 R-04 |
| Availability controls | SOC 2 A1.2 | §3.2 R-01, R-05 |

---

## 9. Related Documents

- `docs/compliance/policies/backup-and-recovery-policy.md` — backup schedules, RTO/RPO targets, restore drills
- `docs/compliance/policies/incident-response-policy.md` — IR lifecycle and breach notification
- `docs/incidents/ir-runbook.md` §9 — ransomware response playbook
- `docs/compliance/risk-register.md` — RSK-013, RSK-014, RSK-015 (ransomware risk entries)
- `docs/guides/deployment-hardening.md` — hardened profile requirements
- `control-plane/internal/config/hardening.go` — hardened profile enforcement
- `control-plane/internal/objectstore/minio.go` — object store implementation (R-01 target)
- `control-plane/internal/httpapi/router.go` — rate limit wiring (R-02 target)
