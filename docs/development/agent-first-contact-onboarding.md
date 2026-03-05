# Agent First-Contact Approval Onboarding (Draft API + Workflow)

This draft defines a Phase C onboarding mode where agents start without a device client cert/key, request enrollment, wait for operator approval, then receive signed mTLS credentials.

It is additive to the existing enrollment flow (`POST /api/v1/devices/enroll`) and does not remove current token-based enroll.

## Goals
- No pre-shipped device client certificate required.
- Explicit operator confirmation before certificate issuance.
- Keep private key generation on-device (key never leaves the device).
- Preserve existing license/device-cap enforcement and hardware identity checks.

## Non-goals (for this draft)
- Zero-touch auto approval.
- TPM/secure attestation proof.
- Replacing existing direct token enrollment.

## Trust model (important)
Removing pre-shipped client certs does **not** remove server trust requirements.
Agent must trust control-plane TLS at first contact via one of:
- Public CA-backed server certificate, or
- Pinned control-plane server certificate/fingerprint.

## State machine
- `pending`: request created, waiting for operator action.
- `approved`: operator approved, waiting for agent claim.
- `issued`: cert issued and enrollment finalized (terminal).
- `denied`: operator denied (terminal).
- `expired`: request TTL exceeded (terminal).

## Agent-side bootstrap state machine

This is the exact agent runtime model that should replace the current dev-harness approach where bootstrap is performed outside the running agent process.

Current implementation status:
- The real Go agent now has an approval-mode scaffold behind `AGENT_ENROLL_MODE=approval`.
- The agent persists `bootstrap-state.json`, creates a pending enrollment request, polls claim, materializes `device.crt` + `device-id`, then switches into the normal mTLS check-in loop.
- Operator reset UX is now in place for denied/conflict/expired pending requests, and the agent will keep retrying so a reset can unblock the same request without manual file surgery.
- Anti-spam controls and richer profile lifecycle UX remain follow-up work.

### Agent config contract
New/updated agent config:
- `AGENT_ENROLL_MODE=token|approval`
- `CONTROL_PLANE_URL=https://...`
- `CONTROL_PLANE_CA_CERT_PATH=/path/to/ca.crt` or pinned public fingerprint
- `ENROLLMENT_PROFILE_TOKEN=...` for approval mode
- `DEVICE_CERT_PATH=/path/to/device.crt`
- `DEVICE_KEY_PATH=/path/to/device.key`
- `DEVICE_ID_PATH=/path/to/device-id`
- `BOOTSTRAP_STATE_PATH=/path/to/bootstrap-state.json`

In approval mode, the agent must start even if `DEVICE_CERT_PATH` does not exist yet.

### Persisted bootstrap state
The agent stores bootstrap progress in `bootstrap-state.json` so restart behavior is deterministic.

Example:
```json
{
  "version": 1,
  "mode": "approval",
  "state": "pending_approval",
  "requestId": "pe_01J...",
  "claimToken": "pec_...",
  "hardwareId": "sha256:...",
  "csrPath": "/var/lib/hardwareops/agent/device.csr",
  "keyPath": "/var/lib/hardwareops/agent/device.key",
  "certPath": "/etc/hardwareops/agent/device.crt",
  "deviceIdPath": "/etc/hardwareops/agent/device-id",
  "expiresAt": "2026-03-03T20:00:00Z",
  "lastError": "",
  "updatedAt": "2026-03-03T19:55:00Z"
}
```

Rules:
- `claimToken` is persisted locally because the agent must survive restart before approval.
- bootstrap state is cleared after successful issuance.
- private key never leaves the host.

### Exact runtime states

#### `bootstrap_init`
Entry:
- agent starts in `AGENT_ENROLL_MODE=approval`
- no valid device cert is present

Actions:
- validate bootstrap config
- validate control-plane trust anchor
- load or derive stable hardware identity
- load existing `bootstrap-state.json` if present

Transitions:
- valid device cert + device ID present -> `active`
- persisted unexpired request exists -> `pending_approval`
- missing required config -> `bootstrap_blocked`
- otherwise -> `keypair_ready`

