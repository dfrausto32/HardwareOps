# Parcel Integration Control Document (ICD)

Status: Active (living document)  
API namespace: `v1`  
Last updated: 2026-03-24
Source of truth for routes: `control-plane/internal/httpapi/router.go`

This is the canonical contract for integrating with Parcel without relying on the GUI.

## 1) Purpose and Scope

This ICD defines:
- HTTP/API interface contracts
- auth and authorization requirements
- high-value integration workflows (enrollment, ingest, rollout, operations)
- compatibility expectations for non-GUI clients and automation systems

This file is intentionally concise and implementation-aligned. Deep dives stay in topic docs and are linked at the end.

## 2) Living-Doc Rules

When API behavior changes, this file must be updated in the same change set.

Minimum required updates:
- endpoint add/remove/auth change
- request/response shape change (top-level fields)
- auth model/scope change
- new hardening guardrail that impacts integrations

Update checklist for maintainers:
1. Update route/auth tables in this ICD.
2. Update the workflow section if behavior changed.
3. Link the related deep-dive doc (if needed).
4. Call out compatibility impact in PR notes/release notes.

## 3) Protocol and Data Conventions

- Base API prefix: `/api/v1`
- Content type: JSON unless endpoint explicitly uses multipart or streaming
- Timestamp format: RFC3339
- IDs: UUID in most resources (`deviceId`, `artifactId`, `profileId`, etc.)
- Unknown JSON fields: clients should ignore them
- Error format: mixed (plain text via `http.Error` on many paths, JSON on structured workflows)

Non-API utility endpoints:
- `GET /healthz` (no auth)
- metrics path is configurable (commonly `/metrics`)

## 4) Auth and Authorization Model

Roles:
- `viewer`
- `operator`
- `admin`

Auth modes:
- `AUTH_MODE=disabled`: role gates are bypassed
- local/JWT auth: use `Authorization: Bearer <token>`
- service tokens: scoped machine tokens for automation
- workload identity exchange: external CI OIDC token exchanged for short-lived Parcel bearer token

Service token scopes — each scope grants access to a specific slice of the API without requiring a user role:

| Scope | Grants access to |
|---|---|
| `artifact.publish` | pull ingest, presign-upload, complete; read artifact/attestation GET routes |
| `artifact.read` | GET artifact list, detail, attestations, lifecycle status, vulnerability scan results |
| `device.read` | GET device list, detail, group list, deployment status, vulnerability scan results |
| `deployment.trigger` | `POST /devices/{deviceId}/trigger-apply`, `POST /groups/{groupId}/trigger-apply` |
| `webhook.manage` | full CRUD on webhooks and deliveries |

A service token may carry multiple scopes (comma-separated in the `scope` field at creation time). Scoped tokens are accepted on endpoints that also accept the corresponding role; a `device.read` token cannot write.

mTLS/device identity:
- device check-in/enroll paths rely on client cert identity when TLS/mTLS is enabled
- control-plane can consume cert identity from TLS peer cert or trusted proxy header

## 5) Endpoint Contract Catalog

All paths below are full paths.

