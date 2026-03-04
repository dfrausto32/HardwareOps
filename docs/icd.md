# HardwareOps Integration Control Document (ICD)

Status: Active (living document)  
API namespace: `v1`  
Last updated: 2026-03-04  
Source of truth for routes: `control-plane/internal/httpapi/router.go`

This is the canonical contract for integrating with HardwareOps without relying on the GUI.

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

Current scope gate:
- `artifact.publish` allows pull/presign/complete ingest flows without user JWT

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
| GET | `/api/v1/auth/status` | public | auth mode and enabled state |
| POST | `/api/v1/auth/login` | public | rate limited, returns JWT |
| POST | `/api/v1/auth/register` | public | voucher/bootstrap-driven local registration |
| GET | `/api/v1/auth/me` | viewer | caller identity |
| POST | `/api/v1/auth/vouchers` | admin | create registration voucher |
| POST | `/api/v1/auth/service-tokens` | admin | create service token |
| GET | `/api/v1/auth/service-tokens` | admin | list service tokens |
| POST | `/api/v1/auth/service-tokens/{tokenId}/revoke` | admin | revoke service token |
| POST | `/api/v1/users` | admin | create local user |
| GET | `/api/v1/users` | admin | list users |
| PATCH | `/api/v1/users/{userId}` | admin | update user |

### 5.2 Groups + Desired State

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/groups` | viewer | list group definitions |
| POST | `/api/v1/groups/batch` | operator | bulk group operations |
| PUT | `/api/v1/groups/{groupId}` | operator | upsert group |
| DELETE | `/api/v1/groups/{groupId}` | operator | delete group |
| GET | `/api/v1/desired-state` | viewer | list desired-state records |
| PUT | `/api/v1/desired-state/groups/{groupId}` | operator | set group desired state |
| DELETE | `/api/v1/desired-state/groups/{groupId}` | operator | clear group desired state |
| PUT | `/api/v1/desired-state/devices/{deviceId}` | operator | set device override |
| DELETE | `/api/v1/desired-state/devices/{deviceId}` | operator | clear device override |

### 5.3 Devices

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/devices` | viewer | list devices |
| GET | `/api/v1/devices/{deviceId}` | viewer | device detail |
| PATCH | `/api/v1/devices/{deviceId}` | operator | update metadata/grouping fields |
| DELETE | `/api/v1/devices/{deviceId}` | operator | delete device record |
| POST | `/api/v1/devices/{deviceId}/decommission` | admin | terminal decommission |
| POST | `/api/v1/devices/{deviceId}/apply-result` | device path | rate limited |
| POST | `/api/v1/devices/checkin` | device path | rate limited |
| POST | `/api/v1/devices/reenroll` | device path | rate limited |

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
| GET | `/api/v1/artifacts` | viewer | list artifacts |
| GET | `/api/v1/artifacts/{artifactId}` | viewer | artifact detail |
| POST | `/api/v1/artifacts` | operator | register artifact metadata |
| POST | `/api/v1/artifacts/upload` | operator | multipart upload + register |
| POST | `/api/v1/artifacts/pull` | operator or service token (`artifact.publish`) | control-plane pull ingest |
| POST | `/api/v1/artifacts/presign-upload` | operator or service token (`artifact.publish`) | presigned upload init |
| POST | `/api/v1/artifacts/complete` | operator or service token (`artifact.publish`) | finalize presigned upload |
| POST | `/api/v1/artifacts/{artifactId}/deprecate` | operator | deprecate artifact |
| POST | `/api/v1/artifacts/{artifactId}/restore` | operator | restore deprecated artifact |
| DELETE | `/api/v1/artifacts/{artifactId}` | operator | hard delete (guarded by refs/policy) |
| POST | `/api/v1/artifacts/{artifactId}/presign` | public route | presign download; integration should treat as sensitive |
| GET | `/api/v1/artifacts/lifecycle/policy` | viewer | lifecycle policy |
| PUT | `/api/v1/artifacts/lifecycle/policy` | admin | set lifecycle policy |
| GET | `/api/v1/artifacts/lifecycle/status` | viewer | lifecycle status summary |
| POST | `/api/v1/artifacts/lifecycle/prune` | admin | immediate prune run |
| GET | `/api/v1/artifacts/pull-credentials` | admin | credential resolver status |
| POST | `/api/v1/artifacts/pull-credentials/reload` | admin | reload resolver configuration |

### 5.6 Security, Audit, Events, Health

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/api/v1/license` | admin | license/device-cap state |
| GET | `/api/v1/release-auto-update` | viewer | release auto-update settings |
| PUT | `/api/v1/release-auto-update` | admin | update auto-update policy |
| POST | `/api/v1/release-auto-update/run` | admin | force auto-update evaluation |
| GET | `/api/v1/cert-rotation` | viewer | CA rotation status |
| POST | `/api/v1/cert-rotation/reload` | admin | reload CA bundle/files |
| POST | `/api/v1/cert-rotation/rotate` | admin | create/switch active CA |
| POST | `/api/v1/cert-rotation/cleanup` | admin | cleanup old/inactive CAs |
| GET | `/api/v1/audit` | admin | list audit events |
| GET | `/api/v1/audit.csv` | admin | CSV export |
| GET | `/api/v1/audit/retention` | admin | audit retention config |
| PUT | `/api/v1/audit/retention` | admin | set audit retention |
| GET | `/api/v1/events/history` | viewer | runtime event history |
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

### 5.8 Metrics

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

- Deployment: `docs/deploy.md`
- Operations: `docs/operations.md`
- Artifact ingest deep dive: `docs/development/artifact-ingest.md`
- Push vs pull guidance: `docs/development/push-vs-pull-workflows.md`
- First-contact onboarding deep dive: `docs/development/agent-first-contact-onboarding.md`
