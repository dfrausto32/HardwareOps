# Device Identity Hardening (Phase B Draft + v1 Implementation)

This document defines the Phase B hardening approach for reducing duplicate or cloned device abuse.

## Goal
- Make "one physical device = one enrolled device" harder to bypass in on-prem deployments.
- Keep compatibility with existing agents and rollout workflows.

## v1 (Implemented)

### Identity source and payload
- Agents now include a hardware identity in `capabilities.hw.identity` on check-in:
  - `id`: stable hash (sha256) from machine-id (fallback hostname, or `HARDWARE_IDENTITY` override).
  - `source`: where the identity was derived from (`machine-id`, `hostname`, `env`, demo-specific source).

### Policy controls
Control-plane env vars:

```env
DEVICE_IDENTITY_MODE=audit
DEVICE_IDENTITY_REQUIRE_ON_ENROLL=0
DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=0
```

Modes:
- `disabled`: no hardware-identity checks.
- `audit`: detect and emit telemetry/audit without blocking traffic.
- `enforce`: reject identity conflicts.

Require flags (only meaningful in `enforce` mode):
- `DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1`: reject enroll requests missing hardware identity.
- `DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1`: reject check-ins missing hardware identity.

### Server-side behavior
- Enrollment (`POST /api/v1/devices/enroll`):
  - Accepts optional `capabilities`.
  - Rejects with `409` if hardware identity already belongs to another device.
  - Stores hardware identity in device metadata when present.
- Check-in (`POST /api/v1/devices/checkin`):
  - Detects identity mismatch/reuse and emits runtime/audit signals.
  - In `enforce` mode, rejects conflicts with `409`.

### Telemetry and audit
- Runtime event: `device.identity_conflict`.
- Audit action: `device.identity_violation` (and `device.enroll_rejected` on enroll conflicts).

## Why this helps (and limits)
- Helps against accidental cloning and simple copy/replay attempts.
- Not tamper-proof on fully hostile hardware (a privileged attacker can still forge software-reported identity).
- Stronger guarantees require hardware attestation (TPM/secure element), deferred to later phases.

## Demo/local notes
- `scripts/run-demo-agent.sh` and `agent/demo/aws-entrypoint.sh` now include enrollment capabilities so `enforce` + `require_on_enroll` can be tested.
- `scripts/agent-enroll.sh` now includes hardware identity in enroll payload by default.

## Verification quick checks
1) Start control-plane with `DEVICE_IDENTITY_MODE=enforce`.
2) Enroll/check-in one device.
3) Attempt second enroll/check-in with the same hardware identity.
4) Expect:
   - HTTP `409` on conflicting request.
   - `device.identity_conflict` event in logs/events history.
   - `device.identity_violation` audit entry.