### 5.1 Bootstrap + Auth

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/bootstrap` | public | bootstrap status |
| GET | `/api/v1/bootstrap/ca` | public + bootstrap-token logic | downloads bootstrap CA |
| GET | `/api/v1/auth/status` | public | auth mode and enabled state; includes `ldapEnabled: bool` when LDAP is configured; includes `smtpEnabled: bool` when SMTP is configured |
| POST | `/api/v1/auth/login` | public | rate limited; returns JWT on success, or HTTP 202 with `{"totpRequired":true,"pendingToken":"..."}` when TOTP is enabled for the account |
| POST | `/api/v1/auth/ldap/login` | public | rate limited; LDAP/AD credential exchange, returns JWT identical to local login |
| POST | `/api/v1/auth/register` | public | voucher/bootstrap-driven local registration |
| POST | `/api/v1/auth/forgot-password` | public | rate limited; accepts `{"email":"..."}`, always returns 200 regardless of whether address is registered (no user-existence leak); sends reset link email when `SMTP_HOST` is configured |
| POST | `/api/v1/auth/password-reset/complete` | public | rate limited; accepts `{"email":"...","token":"...","newPassword":"..."}` to redeem a reset token |
| POST | `/api/v1/auth/totp/verify` | public (rate limited) | second step of TOTP login; body: `{"pendingToken":"...","code":"123456"}`; exchanges a `totp_pending` JWT + valid TOTP code for a full session JWT |
| POST | `/api/v1/auth/totp/enroll` | viewer | generate a new TOTP key; returns `{"otpUri":"...","secret":"..."}` for QR display; key is stored encrypted but not yet active |
| POST | `/api/v1/auth/totp/confirm` | viewer | activate TOTP after verifying ownership; body: `{"code":"123456"}`; idempotent — re-runs enrollment if already enabled |
| POST | `/api/v1/auth/totp/disable` | viewer | disable TOTP; body: `{"password":"..."}` (current password required to prevent session-hijack downgrade) |
| GET | `/api/v1/auth/me` | viewer | caller identity; `totpEnabled` field reflects current TOTP status |
| GET | `/api/v1/auth/workload-identity/status` | admin | configured workload identity providers |
| POST | `/api/v1/auth/vouchers` | admin | create registration voucher |
| POST | `/api/v1/auth/service-tokens` | admin | create service token |
| GET | `/api/v1/auth/service-tokens` | admin | list service tokens |
| POST | `/api/v1/auth/workload-identity/exchange` | anonymous CI workload | exchange external OIDC token for short-lived publish token |
| POST | `/api/v1/auth/service-tokens/{tokenId}/revoke` | operator | break-glass revoke; JSON body requires `reason` |
| POST | `/api/v1/auth/service-tokens/{tokenId}/rotate` | operator | break-glass rotate; JSON body requires `reason`, optional `ttlHours` |
| POST | `/api/v1/users` | admin | create local user |
| GET | `/api/v1/users` | admin | list users |
| PATCH | `/api/v1/users/{userId}` | admin | update user |
| POST | `/api/v1/users/{userId}/password-reset-token` | admin | issue a one-time reset token for a user; body: `{"reason":"...","ttlMinutes":15,"sendEmail":true}`; when `sendEmail: true` and SMTP is configured the token is emailed directly |
| POST | `/api/v1/users/{userId}/invite` | admin | create a 72-hour invite token and email a setup link; requires SMTP to be configured |

### 5.2 Groups + Desired State

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/groups` | viewer or `device.read` scope | list group definitions |
| POST | `/api/v1/groups/batch` | operator | bulk group operations |
| PUT | `/api/v1/groups/{groupId}` | operator | upsert group |
| DELETE | `/api/v1/groups/{groupId}` | operator | delete group |
| GET | `/api/v1/desired-state` | viewer | list desired-state records |
| PUT | `/api/v1/desired-state/groups/{groupId}` | operator | set group desired state |
| DELETE | `/api/v1/desired-state/groups/{groupId}` | operator | clear group desired state |
| PUT | `/api/v1/desired-state/devices/{deviceId}` | operator | set device override |
| DELETE | `/api/v1/desired-state/devices/{deviceId}` | operator | clear device override |
| GET | `/api/v1/groups/{groupId}/deployment-status` | viewer or `device.read` scope | per-device rollout status for a given artifact; requires `?artifactId=<uuid>` query param |