#### `bootstrap_blocked`
Entry:
- missing profile token, no trust anchor, unreadable paths, or invalid bootstrap config

Actions:
- log a clear local error
- do not attempt check-in
- retry config validation on timer / process restart

Transitions:
- config corrected -> `bootstrap_init`

#### `keypair_ready`
Entry:
- no valid pending request to resume

Actions:
- reuse existing private key + CSR if both exist and are still intended for bootstrap
- otherwise generate new private key and CSR
- persist state

Transitions:
- CSR generated -> `request_submit`
- local crypto failure -> `bootstrap_backoff`

#### `request_submit`
Actions:
- call `POST /api/v1/pending-enrollments/request`
- persist returned `requestId`, `claimToken`, `expiresAt`

Transitions:
- `202 pending` -> `pending_approval`
- `401/403` invalid or expired profile token -> `bootstrap_blocked`
- `409` hardware identity conflict -> `bootstrap_conflict`
- `429` or transient `5xx` -> `bootstrap_backoff`
- malformed CSR / local request bug -> `bootstrap_backoff`

#### `pending_approval`
Actions:
- poll `POST /api/v1/pending-enrollments/claim`
- do not create a second request while current request is unexpired
- surface local status: `waiting for operator approval`

Transitions:
- `202 pending` -> remain `pending_approval`
- `200 issued` -> `materialize_identity`
- `410 denied` -> `bootstrap_denied`
- `410 expired` -> `keypair_ready`
- `401` invalid claim token -> `keypair_ready`
- transient `5xx` / network failure -> `bootstrap_backoff_resume_pending`

#### `bootstrap_backoff`
Actions:
- exponential backoff for request creation path
- retain key + CSR
- clear partially written response state

Transitions:
- timer expires -> `request_submit`

#### `bootstrap_backoff_resume_pending`
Actions:
- exponential backoff for polling path
- retain `requestId` + `claimToken`

Transitions:
- timer expires -> `pending_approval`

#### `bootstrap_denied`
Actions:
- log denial reason locally
- stop polling old request

Transitions:
- manual operator/user action resets bootstrap -> `keypair_ready`
- optional future policy may allow auto-retry after deny, but default should be manual reset

#### `bootstrap_conflict`
Actions:
- log that the hardware identity is already enrolled
- do not auto-regenerate identity

Transitions:
- manual operator resolution -> `bootstrap_init`

#### `materialize_identity`
Actions:
- atomically write:
  - `device.crt`
  - `device-id`
  - CA bundle if returned
- preserve generated private key
- fsync/rename semantics so restart cannot leave half-written identity files
- clear `bootstrap-state.json`

Transitions:
- all writes succeed -> `active`
- write failure -> `bootstrap_backoff_resume_pending`

#### `active`
Actions:
- standard mTLS check-in/apply loop
- bootstrap endpoints are no longer used

Transitions:
- cert missing/corrupt on startup -> `bootstrap_init`
- explicit reenroll/rotation workflow remains separate from bootstrap

### Restart behavior
- If the agent restarts in `pending_approval`, it must resume polling with the saved `requestId` and `claimToken`.
- It must not create a new request on every restart.
- If the pending request has expired, it transitions to `keypair_ready` and creates a fresh request.
- If `device.crt` and `device-id` already exist and are readable, bootstrap is skipped entirely.

### Production install behavior
Production install should be:
1. install agent binary/service
2. provide control-plane URL + trust anchor + enrollment profile token
3. start service immediately in approval mode
4. service remains alive while waiting for approval
5. after claim succeeds, same process moves into normal mTLS mode

This avoids shipping a client cert with the installer and avoids requiring a second installer pass after approval.

## Proposed API contract

### 1) Create enrollment profile (operator/admin)
`POST /api/v1/enrollment-profiles`

Creates a reusable bootstrap profile for first-contact onboarding.

Request:
```json
{
  "name": "factory-floor-a",
  "expiresInSec": 2592000,
  "maxUses": 500,
  "requireApproval": true,
  "challengeSecret": "factory-a-2026",
  "challengeHint": "Shared floor secret",
  "approvalDelaySec": 300,
  "allowUnsignedHardwareIdentity": false,
  "defaultLabels": {
    "site": "factory-a"
  }
}
```

