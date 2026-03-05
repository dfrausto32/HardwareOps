# Metrics + Health (Phase B)

This doc defines the **metrics surface**, **health summary API**, and **UI panel** for fleet operations.

## Goals
- Provide operators with **early warning** signals (latency, error rate, device health).
- Support **capacity planning** and **incident response**.
- Enable simple **dashboards** with Prometheus/Grafana.

## Metrics endpoint
- **Path:** `/metrics`
- **Auth:** same as API (requires admin role if auth enabled).
- **Enabled by default:** `METRICS_ENABLED=1`

### Environment settings
```
METRICS_ENABLED=1
METRICS_PATH=/metrics
METRICS_REFRESH_INTERVAL=30s
```

## Exported metrics (v1)
### HTTP
- `hwops_http_requests_total{method,route,status}`
- `hwops_http_request_duration_seconds{method,route}`

### Fleet counts
- `hwops_devices_total`
- `hwops_devices_status_total{status}`  
  Status values: `active`, `stale`, `offline`, `degraded`

### Database pool pressure
- `hwops_db_open_conns`
- `hwops_db_in_use`
- `hwops_db_wait_count` (cumulative wait count)

### Storage usage (artifact metadata)
- `hwops_s3_objects_total`
- `hwops_s3_bytes_total`

### Check‑ins
- `hwops_checkin_total{status,reason}`  
  `status`: `success` or `error`  
  `reason`: `ok`, `unauthorized`, `bad_request`, `storage_error`, `unknown`

### Enrollments
- `hwops_enrollment_token_total{status,reason}`  
  `status`: `success` or `error`  
  `reason`: `ok`, `bad_request`, `expires_too_large`, `expires_negative`, `token_error`, `storage_error`, `license_limit`, `license_invalid`, `license_error`
- `hwops_enroll_total{status,reason}`  
  `status`: `success` or `error`  
  `reason`: `ok`, `bad_request`, `csr_invalid`, `csr_too_large`, `token_invalid`, `storage_error`, `sign_error`, `signer_missing`, `license_limit`, `license_invalid`, `license_error`

### Pending enrollment queue pressure + abuse telemetry
- `hwops_pending_enroll_active_total`  
  Current active pending-enrollment queue depth (pending + unexpired).
- `hwops_pending_enroll_queue_age_total{bucket}`  
  Active queue depth split by age buckets:
  `lt_1m`, `1m_5m`, `5m_15m`, `15m_1h`, `gte_1h`.
- `hwops_pending_enroll_oldest_age_seconds`  
  Age (seconds) of the oldest active pending enrollment.
- `hwops_pending_enroll_throttle_total{reason}`  
  Throttle/guardrail rejections by reason.
  Common reasons: `pending_enroll_source`, `pending_enroll_profile`, `queue_full_global`, `queue_full_profile`, `queue_full_source`, `approval_delay`.

### Apply pipeline
- `hwops_apply_total{status,component}`  
  `status`: `success` or `error`  
  `component`: `agent_bundle`, `customer`, etc.
- `hwops_preapply_total{status,component}`  
  `status`: `success`, `error`, or `skipped`

### Artifact delivery
- `hwops_artifact_upload_total{status}`  
  `status`: `success` or `error`
- `hwops_artifact_presign_total{status}`  
  `status`: `success` or `error`

### Rate limiting
- `hwops_rate_limit_total{endpoint}`  
  `endpoint`: `enrollment_token`, `device_enroll`, `checkin`, `apply_result`

### Operations
- `hwops_upgrade_total{status}`  
  `status`: `started` or `error`
- `hwops_backup_total{operation,status}`  
  `operation`: `backup` or `restore`  
  `status`: `started` or `error`

### Pending actions
- `hwops_pending_actions_total{type}`  
  `type`: `device.reenroll`

> Note: route labels use the **route pattern** (e.g., `/api/v1/devices/{deviceId}`) to avoid cardinality explosion.

## Pending enrollment alert thresholds (recommended)
- **Queue depth pressure**: alert when `hwops_pending_enroll_active_total` is sustained above 80% of `PENDING_ENROLL_MAX_ACTIVE` for 10 minutes.
- **Approval backlog age**: alert when `hwops_pending_enroll_oldest_age_seconds > 300` for 10 minutes.
- **Abuse/throttle spikes**: alert on rapid increase in `hwops_pending_enroll_throttle_total{reason=~"pending_enroll_.*|queue_full_.*|approval_delay"}` over 5-minute windows.
- **Critical backlog**: page when any requests reach `bucket="gte_1h"` for sustained periods.

## Health summary API (UI)
**Endpoint:** `GET /api/v1/health/summary`

Response:
```
{
  "generatedAt": "2026-02-12T14:12:01Z",
  "devices": {
    "total": 10,
    "active": 7,
    "stale": 2,
    "offline": 1,
    "degraded": 0,
    "lastSeen": "2026-02-12T14:11:58Z"
  }
}
```

## UI surface
**Dashboard → Health card**
- Total, Active, Degraded, Stale, Offline
- Last Check‑in timestamp

This panel reads from `/api/v1/health/summary` and updates periodically.

## Local metrics stack (Prometheus + Grafana)
Use the bundled compose file to launch Prometheus + Grafana with a preloaded dashboard.

From repo root:
```
cd deploy/compose
docker compose -f docker-compose.metrics.yml up -d
```

Open:
- **Prometheus:** `http://localhost:9090`
- **Grafana:** `http://localhost:3000` (admin/admin)

### Auth notes
If `AUTH_ENABLED=1`, `/metrics` requires an admin JWT. Update `deploy/compose/metrics/prometheus.yml`
and add a bearer token:
```
authorization:
  type: Bearer
  credentials: REPLACE_ME
```
For local dev you can also set `AUTH_ENABLED=0` to keep metrics open.