### 5.3 Devices

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/devices` | viewer or `device.read` scope | list devices |
| GET | `/api/v1/devices/{deviceId}` | viewer or `device.read` scope | device detail |
| PATCH | `/api/v1/devices/{deviceId}` | operator | update metadata/grouping fields |
| DELETE | `/api/v1/devices/{deviceId}` | operator | delete device record |
| POST | `/api/v1/devices/{deviceId}/decommission` | admin | terminal decommission |
| POST | `/api/v1/devices/{deviceId}/apply-result` | device path | rate limited |
| POST | `/api/v1/devices/checkin` | device path | rate limited; response includes `immediateRecheckin: true` when a deploy trigger is pending |
| POST | `/api/v1/devices/reenroll` | device path | rate limited |
| GET | `/api/v1/devices/{deviceId}/vulnerability-scans` | viewer or `device.read` scope | list vulnerability scans for a device (Nessus-sourced) |
| GET | `/api/v1/devices/{deviceId}/vulnerability-scans/latest` | viewer or `device.read` scope | most recent vulnerability scan for a device |
| POST | `/api/v1/devices/{deviceId}/trigger-apply` | operator or `deployment.trigger` scope | signal a specific device to re-check in immediately |

### 5.4 Enrollment (legacy + first-contact)

| Method | Path | Access | Notes |
|---|---|---|---|
| POST | `/api/v1/enrollments` | operator | create legacy enrollment token (rate limited) |
| POST | `/api/v1/devices/enroll` | bootstrap token | legacy enroll endpoint (rate limited) |
| GET | `/api/v1/enrollment-profiles` | operator | list first-contact profiles |
| POST | `/api/v1/enrollment-profiles` | operator | create profile |
| PATCH | `/api/v1/enrollment-profiles/{profileId}` | operator | edit profile without rotating token |
| POST | `/api/v1/enrollment-profiles/{profileId}/rotate` | operator | rotate profile bootstrap token (optional body: `reason`, `gracePeriodSec` up to 86400) |
| POST | `/api/v1/enrollment-profiles/{profileId}/disable` | operator | disable profile |
| POST | `/api/v1/enrollment-profiles/{profileId}/enable` | operator | enable profile |
| POST | `/api/v1/pending-enrollments/request` | profile token | submit pending request (rate limited + guarded) |
| POST | `/api/v1/pending-enrollments/claim` | claim token | claim issued cert/device identity (rate limited) |
| GET | `/api/v1/pending-enrollments` | operator | list pending queue |
| POST | `/api/v1/pending-enrollments/{requestId}/approve` | operator | approve request |
| POST | `/api/v1/pending-enrollments/{requestId}/deny` | operator | deny request |
| POST | `/api/v1/pending-enrollments/{requestId}/reset` | operator | reset denied/conflict request to pending |

### 5.5 Artifacts + Ingest

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/artifacts` | viewer or `artifact.read` scope | list artifacts |
| GET | `/api/v1/artifacts/{artifactId}` | viewer or `artifact.read` scope | artifact detail |
| POST | `/api/v1/artifacts` | operator | register artifact metadata (rate limited: `ARTIFACT_UPLOAD_RPM`, default 60/min) |
| POST | `/api/v1/artifacts/upload` | operator | multipart upload + register (rate limited: `ARTIFACT_UPLOAD_RPM`) |
| POST | `/api/v1/artifacts/pull` | operator or `artifact.publish` scope | control-plane pull ingest (rate limited: `ARTIFACT_UPLOAD_RPM`) |
| POST | `/api/v1/artifacts/presign-upload` | operator or `artifact.publish` scope | presigned upload init |
| POST | `/api/v1/artifacts/complete` | operator or `artifact.publish` scope | finalize presigned upload (rate limited: `ARTIFACT_UPLOAD_RPM`) |
| POST | `/api/v1/artifacts/{artifactId}/attestations` | operator | submit in-toto / SLSA attestation; keyless bundles verified at upload |
| GET | `/api/v1/artifacts/{artifactId}/attestations` | viewer or `artifact.read` scope | list attestations for an artifact |
| POST | `/api/v1/artifacts/{artifactId}/deprecate` | operator | deprecate artifact (rate limited: `ARTIFACT_DELETE_RPM`, default 20/min) |
| POST | `/api/v1/artifacts/{artifactId}/restore` | operator | restore deprecated artifact |
| DELETE | `/api/v1/artifacts/{artifactId}` | operator; `force=true` requires **admin** | hard delete (guarded by refs/policy; rate limited: `ARTIFACT_DELETE_RPM`). `force=true` bypasses the deprecation window, requires admin (operator → 403), and is audited as `artifact.force_delete` |
| POST | `/api/v1/artifacts/{artifactId}/presign` | viewer or `artifact.read` scope | presign download |
| GET | `/api/v1/artifacts/lifecycle/policy` | viewer or `artifact.read` scope | lifecycle policy |
| PUT | `/api/v1/artifacts/lifecycle/policy` | admin | set lifecycle policy |
| GET | `/api/v1/artifacts/lifecycle/status` | viewer or `artifact.read` scope | lifecycle status summary |
| POST | `/api/v1/artifacts/lifecycle/prune` | admin | immediate prune run |
| GET | `/api/v1/artifacts/pull-credentials` | admin | credential resolver status; includes `vault_backed`, `vault_addr`, `vault_path`, `vaultCredentialRefs` when Vault is configured |
| POST | `/api/v1/artifacts/pull-credentials/reload` | admin | reload resolver configuration; re-fetches from all configured backends (file, AWS SM, Vault) |
| GET | `/api/v1/artifacts/{artifactId}/vulnerability-scans` | viewer or `artifact.read` scope | list vulnerability scans for an artifact |
| GET | `/api/v1/artifacts/{artifactId}/vulnerability-scans/latest` | viewer or `artifact.read` scope | most recent vulnerability scan for an artifact |
| POST | `/api/v1/artifacts/{artifactId}/vulnerability-scans` | operator | trigger manual rescan for an artifact |
| POST | `/api/v1/vulnerability-scans/nessus/sync` | admin | trigger immediate Nessus sync |
| GET | `/api/v1/vulnerability-scans/nessus/status` | admin | Nessus sync status (last sync time, device match count) |

