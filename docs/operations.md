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
- **API:** `POST /api/v1/cert-rotation/rotate` with JSON body `{"reason":"<why this break-glass rotation is required>"}`
- **Script:** `scripts/rotate--ca.sh`

All break-glass certificate endpoints require operator or admin auth plus a JSON `reason`:
- `POST /api/v1/cert-rotation/reload`
- `POST /api/v1/cert-rotation/rotate`
- `POST /api/v1/cert-rotation/cleanup`

### 3.2 Verify devices moved to active CA

Check device metadata:
- `metadata.hwops.cert.caFingerprint`
- `metadata.hwops.cert.active=true`

No license-slot increase should occur during re-enroll.

### 3.3 Cleanup old CA

After rotation is complete:
- run cleanup in UI/API
- or follow `certs.md` manual cleanup

### 3.4 Device decommission (license slot reclaim)

Use decommission instead of raw delete so slot reclaim has an explicit audit trail.

```bash
curl --cacert /opt/hardwareops/certs/ca.crt \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  https://hardwareops.internal/api/v1/devices/<device-id>/decommission \
  -d '{"reason":"device retired","ticketId":"OPS-123"}'
```

Expect:
- response with `slotBefore` and `slotAfter`
- audit action `device.decommission`

---

## 4) Pull credential rotation/reload

Use this when rotating repository credentials used by pull ingest (`source.credentialRef`).

### 4.1 Rotate at source

- Static file/json mode: update `ARTIFACT_PULL_CREDENTIALS_FILE` content (or `ARTIFACT_PULL_CREDENTIALS_JSON` value).
- AWS mode: update the Secrets Manager secret value referenced by `ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`.
- Vault mode: update the KV v2 secret at `ARTIFACT_PULL_CREDENTIALS_VAULT_PATH`, then trigger a reload (step 4.2). To rotate the Vault token itself, update `ARTIFACT_PULL_CREDENTIALS_VAULT_TOKEN` in `.env.onprem` and restart the control-plane container. Rotate Vault tokens before their TTL expires — there is no automatic renewal.

### 4.2 Reload in control-plane (no restart)

```bash
curl --cacert /opt/hardwareops/certs/ca.crt \
  -H "Authorization: Bearer <admin-jwt>" \
  -X POST https://hardwareops.internal/api/v1/artifacts/pull-credentials/reload
```

Check status:

```bash
curl --cacert /opt/hardwareops/certs/ca.crt \
  -H "Authorization: Bearer <admin-jwt>" \
  https://hardwareops.internal/api/v1/artifacts/pull-credentials
```

### 4.3 Helper script

```bash
AUTH_EMAIL=admin@example.com AUTH_PASSWORD='change-me' \
BASE_URL=https://localhost:8080 \
./scripts/reload-pull-credentials.sh
```

Audit action emitted:
- `artifact.pull_credentials.reload`

---

## 5) Security checks (periodic)

- Confirm auth is enabled and bootstrap secrets rotated.
- Confirm `HARDENED_PROFILE=1` in production on-prem.
- Confirm `TRUST_PROXY_CIDRS` is restricted (not `0.0.0.0/0`).
- Confirm audit retention policy is set as intended.
- Confirm backups are restorable by doing restore drills.

---

## 6) Metrics and health checks

Use:
- `/metrics` (Prometheus)
- `/api/v1/health/summary`

Focus on:
- device activity
- check-in errors
- DB pool pressure (`hwops_db_open_conns`, `hwops_db_in_use`, `hwops_db_wait_count`)
- storage growth (`hwops_s3_objects_total`, `hwops_s3_bytes_total`)
- pending-enrollment queue pressure (`hwops_pending_enroll_active_total`, `hwops_pending_enroll_queue_age_total`, `hwops_pending_enroll_oldest_age_seconds`)
- pending-enrollment abuse/throttle reasons (`hwops_pending_enroll_throttle_total{reason}`)

For metric definitions and dashboard details:
- `development/metrics-health.md`

---

## Deep-dive references

- Backup/restore runbook: `backup-restore.md`
- CA and TLS rotation details: `certs.md`
- Upgrade design and contract: `development/upgrade-strategy.md`
- Deployment hardening controls: `deployment-hardening.md`
- Pull credential backends (S3, GCS, Vault): `cloud-pull-adapters.md`
- LDAP/AD authentication: `ldap-auth.md`
- Keyless signing and attestations: `artifact-provenance.md`
