# License Enforcement (On‑Prem Device Cap)

This document describes the **signed license file** used to enforce a hard
device limit in on‑prem deployments.

## Why
On‑prem customers need a **fixed device cap** that operators cannot change.
The cap can be **increased** by replacing the signed license file (no reinstall,
no software update).

## File Format
```json
{
  "payload": {
    "issuedTo": "Customer Name",
    "maxDevices": 250,
    "notBefore": "2026-02-10T00:00:00Z",
    "expiresAt": "2027-02-10T00:00:00Z"
  },
  "signature": "<base64>",
  "keyId": "sha256:<pubkey-hash>"
}
```

### Required fields
- `payload.issuedTo`
- `payload.maxDevices` (must be > 0)
- `signature` (Ed25519 over the JSON payload bytes)

### Optional fields
- `notBefore`, `expiresAt` (RFC3339 timestamps)
- `keyId` (for traceability)

## How to Sign a License (vendor‑side)
1) Generate an Ed25519 keypair:
```bash
./scripts/generate-signing-key.sh ./license-keys
```

2) Create a signed license:
```bash
LICENSE_KEY=./license-keys/ed25519.key \
ISSUED_TO="Customer Name" \
MAX_DEVICES=250 \
EXPIRES_AT="2027-02-10T00:00:00Z" \
OUT=./license.json \
./scripts/sign-license.sh
```

3) Distribute:
- Copy `license.json` to the target host, e.g. `/opt/parcel/license.json`
- Provide the **public key** to the control‑plane via `LICENSE_PUBLIC_KEY_PATH`

## Control‑Plane Configuration
Set these in `.env.onprem` (or control‑plane env):
```
LICENSE_ENFORCE=1
LICENSE_PATH=/opt/parcel/license.json
LICENSE_PUBLIC_KEY_PATH=/opt/parcel/license.pub
LICENSE_KEY_MODE=embedded
LICENSE_CACHE_TTL=30s
```

Notes:
- **Locked mode:** set `LICENSE_KEY_MODE=embedded` and build the control‑plane with an embedded public key.
- `LICENSE_PUBLIC_KEY_PATH` is only used when `LICENSE_KEY_MODE=env`.
- The control‑plane **reloads the license file** on change (default cache TTL 30s).
- If the license is invalid or expired, **enrollments will be blocked**.

## Increasing the Cap (no reinstall)
1) Generate a **new license file** with the higher `maxDevices`.
2) Replace the file at `LICENSE_PATH`.
3) Wait for `LICENSE_CACHE_TTL` (or restart the control‑plane).

## Embedding the Public Key (Locked Mode)
Build with an embedded key (PEM public key):
```bash
LICENSE_EMBED_PUBKEY_PATH=./license-keys/ed25519.pub \
./scripts/build-installers.sh
```

For upgrade bundles:
```bash
LICENSE_EMBED_PUBKEY_PATH=./license-keys/ed25519.pub \
./scripts/build-upgrade-package.sh
```

## Enforcement Behavior
- **Enrollments** are blocked when cap is exceeded.
- Existing devices can continue to check in.
- Re-enroll updates an existing device record and does not consume a new slot.

## Explicit Slot Reclaim (Decommission)
Device-slot reclaim now uses an explicit decommission API (admin-only) so slot release is auditable.

```bash
curl --cacert /opt/parcel/certs/ca.crt \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  https://parcel.internal/api/v1/devices/<device-id>/decommission \
  -d '{"reason":"device retired","ticketId":"OPS-123"}'
```

Response includes slot counts (`slotBefore`, `slotAfter`) and whether a slot was released.
Audit action emitted:
- `device.decommission`

## Optional Duplicate-Device Hardening (Phase B)
To make cloning/reuse more painful, enable hardware identity checks:

```env
DEVICE_IDENTITY_MODE=enforce
DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1
DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1
```

Details and rollout strategy: `device-identity-hardening.md`.

## Hardened Runtime Profile (recommended for production on-prem)
Use this to block accidental insecure startup combinations:

```env
HARDENED_PROFILE=1
AUTH_MODE=local
LICENSE_ENFORCE=1
DEVICE_IDENTITY_MODE=enforce
DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1
DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1
```

When `HARDENED_PROFILE=1`, startup fails if these requirements are not met.