### 5.6 Security, Audit, Events, Health

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/artifact-trust/policy` | viewer | global artifact trust policy; includes optional `provenance` object (Phase C) |
| PUT | `/api/v1/artifact-trust/policy` | admin | set global artifact trust policy; accepts optional `provenance` object |
| GET | `/api/v1/artifact-trust/keys` | viewer | list trusted signing keys |
| POST | `/api/v1/artifact-trust/keys` | admin | register trusted signing key |
| DELETE | `/api/v1/artifact-trust/keys/{keyId}` | admin | remove trusted signing key |
| GET | `/api/v1/license` | admin | license/device-cap state |
| GET | `/api/v1/release-auto-update` | viewer | release auto-update settings |
| PUT | `/api/v1/release-auto-update` | admin | update auto-update policy |
| POST | `/api/v1/release-auto-update/run` | admin | force auto-update evaluation |
| GET | `/api/v1/cert-rotation` | viewer | CA rotation status |
| POST | `/api/v1/cert-rotation/reload` | operator | break-glass reload; JSON body requires `reason` |
| POST | `/api/v1/cert-rotation/rotate` | operator | break-glass rotate; JSON body requires `reason` |
| POST | `/api/v1/cert-rotation/cleanup` | operator | break-glass cleanup; JSON body requires `reason` |
| GET | `/api/v1/audit` | admin | list audit events |
| GET | `/api/v1/audit.csv` | admin | CSV export |
| GET | `/api/v1/audit/retention` | admin | audit retention config |
| PUT | `/api/v1/audit/retention` | admin | set audit retention |
| GET | `/api/v1/events/history` | viewer | runtime event history; includes security anomaly events (`security.anomaly.bulk_artifact_deletion` — emitted when one actor deletes/deprecates >10 artifacts in 5 minutes) |
| GET | `/api/v1/events` | viewer | realtime event stream |
| GET | `/api/v1/events/retention` | admin | event retention config |
| PUT | `/api/v1/events/retention` | admin | set event retention |
| GET | `/api/v1/health/summary` | viewer | control-plane health summary |
| GET | `/api/v1/logs/{deviceId}` | viewer | device log export/read |

### 5.7 Maintenance: Backup, Restore, Upgrade

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/maintenance` | viewer | maintenance status |
| PUT | `/api/v1/maintenance` | admin | toggle/update maintenance mode |
| GET | `/api/v1/maintenance/backups` | admin | list backups |
| GET | `/api/v1/maintenance/backup` | admin | backup runner status |
| POST | `/api/v1/maintenance/backup` | admin | start backup |
| GET | `/api/v1/maintenance/restore` | admin | restore runner status |
| POST | `/api/v1/maintenance/restore` | admin | start restore |
| GET | `/api/v1/maintenance/upgrade/available` | viewer | discover available bundles |
| GET | `/api/v1/maintenance/upgrade/preflight` | viewer | preflight checks |
| GET | `/api/v1/maintenance/upgrade` | viewer | upgrade runner status |
| POST | `/api/v1/maintenance/upgrade` | admin | trigger upgrade |