Response `201`:
```json
{
  "profileId": "ep_01J...",
  "name": "factory-floor-a",
  "bootstrapToken": "ep_tok_...",
  "expiresAt": "2026-03-20T12:00:00Z",
  "maxUses": 500,
  "uses": 0,
  "requireApproval": true,
  "challengeEnabled": true,
  "challengeHint": "Shared floor secret",
  "approvalDelaySec": 300
}
```

Notes:
- Token is shown once (hashed at rest).
- RBAC: admin/operator with enrollment permission.
- `approvalDelaySec` must be shorter than the pending enrollment TTL.

### 1b) Rotate bootstrap token for an existing profile
`POST /api/v1/enrollment-profiles/{profileId}/rotate`

Optional request body:
```json
{
  "reason": "quarterly_rotation",
  "gracePeriodSec": 300
}
```

Response `200`:
```json
{
  "profileId": "ep_01J...",
  "name": "factory-floor-a",
  "bootstrapToken": "ep_tok_new_...",
  "expiresAt": "2026-03-20T12:00:00Z",
  "maxUses": 500,
  "uses": 12,
  "requireApproval": true,
  "challengeEnabled": true,
  "challengeHint": "Shared floor secret",
  "approvalDelaySec": 300,
  "tokenRotatedAt": "2026-03-04T13:00:00Z",
  "previousTokenGraceUntil": "2026-03-04T13:05:00Z"
}
```

Notes:
- `gracePeriodSec` is optional and capped at 86400 seconds.
- If `gracePeriodSec=0` (default), old distributed bootstrap tokens stop working immediately.
- If `gracePeriodSec>0`, only the immediately previous token remains valid until `previousTokenGraceUntil`.
- Pending requests already created keep their own claim tokens and are unaffected.

---

### 2) Request pending enrollment (agent, unauthenticated but token-bound)
`POST /api/v1/pending-enrollments/request`

Request:
```json
{
  "profileToken": "ep_tok_...",
  "challengeResponse": "factory-a-2026",
  "csr": "-----BEGIN CERTIFICATE REQUEST-----\n...\n-----END CERTIFICATE REQUEST-----\n",
  "agentVersion": "0.1.0",
  "capabilities": {
    "hw": {
      "identity": {
        "id": "sha256:...",
        "source": "machine-id"
      }
    }
  },
  "metadata": {
    "hostname": "kiosk-17",
    "os": "linux"
  }
}
```

Response `202`:
```json
{
  "requestId": "pe_01J...",
  "status": "pending",
  "claimToken": "pec_...",
  "pollAfterSec": 5,
  "expiresAt": "2026-02-21T12:15:00Z"
}
```

Validation:
- CSR format/size/SAN/CN limits (same policy as current enroll).
- Rate limit by source IP and profile.
- Active queue caps by source IP, profile, and globally.
- Optional challenge secret for internet-exposed profiles.
- Optional hardware identity required (policy).
- Profile token validity + usage limits.

---

### 3) List pending requests (operator/admin)
`GET /api/v1/pending-enrollments?status=pending&limit=100`

Response `200`:
```json
{
  "items": [
    {
      "requestId": "pe_01J...",
      "status": "pending",
      "createdAt": "2026-02-20T18:00:00Z",
      "expiresAt": "2026-02-20T18:15:00Z",
      "approvalAvailableAt": "2026-02-20T18:05:00Z",
      "profileId": "ep_01J...",
      "sourceIp": "203.0.113.10",
      "agentVersion": "0.1.0",
      "metadata": {
        "hostname": "kiosk-17"
      },
      "capabilities": {
        "hw": {
          "identity": {
            "id": "sha256:..."
          }
        }
      }
    }
  ]
}
```

---

### 4) Approve request (operator/admin)
`POST /api/v1/pending-enrollments/{requestId}/approve`

Request:
```json
{
  "labels": {
    "site": "factory-a",
    "role": "kiosk"
  },
  "groups": ["g_factory_a", "g_kiosk"],
  "notes": "Approved after serial verification"
}
```

