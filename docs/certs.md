# TLS + CA Setup (On‑Prem)

This uses a local internal CA to issue the server certificate for `hardwareops.internal`.

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
sudo OUT_DIR=/opt/hardwareops/certs ./scripts/bootstrap-ca.sh
```

Outputs:
- `/opt/hardwareops/certs/ca.crt`
- `/opt/hardwareops/certs/ca.key`

## 2) Issue server cert
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/issue-server-cert.sh
```

### Fast path (CA + server cert + env)
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

If you change domains or see TLS errors (AKI/SKI mismatch), reissue with:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

Outputs:
- `/opt/hardwareops/certs/server.crt`
- `/opt/hardwareops/certs/server.key`

## 3) Trust the CA on clients
### Linux (system trust)
```
sudo cp /opt/hardwareops/certs/ca.crt /usr/local/share/ca-certificates/hardwareops-ca.crt
sudo update-ca-certificates
```

### Windows
1. Open `ca.crt`
2. Install to **Trusted Root Certification Authorities**

### macOS
1. Add `ca.crt` to Keychain Access
2. Set to **Always Trust**

## 4) Agent trust (no system trust required)
Agents can point directly to the CA:
```
CONTROL_PLANE_CA_CERT_PATH=/etc/hardwareops/agent/certs/ca.crt
```

---

## Certificate Rotation (Phase A)
Rotation is supported by trusting **multiple CAs** on the control‑plane and automatically re‑issuing device certs.

### Control‑plane settings
```
# Trust both old + new for device mTLS
CA_BUNDLE_PATH=/opt/hardwareops/certs/ca-bundle.crt

# Sign new device certs with the active CA
ACTIVE_CA_CERT_PATH=/opt/hardwareops/certs/ca-active.crt
ACTIVE_CA_KEY_PATH=/opt/hardwareops/certs/ca-active.key
```

Create the bundle:
```
cat /opt/hardwareops/certs/ca.crt /opt/hardwareops/certs/ca-active.crt > /opt/hardwareops/certs/ca-bundle.crt
```

### Agent behavior (automatic)
If the device cert is **not signed by the active CA**, the control‑plane returns a `device.reenroll` action.
The agent automatically re‑enrolls and replaces its device cert.

> Note: This does **not** rotate the server TLS cert. Keep server TLS on the existing CA until you explicitly rotate it.

---

## CA Rotation Runbook + Verification

**Goal:** introduce a new device‑signing CA without breaking mTLS. Device limits are not affected because re‑enroll updates the existing device record (no new enrollment).

### 1) Prepare the new CA
```
sudo OUT_DIR=/opt/hardwareops/certs ./scripts/bootstrap-ca.sh
```
Save the new CA as:
- `/opt/hardwareops/certs/ca-active.crt`
- `/opt/hardwareops/certs/ca-active.key`

### 2) Build a trust bundle (old + new)
```
cat /opt/hardwareops/certs/ca.crt /opt/hardwareops/certs/ca-active.crt > /opt/hardwareops/certs/ca-bundle.crt
```

### 3) Switch the control‑plane to “dual‑trust + active‑sign”
Set in the control‑plane environment:
```
CA_BUNDLE_PATH=/opt/hardwareops/certs/ca-bundle.crt
ACTIVE_CA_CERT_PATH=/opt/hardwareops/certs/ca-active.crt
ACTIVE_CA_KEY_PATH=/opt/hardwareops/certs/ca-active.key
```
Restart the control‑plane.

### Optional: reload without restart (admin)
If you want to rotate without restarting, use the reload endpoint or the UI button:
```
curl --cacert /opt/hardwareops/certs/ca.crt -X POST \\
  https://hardwareops.internal/api/v1/cert-rotation/reload
```
This reloads the active CA + client bundle in‑process.

### Rotate (admin, generates new CA + bundle + reload)
```
curl --cacert /opt/hardwareops/certs/ca.crt -X POST \\
  https://hardwareops.internal/api/v1/cert-rotation/rotate
```
This generates a new active CA, rebuilds the bundle, and reloads without restart.

**Helper script (dev/on‑prem)**
```
scripts/rotate--ca.sh
```
Environment overrides:
```
CERTS_DIR=/opt/hardwareops/certs
BASE_URL=https://hardwareops.internal
AUTH_TOKEN=<admin-jwt>
CA_BUNDLE_PATH=/opt/hardwareops/certs/ca-bundle.crt
ACTIVE_CA_CERT_PATH=/opt/hardwareops/certs/ca-active.crt
ACTIVE_CA_KEY_PATH=/opt/hardwareops/certs/ca-active.key
```

### 4) Verify rotation status (UI + API)
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/api/v1/cert-rotation
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
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/api/v1/license
```
Device count should be unchanged before/after rotation.

### 7) Optional: rotate the server TLS cert
Once **all devices** are re‑enrolled:
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/issue-server-cert.sh
```
Point the gateway to the new server cert/key, and update client trust stores as needed.

### 8) Cleanup (remove old CA)
After all devices are active on the new CA and server TLS is rotated:
```
cat /opt/hardwareops/certs/ca-active.crt > /opt/hardwareops/certs/ca-bundle.crt
```
