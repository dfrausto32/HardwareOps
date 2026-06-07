# TLS + CA Setup (On‑Prem)

Canonical operations guide: `operations.md`  
Use this file for CA/TLS and rotation detail.

This uses a local internal CA to issue the server certificate for `parcel.internal`
and the agent endpoint host `agent.parcel.internal` (SAN).

## Formal split: Server TLS vs Device CA

There are two different trust chains:

- **Device CA (agent mTLS):** `ca.crt` + `ca.key`
  - Used for device enrollment, check-in mTLS, and CA rotation.
- **Server TLS cert (browser/UI):** `server.crt` + `server.key`
  - Used by the gateway HTTPS endpoint for human users.

Recommended production posture:
- Keep device CA private to Parcel agent trust.
- Use an enterprise/publicly trusted server cert for `server.crt`/`server.key`.
- In on-prem scripts, set `SERVER_CERT_MODE=external`.

## Certificate Rotation — What It Is (and What It Isn’t)
Rotation means **switching the device‑signing CA** to a new one without breaking mTLS.
You do **not** need a “pile” of certs — you need:

**Minimum files for rotation**
- **Old CA** (current): `ca.crt`, `ca.key`
- **New CA** (active signer): `ca-active.crt`, `ca-active.key`
- **Bundle** (trusts both): `ca-bundle.crt` (old + new)

Optional (separate concern):
- **Server TLS cert** (`server.crt`, `server.key`) — rotate later, after devices are re‑enrolled.

**Why this works**
1) The control‑plane **trusts both** old + new (bundle).
2) The control‑plane **signs new device certs** with the active CA.
3) Devices detect they’re on the old CA → **auto re‑enroll** → switch to new CA.

**License safety**
Re‑enroll updates the existing device record; it does **not** consume a new license slot.

## 1) Generate CA
```
sudo OUT_DIR=/opt/parcel/certs ./scripts/bootstrap-ca.sh
```

Outputs:
- `/opt/parcel/certs/ca.crt`
- `/opt/parcel/certs/ca.key`

## 2) Issue server cert
```
sudo OUT_DIR=/opt/parcel/certs DOMAIN=parcel.internal AGENT_DOMAIN=agent.parcel.internal ./scripts/issue-server-cert.sh
```

### Fast path (CA + server cert + env)
```
sudo OUT_DIR=/opt/parcel/certs DOMAIN=parcel.internal AGENT_DOMAIN=agent.parcel.internal ./scripts/setup-control-plane.sh
```

### External server cert path (recommended for UI without local CA install)
Use this when you have a trusted cert/key from enterprise PKI or a public CA:
```
sudo OUT_DIR=/opt/parcel/certs \
  SERVER_CERT_MODE=external \
  SERVER_CERT_INPUT=/path/to/trusted-server.crt \
  SERVER_KEY_INPUT=/path/to/trusted-server.key \
  ./scripts/setup-control-plane.sh
```
This still creates/uses `ca.crt` + `ca.key` for device mTLS, but leaves browser trust to your external server cert.

If you change domains or see TLS errors (AKI/SKI mismatch), reissue with:
```
sudo FORCE=1 OUT_DIR=/opt/parcel/certs DOMAIN=parcel.internal AGENT_DOMAIN=agent.parcel.internal ./scripts/setup-control-plane.sh
```

Outputs:
- `/opt/parcel/certs/server.crt`
- `/opt/parcel/certs/server.key`

## 3) Trust the CA on clients
### Linux (system trust)
```
sudo cp /opt/parcel/certs/ca.crt /usr/local/share/ca-certificates/parcel-ca.crt
sudo update-ca-certificates
```

### Windows
1. Open `ca.crt`
2. Install to **Trusted Root Certification Authorities**

### macOS
1. Add `ca.crt` to Keychain Access
2. Set to **Always Trust**

### Bootstrap download flow (no auth session yet)
If on-prem auth is enabled and you want a pre-login path to fetch CA cert:
```
curl -k -H "X-Bootstrap-Token: <BOOTSTRAP_TOKEN>" \
  https://parcel.internal/api/v1/bootstrap/ca -o parcel-ca.crt
```
Then install `parcel-ca.crt` in system/browser trust and use the UI normally.