Response `200`:
```json
{
  "requestId": "pe_01J...",
  "status": "approved",
  "approvedAt": "2026-02-20T18:01:10Z"
}
```

Checks at approve/issue boundary:
- License cap availability.
- Hardware identity conflict policy.
- Request not expired/consumed.

---

### 5) Deny request (operator/admin)
`POST /api/v1/pending-enrollments/{requestId}/deny`

Request:
```json
{
  "reason": "unknown hardware"
}
```

Response `200`:
```json
{
  "requestId": "pe_01J...",
  "status": "denied",
  "deniedAt": "2026-02-20T18:01:40Z"
}
```

---

### 6) Claim approval and receive cert (agent)
`POST /api/v1/pending-enrollments/claim`

Request:
```json
{
  "requestId": "pe_01J...",
  "claimToken": "pec_..."
}
```

Responses:
- `202` (still pending):
```json
{
  "status": "pending",
  "pollAfterSec": 5
}
```

- `200` (approved, certificate issued):
```json
{
  "status": "issued",
  "deviceId": "7ca73149-98cb-49d0-a2d9-a0248720edf5",
  "certPem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
  "caCertPem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n"
}
```

- `410` (denied/expired):
```json
{
  "status": "denied",
  "reason": "unknown hardware"
}
```

Issuance behavior:
- Sign CSR only at claim time after approval.
- Consume request + claim token atomically.
- Create/enroll device record in one transaction.

## Local test runbook

Current state:
- `scripts/run-demo-agent.sh` supports both:
  - legacy direct enroll (`ENROLLMENT_MODE=legacy`, default)
  - pending first-contact onboarding (`ENROLLMENT_MODE=pending`)
- `scripts/test-pending-enrollment.sh` is still available for lower-level endpoint testing without launching the demo container.

### 1) Start dependencies
From repo root:
```bash
cp deploy/compose/.env.example deploy/compose/.env
make dev-up
```

### 2) Start the control-plane locally
Simplest auth-enabled local setup:
```bash
AUTH_MODE=local \
AUTH_JWT_SECRET=dev-jwt-secret-change-me \
AUTH_BOOTSTRAP_EMAIL=admin@example.com \
AUTH_BOOTSTRAP_PASSWORD=change-me \
MAINTENANCE_MODE=0 \
ENABLE_TLS=1 \
./scripts/run-control-plane.sh
```

If you want no login friction for this specific test:
```bash
AUTH_MODE=disabled \
MAINTENANCE_MODE=0 \
ENABLE_TLS=1 \
./scripts/run-control-plane.sh
```

### 3) Start the UI
```bash
cd ui
npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
```

### 4) Run the pending-enrollment demo agent
Auth enabled:
```bash
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
ENROLLMENT_MODE=pending \
./scripts/run-demo-agent.sh
```

Auth disabled:
```bash
ENROLLMENT_MODE=pending ./scripts/run-demo-agent.sh
```

What the script does:
- creates an enrollment profile
- generates a local device key + CSR
- submits `POST /api/v1/pending-enrollments/request`
- prints the request ID
- polls `POST /api/v1/pending-enrollments/claim` every few seconds
- after issuance, starts the demo agent container with the issued cert

Artifacts are written under a temp directory such as:
```text
/tmp/hardwareops-pending-enroll.xxxxxx
```

That directory contains:
- `device.key`
- `device.csr`
- `request.json`
- `claim.json`
- `profile.json`
- after approval: `device.crt`, `ca.crt`, `device-id`

### 5) Approve in the UI
In the UI:
- open `http://localhost:5173`
- sign in if auth is enabled
- go to `Security`
- find the request in `Pending enrollments`
- click `Approve`

The polling script will detect approval, write out the issued cert + CA cert, and then launch the demo container.

### 6) Optional: approve by API instead of UI
```bash
curl --cacert ./dev-ca.crt \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -X POST https://localhost:8080/api/v1/pending-enrollments/<request-id>/approve \
  -H 'Content-Type: application/json' \
  -d '{}'
```

