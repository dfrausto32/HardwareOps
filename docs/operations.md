# Operations Guide (Canonical)

This is the primary day-2 operations guide. Use it for backup/restore, upgrades, certificate rotation, and validation checks.

---

## 1) Backups and restore

### 1.1 Put system in maintenance mode

```bash
curl --cacert /opt/hardwareops/certs/ca.crt -X POST \
  https://hardwareops.internal/api/v1/maintenance \
  -H "Content-Type: application/json" \
  -d '{"enabled":true,"message":"Maintenance operation in progress"}'
```

### 1.2 Create backup (UI preferred)

- Settings -> Backups -> Create backup

or use the runbook in `backup-restore.md`.

### 1.3 Restore and wipe

- Settings -> Backups -> Restore + wipe

Always validate after restore:
- `/healthz` is healthy
- devices + artifacts load in UI
- agent check-ins resume

---

## 2) Upgrades

### 2.1 Build upgrade package (build machine)

```bash
./scripts/build-upgrade-package.sh
```

### 2.2 Stage bundle on target stack host

Place the generated tarball under:

```bash
/opt/hardwareops/stack/updates/
```

### 2.3 Run preflight and apply

- Settings -> Maintenance -> Upgrade
- Run preflight
- Apply upgrade

If apply fails:
- inspect `upgrade-*.log` under `/var/lib/hardwareops/logs` in control-plane container

For strategy and rollback behavior, see:
- `development/upgrade-strategy.md`

---

## 3) Certificate rotation

### 3.1 Rotation methods

- **UI:** Security page -> Certificate Rotation
- **API:** `POST /api/v1/cert-rotation/rotate`
- **Script:** `scripts/rotate--ca.sh`

### 3.2 Verify devices moved to active CA

Check device metadata:
- `metadata.hwops.cert.caFingerprint`
- `metadata.hwops.cert.active=true`

No license-slot increase should occur during re-enroll.

### 3.3 Cleanup old CA

After rotation is complete:
- run cleanup in UI/API
- or follow `certs.md` manual cleanup

---

## 4) Security checks (periodic)

- Confirm auth is enabled and bootstrap secrets rotated.
- Confirm `TRUST_PROXY_CIDRS` is restricted (not `0.0.0.0/0`).
- Confirm audit retention policy is set as intended.
- Confirm backups are restorable by doing restore drills.

---

## 5) Metrics and health checks

Use:
- `/metrics` (Prometheus)
- `/api/v1/health/summary`

Focus on:
- device activity
- check-in errors
- DB pool pressure (`hwops_db_open_conns`, `hwops_db_in_use`, `hwops_db_wait_count`)
- storage growth (`hwops_s3_objects_total`, `hwops_s3_bytes_total`)

For metric definitions and dashboard details:
- `development/metrics-health.md`

---

## Deep-dive references

- Backup/restore runbook: `backup-restore.md`
- CA and TLS rotation details: `certs.md`
- Upgrade design and contract: `development/upgrade-strategy.md`
- Deployment hardening controls: `deployment-hardening.md`