## 4) Agent trust (no system trust required)
Agents can point directly to the CA:
```
CONTROL_PLANE_CA_CERT_PATH=/etc/parcel/agent/certs/ca.crt
```

---

## CA Rotation Runbook + Verification

**Goal:** introduce a new device‑signing CA without breaking mTLS. Device limits are not affected because re‑enroll updates the existing device record (no new enrollment).

### 1) Prepare the new CA
```
sudo OUT_DIR=/opt/parcel/certs ./scripts/bootstrap-ca.sh
```
Save the new CA as:
- `/opt/parcel/certs/ca-active.crt`
- `/opt/parcel/certs/ca-active.key`

### 2) Build a trust bundle (old + new)
```
cat /opt/parcel/certs/ca.crt /opt/parcel/certs/ca-active.crt > /opt/parcel/certs/ca-bundle.crt
```

### 3) Switch the control‑plane to “dual‑trust + active‑sign”
Set in the control‑plane environment:
```
CA_BUNDLE_PATH=/opt/parcel/certs/ca-bundle.crt
ACTIVE_CA_CERT_PATH=/opt/parcel/certs/ca-active.crt
ACTIVE_CA_KEY_PATH=/opt/parcel/certs/ca-active.key
```
Restart the control‑plane.

### Optional: reload without restart (operator/admin)
If you want to rotate without restarting, use the reload endpoint or the UI button:
```
curl --cacert /opt/parcel/certs/ca.crt \
  -H "Authorization: Bearer <operator-or-admin-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"reload CA bundle after emergency file update"}' \
  https://parcel.internal/api/v1/cert-rotation/reload
```
This reloads the active CA + client bundle in‑process.

### Rotate (operator/admin, generates new CA + bundle + reload)
```
curl --cacert /opt/parcel/certs/ca.crt \
  -H "Authorization: Bearer <operator-or-admin-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"break-glass CA rotation after suspected compromise"}' \
  https://parcel.internal/api/v1/cert-rotation/rotate
```
This generates a new active CA, rebuilds the bundle, and reloads without restart.

**Helper script (dev/on‑prem)**
```
scripts/rotate--ca.sh
```
Environment overrides:
```
CERTS_DIR=/opt/parcel/certs
BASE_URL=https://parcel.internal
AUTH_TOKEN=<admin-jwt>
CA_BUNDLE_PATH=/opt/parcel/certs/ca-bundle.crt
ACTIVE_CA_CERT_PATH=/opt/parcel/certs/ca-active.crt
ACTIVE_CA_KEY_PATH=/opt/parcel/certs/ca-active.key
```

### 4) Verify rotation status (UI + API)
```
curl --cacert /opt/parcel/certs/ca.crt https://parcel.internal/api/v1/cert-rotation
```
Expect:
- Active CA fingerprint present
- Bundle shows `containsActive=true`
- Devices begin moving from **needs reenroll** → **active**

### 5) Device re‑enroll behavior
Agents that present certs from the old CA receive `device.reenroll` and automatically swap their certs.

### 6) License safety check
Re‑enroll does **not** consume device slots. Verify:
```
curl --cacert /opt/parcel/certs/ca.crt https://parcel.internal/api/v1/license
```
Device count should be unchanged before/after rotation.

### 7) Optional: rotate the server TLS cert
Once **all devices** are re‑enrolled:
```
sudo OUT_DIR=/opt/parcel/certs DOMAIN=parcel.internal ./scripts/issue-server-cert.sh
```
Point the gateway to the new server cert/key, and update client trust stores as needed.

### 8) Cleanup (remove old CA)
After all devices are active on the new CA (or after the grace window), prune the old CA:

**UI / API (hybrid cleanup)**
```
curl --cacert /opt/parcel/certs/ca.crt \
  -H "Authorization: Bearer <operator-or-admin-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"cleanup previous CA after rotation coverage verified"}' \
  https://parcel.internal/api/v1/cert-rotation/cleanup
```

**Manual fallback**
```
cat /opt/parcel/certs/ca-active.crt > /opt/parcel/certs/ca-bundle.crt
```

Grace window (default 7 days):
```
CERT_ROTATION_GRACE_PERIOD=168h
```
If the grace window expires, cleanup is allowed even if some devices are still on the old CA (they will need manual intervention).
