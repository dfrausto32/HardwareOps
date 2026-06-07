# Backup and Recovery Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All production Parcel platform environments.

---

## 1. Purpose

This policy defines backup schedules, retention periods, recovery objectives, and restore-drill requirements to ensure that the Parcel platform can recover from data loss, corruption, or infrastructure failure within defined time targets.

---

## 2. Recovery Objectives

| Metric | Target |
|--------|--------|
| **RTO** (Recovery Time Objective) — time to restore service | ≤ 4 hours for P0 data loss events |
| **RPO** (Recovery Point Objective) — maximum acceptable data loss | ≤ 24 hours (daily backup baseline); ≤ 5 minutes for RDS point-in-time recovery |

---

## 3. Backup Inventory

### 3.1 AWS Production Deployment

| Data Store | Backup Mechanism | Frequency | Retention | Encryption |
|------------|-----------------|-----------|-----------|-----------|
| RDS PostgreSQL | AWS automated backups (continuous WAL + daily snapshot) | Daily snapshot; continuous WAL for PITR | 14 days | AES-256 (same key as primary) |
| RDS PostgreSQL | Manual snapshot before any significant migration or upgrade | Before each deployment with schema changes | Indefinite (until manually deleted) | AES-256 |
| S3 artifact store | S3 versioning (optional, per deployment) | Object-level version on every PUT | Per bucket versioning policy | AWS-KMS CMK |
| Secrets Manager secrets | AWS managed; secrets are configuration, not primary data | N/A — secrets are re-provisioned from source | N/A | AWS-managed |

**AWS RDS Point-in-Time Recovery:** The 14-day automated backup window allows restoration to any second within the retention period. This provides an effective RPO of < 5 minutes for the database layer.

### 3.2 On-Premises Deployment

| Data Store | Backup Mechanism | Frequency | Retention | Operator Action Required |
|------------|-----------------|-----------|-----------|--------------------------|
| PostgreSQL | `backup/runner.go` — `pg_dump` or container snapshot | Daily (recommended; configurable via `BACKUP_CMD`) | Operator-defined (recommend ≥ 30 days) | Configure `BACKUP_CMD`, `BACKUP_DIR`, `BACKUP_RUNNER_*` |
| MinIO (artifact store) | `mc mirror` or container snapshot via `backup/runner.go` | Daily (recommended) | Operator-defined (recommend ≥ 30 days) | Configure alongside PostgreSQL backup |
| `.env.onprem` / config | Stored in operator-controlled secrets management; not in backup scope | N/A | N/A | Operator responsibility |

The backup runner exposes a status API; backup success/failure is visible in logs and the platform UI.

---

## 4. Backup Verification and Restore Drills

Untested backups provide a false sense of security. The following restore drills are mandatory:

| Drill Type | Frequency | Owner | Documentation |
|-----------|-----------|-------|--------------|
| AWS RDS PITR restore to isolated environment | Quarterly | Engineering Lead | `docs/runbooks/rds-restore-drill.md` |
| On-prem PostgreSQL restore from `pg_dump` | Quarterly (for each active on-prem deployment) | Customer operator (guided by Parcel runbook) | `docs/runbooks/onprem-restore-drill.md` |
| Full environment restore (RDS + S3) | Annual | Engineering Lead | Documented in `docs/incidents/restore-drill-YYYY-MM.md` |

### 4.1 Drill Procedure (AWS RDS)

1. Identify the restore point (PITR timestamp or snapshot ARN).
2. Restore to a new RDS instance in the same VPC but not connected to production ECS services:
   ```bash
   aws rds restore-db-instance-to-point-in-time \
     --source-db-instance-identifier <prod-instance-id> \
     --target-db-instance-identifier <drill-instance-id> \
     --restore-time <ISO-8601-timestamp>
   ```
3. Point a test control-plane instance at the restored database.
4. Verify: device count, artifact count, audit event count match pre-restore expectations.
5. Confirm `AUTO_MIGRATE=1` runs cleanly against the restored database.
6. Destroy the drill instance.
7. Document results in `docs/incidents/restore-drill-YYYY-MM.md`.

### 4.2 Drill Pass Criteria

A drill passes when:
- The restored database starts and accepts connections.
- The control-plane starts against the restored database without errors.
- A representative sample of devices, artifacts, and audit events are present and correct.
- Any schema migrations that ran since the backup point apply cleanly.

A drill fails when data is missing, the control-plane fails to start, or migration errors occur. A failed drill must be treated as a P1 incident and root-caused within 48 hours.

---

## 5. Backup Monitoring

| Check | Method | Alert |
|-------|--------|-------|
| AWS RDS automated backup completed | CloudWatch `RecentlyRestoredFreeStorageSpace` metric; RDS Events | Alert Engineering Lead if no backup event in 26 hours |
| On-prem backup runner status | `GET /api/v1/backup/status` endpoint | Alert operator if `status != "success"` or `lastSuccessAt` > 26 hours ago |

---

## 6. Data Retention

| Data Type | Retention Period | Deletion Mechanism |
|-----------|-----------------|-------------------|
| Audit events | Configurable (default 90 days) via `AUDIT_RETENTION_DAYS`; automated cleanup job | `DeleteAuditEventsBefore` store method |
| Runtime events | Configurable (default 30 days) via `EVENT_RETENTION_DAYS` | `DeleteRuntimeEventsBefore` store method |
| Artifacts | Per lifecycle policy (`DeprecatedDeleteAfterDays`); pruned by automated job | Artifact prune job + MinIO/S3 object delete |
| RDS automated backups | 14 days (AWS managed) | AWS automatic expiry |
| Restore drill instances | Destroyed immediately after drill completes | Manual: `aws rds delete-db-instance` |

Data retention periods balance operational investigation needs against storage cost and privacy obligations. Changes to default retention periods must be approved by the Engineering Lead and documented.

---

## 7. Disaster Recovery Scenarios

| Scenario | Recovery Path | Expected RTO |
|----------|--------------|-------------|
| RDS instance failure (hardware/AZ) | AWS Multi-AZ automatic failover | < 2 minutes (automatic) |
| Accidental data deletion (rows/tables) | RDS PITR to point before deletion | < 2 hours |
| Corrupted artifact in S3 | Re-ingest from source or restore from S3 version | < 1 hour |
| Full production environment loss | Terraform re-apply + RDS restore from snapshot | ≤ 4 hours |
| On-prem deployment failure | Restore from latest `pg_dump` + re-run `AUTO_MIGRATE=1` | Per operator procedure; target ≤ 4 hours |

---

## 8. Related Documents

- `backup/runner.go` — backup runner implementation
- `docs/guides/operations.md` — day-2 operational runbook including backup commands
- `docs/compliance/policies/incident-response-policy.md` — disaster recovery under incident conditions
- `docs/guides/deployment-hardening.md` §7 — encryption of backup data at rest