### 5.8 Webhooks

Webhooks deliver real-time event payloads to an external HTTPS endpoint via POST. Each delivery is signed with HMAC-SHA256 (key = the webhook's `secret`; header: `X-Parcel-Signature`).

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/webhooks` | operator or `webhook.manage` scope | list webhooks |
| POST | `/api/v1/webhooks` | operator or `webhook.manage` scope | create webhook |
| GET | `/api/v1/webhooks/{webhookId}` | operator or `webhook.manage` scope | get webhook |
| PATCH | `/api/v1/webhooks/{webhookId}` | operator or `webhook.manage` scope | update webhook |
| DELETE | `/api/v1/webhooks/{webhookId}` | operator or `webhook.manage` scope | delete webhook |
| POST | `/api/v1/webhooks/{webhookId}/test` | operator or `webhook.manage` scope | send a test ping payload |
| GET | `/api/v1/webhooks/{webhookId}/deliveries` | operator or `webhook.manage` scope | list deliveries (most recent first) |
| GET | `/api/v1/webhooks/{webhookId}/deliveries/{deliveryId}` | operator or `webhook.manage` scope | delivery detail including request/response bodies |
| POST | `/api/v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver` | operator or `webhook.manage` scope | re-send original payload; creates a new delivery record |

**Webhook payload envelope:**

```json
{
  "id": "uuid",
  "eventType": "apply.result",
  "deviceId": "uuid",
  "firedAt": "2026-03-24T10:00:00Z",
  "data": { ... }
}
```

`deviceId` is present on device-scoped events (`apply.result`, `deploy.trigger`).

**Supported `eventType` values:** `apply.result`, `deploy.trigger`, `ping`

**Signature verification:**

```bash
# Python example
import hmac, hashlib
expected = hmac.new(secret.encode(), payload_bytes, hashlib.sha256).hexdigest()
assert request.headers["X-Parcel-Signature"] == expected
```

### 5.9 Deploy Triggers

Deploy triggers signal one or more devices to skip their normal check-in interval and re-check in immediately, consuming any pending desired-state change without waiting.

| Method | Path | Access | Notes |
|---|---|---|---|
| POST | `/api/v1/devices/{deviceId}/trigger-apply` | operator or `deployment.trigger` scope | signal one device to re-check in immediately |
| POST | `/api/v1/groups/{groupId}/trigger-apply` | operator or `deployment.trigger` scope | signal all devices matching the group's label selector |

**Request body:** empty or `{}`

**Response (device trigger):**

```json
{ "deviceId": "uuid", "triggered": true }
```

**Response (group trigger):**

```json
{ "groupId": "uuid", "deviceCount": 5, "triggered": true }
```

When a trigger is pending, the device's next `POST /devices/checkin` response includes `"immediateRecheckin": true`, causing the agent to skip its sleep interval and re-check in within seconds.

### 5.10 Metrics

- Metrics route is configurable (`MetricsPath` in control-plane config; commonly `/metrics`)
- Auth role when enabled: admin

## 6) Headless Workflow Contracts

### 6.1 Login and JWT usage

Request:
```json
POST /api/v1/auth/login
{
  "email": "admin@example.com",
  "password": "change-me"
}
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expiresAt": "2026-03-05T10:00:00Z",
  "user": {
    "userId": "uuid",
    "email": "admin@example.com",
    "roles": ["admin"]
  }
}
```

### 6.2 First-contact enrollment (recommended)

1. Operator creates profile (`POST /api/v1/enrollment-profiles`).
2. Agent requests enrollment (`POST /api/v1/pending-enrollments/request`) with profile token + CSR.
3. Operator approves/denies (`POST /api/v1/pending-enrollments/{id}/approve|deny`).
4. Agent polls claim (`POST /api/v1/pending-enrollments/claim`).
5. On `status=issued`, agent persists `deviceId`, `certPem`, `caCertPem`, then moves to check-in loop.

Token reissue policy:
- Profile token rotation supports an optional grace window (`gracePeriodSec`) where the immediately previous token remains valid.
- Use short grace windows for staged rollout (for example 300 seconds), then rely on the new token only.
- Rotation audit records include the operator reason and grace window.

Claim statuses:
- `pending`: keep polling after `pollAfterSec`
- `denied`: terminal until reset/re-request
- `issued`: enrollment complete

### 6.3 Artifact publish

Supported integration patterns:
- push multipart: `POST /api/v1/artifacts/upload`
- push presigned: `POST /api/v1/artifacts/presign-upload` -> object upload -> `POST /api/v1/artifacts/complete`
- pull ingest: `POST /api/v1/artifacts/pull` (control-plane downloads from source URI)

### 6.4 Fleet rollout

1. Define/update groups (`/api/v1/groups...`).
2. Set desired state (`/api/v1/desired-state/...`).
3. Agents consume desired state through `/api/v1/devices/checkin`.
4. Agents report execution via `/api/v1/devices/{deviceId}/apply-result`.

### 6.5 TOTP two-step login

When a user account has TOTP enabled, `POST /api/v1/auth/login` returns HTTP 202 instead of 200:

```json
{
  "totpRequired": true,
  "pendingToken": "<short-lived JWT, 5 min TTL>",
  "expiresAt": "2026-04-06T12:05:00Z"
}
```

The client prompts for the six-digit code and calls:

```json
POST /api/v1/auth/totp/verify
{
  "pendingToken": "<pendingToken from above>",
  "code": "123456"
}
```

On success, the response is the same shape as a standard login (HTTP 200 with `token`, `expiresAt`, `user`).

**Setup flow (operator self-enrollment):**

1. `POST /api/v1/auth/totp/enroll` — returns `otpUri` (scan with any TOTP app) and `secret` (manual entry fallback).
2. Scan the QR code or enter the secret in an authenticator app (Google Authenticator, Authy, 1Password, etc.).
3. `POST /api/v1/auth/totp/confirm` with `{"code":"<first code from app>"}` — activates TOTP.
4. Subsequent logins require the TOTP code as a second factor.

To disable: `POST /api/v1/auth/totp/disable` with `{"password":"<current password>"}`.

**Server requirements:** `TOTP_ENCRYPTION_KEY` must be set (base64-encoded 32-byte key). When unset, all `/auth/totp/*` endpoints return 503.

### 6.6 LDAP/AD login

Only available when `AUTH_LDAP_URL` is configured. Confirm availability first:

```bash
curl -s /api/v1/auth/status | jq .ldapEnabled   # true when available
```

Request:
```json
POST /api/v1/auth/ldap/login
{
  "username": "alice",
  "password": "her-directory-password"
}
```

Response (same shape as local login):
```json
{
  "token": "eyJhbGciOi...",
  "expiresAt": "2026-03-18T12:00:00Z",
  "user": {
    "id": "usr_01HZ...",
    "email": "alice@example.com",
    "role": "operator",
    "authProvider": "ldap"
  }
}
```

The returned `token` is used identically to a local or OIDC JWT. See `docs/reference/ldap-auth.md` for configuration and role mapping details.

### 6.6 Keyless artifact ingest and attestations

Register an artifact with a keyless cosign signature:

```json
POST /api/v1/artifacts/pull
{
  "name": "firmware",
  "version": "2.0.0",
  "signatureType": "keyless",
  "signature": "<JSON-encoded cosign bundle>",
  "source": { "kind": "s3", "uri": "s3://bucket/firmware.tar.gz" }
}
```

Upload an in-toto / SLSA attestation after ingest:

```json
POST /api/v1/artifacts/{artifactId}/attestations
{
  "predicateType": "https://slsa.dev/provenance/v1",
  "payload": { ... },
  "signatureType": "keyless",
  "keylessBundle": "<JSON-encoded cosign bundle>"
}
```

List attestations:

```
GET /api/v1/artifacts/{artifactId}/attestations
```

See `docs/reference/artifact-provenance.md` for the full workflow including provenance policy configuration.

### 6.7 Vulnerability scan lifecycle

Only available when `VULN_ARTIFACT_SCANNER` is set to `grype` or `trivy` (artifact scanning) or `VULN_NESSUS_URL` is configured (device scanning).

**Artifact scan (auto-triggered at ingest):**

```
POST /api/v1/artifacts/upload   → scan triggered automatically in background
```

**Manual artifact rescan:**

```json
POST /api/v1/artifacts/{artifactId}/vulnerability-scans
```

Response: `{"scanId": "uuid", "status": "pending", "message": "scan triggered"}`

**Poll for result:**

```
GET /api/v1/artifacts/{artifactId}/vulnerability-scans/latest
```

Response:
```json
{
  "scanId": "uuid",
  "artifactId": "uuid",
  "scannerType": "grype",
  "scannerVersion": "0.74.0",
  "scanStatus": "completed",
  "scannedAt": "2026-03-17T10:00:00Z",
  "severityCounts": {"critical": 1, "high": 4, "medium": 12, "low": 3, "unknown": 0},
  "findings": [
    {"id": "CVE-2023-12345", "severity": "critical", "package": "openssl", "version": "3.0.1", "fixedIn": "3.0.2", "description": "..."}
  ]
}
```

`scanStatus` values: `pending` | `running` | `completed` | `failed` | `skipped`

**Device vulnerability scan (Nessus-sourced):**

```
GET /api/v1/devices/{deviceId}/vulnerability-scans/latest
```

Response mirrors the artifact scan shape; `scannerType` is `nessus`.

**Trigger immediate Nessus sync (admin only):**

```
POST /api/v1/vulnerability-scans/nessus/sync
```

**Check sync status:**

```
GET /api/v1/vulnerability-scans/nessus/status
```

See `docs/reference/vulnerability-scanning.md` for configuration details and device-matching setup.

### 6.8 Webhook lifecycle

1. **Create a webhook** with a target URL and one or more event types:

```json
POST /api/v1/webhooks
{
  "name": "ci-listener",
  "url": "https://ci.example.com/hooks/parcel",
  "secret": "shared-hmac-secret",
  "events": ["apply.result", "deploy.trigger"],
  "active": true
}
```

Response includes `webhookId`.

2. **Test connectivity:**

```bash
POST /api/v1/webhooks/{webhookId}/test
```

Response: `{"deliveryId": "uuid", "success": true, "statusCode": 200}`

3. **Check delivery history:**

```bash
GET /api/v1/webhooks/{webhookId}/deliveries
```

4. **Redeliver a failed delivery** (for example after listener downtime):

```bash
POST /api/v1/webhooks/{webhookId}/deliveries/{deliveryId}/redeliver
```

Redelivery creates a new delivery record with the original `payloadJson` and a fresh HMAC signature. The original failure record is preserved.

### 6.9 CI/CD feedback loop (trigger → webhook → poll)

This is the recommended pattern for CI pipelines that need fast, reliable feedback on a deployment.

**Prerequisites:**
- A service token with `artifact.publish`, `deployment.trigger`, and `artifact.read` scopes (or `webhook.manage` if self-managing the webhook)
- A webhook registered with `apply.result` events pointing to a CI-reachable listener

**Steps:**

1. Publish artifact (see §6.3).
2. Set desired state for the target group (`PUT /api/v1/desired-state/groups/{groupId}`).
3. Trigger immediate re-check-in for all group devices:

```bash
POST /api/v1/groups/{groupId}/trigger-apply
```

4. Wait for `apply.result` webhook deliveries — each carries `deviceId`, `status` (`success`|`error`), and `appliedVersion`.

5. Alternatively (or in addition), poll the deployment status endpoint:

```
GET /api/v1/groups/{groupId}/deployment-status?artifactId={artifactId}
```

Response:

```json
{
  "groupId": "uuid",
  "artifactId": "uuid",
  "total": 10,
  "pending": 2,
  "applied": 7,
  "failed": 1,
  "complete": false,
  "devices": [
    {
      "deviceId": "uuid",
      "status": "success",
      "appliedVersion": "2.1.0",
      "error": "",
      "lastApplyAt": "2026-03-24T10:05:00Z"
    }
  ]
}
```

`complete` is `true` when `pending == 0` and `total > 0`. Poll until `complete` or a timeout, then inspect `failed`.

**End-to-end script:** `scripts/test-ci-feedback-loop.sh` exercises this full flow against a running stack.

## 7) Guardrails and Security-Relevant Behavior

- Auth login is rate-limited and subject to backoff controls.
- Enrollment and check-in paths are rate-limited.
- Pending enrollment has queue and per-source/per-profile guardrails.
- Artifact pull ingest is host/timeout/size bounded by config.
- Signature policy can require signatures for ingest and/or apply.

Integrations must handle:
- `429` retry/backoff responses (respect `Retry-After` when present)
- `401/403` auth and permission failures
- `409` conflict states (for example enrollment claim conflicts)

## 8) Compatibility and Versioning

- Current stable namespace: `v1`
- Backward-compatible changes are additive (new fields/endpoints)
- Breaking changes require explicit compatibility callout and should move behind a new versioned surface
- Clients should not fail on unknown JSON fields

## 9) Related References

- Deployment: `docs/guides/deploy.md`
- Operations: `docs/guides/operations.md`
- Artifact ingest deep dive: `docs/development/artifact-ingest.md`
- Push vs pull guidance: `docs/development/artifact-ingest.md`
- First-contact onboarding deep dive: `docs/development/agent-first-contact-onboarding.md`
- LDAP/AD authentication: `docs/reference/ldap-auth.md`
- Keyless signing and attestations: `docs/reference/artifact-provenance.md`
- S3/GCS pull adapters and Vault credentials: `docs/reference/cloud-pull-adapters.md`
- Phase C developer internals: `docs/development/phase-c-internals.md`
- CI/CD feedback loop smoke test: `scripts/test-ci-feedback-loop.sh`
- Email delivery: `docs/reference/email-delivery.md`
- Vulnerability scanning: `docs/reference/vulnerability-scanning.md`
