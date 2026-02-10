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
- Copy `license.json` to the target host, e.g. `/opt/hardwareops/license.json`
- Provide the **public key** to the control‑plane via `LICENSE_PUBLIC_KEY_PATH`

## Control‑Plane Configuration
Set these in `.env.onprem` (or control‑plane env):
```
LICENSE_ENFORCE=1
LICENSE_PATH=/opt/hardwareops/license.json
LICENSE_PUBLIC_KEY_PATH=/opt/hardwareops/license.pub
LICENSE_CACHE_TTL=30s
```

Notes:
- `LICENSE_PUBLIC_KEY_PATH` points to the PEM public key.
- The control‑plane **reloads the license file** on change (default cache TTL 30s).
- If the license is invalid or expired, **enrollments will be blocked**.

## Increasing the Cap (no reinstall)
1) Generate a **new license file** with the higher `maxDevices`.
2) Replace the file at `LICENSE_PATH`.
3) Wait for `LICENSE_CACHE_TTL` (or restart the control‑plane).

## Enforcement Behavior
- **Enrollments** are blocked when cap is exceeded.
- Existing devices can continue to check in.