Or auto-approve with the demo launcher:
```bash
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
ENROLLMENT_MODE=pending \
PENDING_ENROLL_AUTO_APPROVE=1 \
./scripts/run-demo-agent.sh
```

To attach profile labels and let the first post-claim check-in resolve group desired state immediately:
```bash
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
PROFILE_DEFAULT_LABELS_JSON='{"site":"factory-a","role":"kiosk"}' \
GROUP_DESIRED_VERSION=v9 \
CHECKIN_AFTER_CLAIM=1 \
./scripts/test-pending-enrollment.sh
```

To use the same profile-label path with the demo launcher:
```bash
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
ENROLLMENT_MODE=pending \
PENDING_ENROLL_PROFILE_DEFAULT_LABELS_JSON='{"site":"factory-a","role":"kiosk"}' \
./scripts/run-demo-agent.sh
```

### 7) Lower-level endpoint helper
If you only want to test profile creation, pending request, UI approval, and claim issuance without launching the demo container:
```bash
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
./scripts/test-pending-enrollment.sh
```

## End-to-end workflow
1. Operator creates an enrollment profile and distributes the bootstrap token out-of-band.
2. Agent starts with:
   - control-plane URL
   - profile token
   - server trust anchor (public CA or pinned cert/fingerprint)
3. Agent generates keypair locally and CSR.
4. Agent calls `pending-enrollments/request`.
5. Request appears in UI approval queue.
6. Operator approves or denies.
7. Agent polls `pending-enrollments/claim`.
8. On approval, control-plane issues cert + device ID.
9. Agent stores cert/key and switches to normal mTLS `checkin`.

## Error contract (summary)
- `400`: invalid JSON/CSR/payload.
- `401`: invalid profile token or claim token.
- `403`: profile disabled or policy forbidden.
- `409`: hardware identity conflict / duplicate request conflict.
- `410`: pending request denied or expired.
- `429`: rate limit triggered.
- `429`: rate limit triggered, queue cap reached, or approval delay window still active.
- `500`: internal/storage/signing errors.

## Audit, events, metrics
Audit actions:
- `pending_enrollment.request`
- `pending_enrollment.approve`
- `pending_enrollment.deny`
- `pending_enrollment.issue`
- `pending_enrollment.expire`

Runtime events:
- `device.enroll_pending`
- `device.enroll_approved`
- `device.enroll_denied`
- `device.enroll`

Metrics:
- `hwops_enroll_total{status,reason}`
- `hwops_pending_enroll_active_total`
- `hwops_pending_enroll_queue_age_total{bucket}` (`lt_1m`, `1m_5m`, `5m_15m`, `15m_1h`, `gte_1h`)
- `hwops_pending_enroll_oldest_age_seconds`
- `hwops_pending_enroll_throttle_total{reason}`

## Backward compatibility and rollout
- Keep current `POST /api/v1/devices/enroll` path unchanged.
- Add agent mode flag:
  - `AGENT_ENROLL_MODE=token` (existing behavior, default)
  - `AGENT_ENROLL_MODE=approval` (new first-contact workflow)
- UI adds pending-enrollment queue and approve/deny actions.

## Security controls checklist
- Profile tokens and claim tokens hashed at rest.
- Short TTLs and one-time claim semantics.
- Per-IP and per-profile request limits.
- Optional challenge/proof gate for internet-exposed onboarding endpoints.
- Approval delay throttles for profiles that require an operator soak window.
- Pending queue caps with automatic expiry cleanup before request/list/approve operations.
- Strict allowlist for trusted forwarded headers.
- Full audit trail for request/approval/issuance.

### Runtime guardrails
Control-plane knobs for first-contact abuse resistance:

- `PENDING_ENROLL_REQUEST_RPM_PER_SOURCE`
- `PENDING_ENROLL_REQUEST_RPM_PER_PROFILE`
- `PENDING_ENROLL_MAX_ACTIVE`
- `PENDING_ENROLL_MAX_ACTIVE_PER_PROFILE`
- `PENDING_ENROLL_MAX_ACTIVE_PER_SOURCE`
