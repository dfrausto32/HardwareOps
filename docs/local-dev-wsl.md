# Local Development on WSL (Windows Host)

This is the **clean, current** dev flow for running the control‑plane, UI, and agent locally on WSL2.

## Prereqs
- Windows 10/11 with **WSL2** + Ubuntu 22.04
- **Docker Desktop** with WSL integration enabled
- **Go 1.22+**
- **Node 18+** (for UI)
- `python3` (artifact scripts)

## 1) Start dependencies (Postgres + MinIO)
From the repo root:
```
cp deploy/compose/.env.example deploy/compose/.env
make dev-up
```

## 2) Create a dev CA
```
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ./dev-ca.key -out ./dev-ca.crt \
  -days 365 -subj "/CN=Parcel Dev CA"
```

## 3) Run the control‑plane (TLS + mTLS)
```
export DATABASE_URL=postgres://parcel:parcel@localhost:5432/parcel?sslmode=disable
export CA_CERT_PATH=./dev-ca.crt
export CA_KEY_PATH=./dev-ca.key
export AUTO_MIGRATE=1
export DISABLE_HTTP2=1
export CORS_ALLOWED_ORIGINS=http://localhost:5173

ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

You can also load envs from a file:
```
CONTROL_PLANE_ENV_FILE=./control-plane.env ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

Verify:
```
curl --cacert ./dev-ca.crt https://localhost:8080/healthz
```

Note on trust paths:
- `VITE_SIMULATE_PROD=1` means browser talks directly to `https://localhost:8080` and must trust `dev-ca.crt`.
- `VITE_API_PROXY=1` (dev proxy mode) avoids installing CA on the browser machine for local testing.

Note: `run-control-plane.sh` starts with maintenance mode **enabled** by default.
Disable maintenance from the UI using an admin session (or with auth disabled in dev).
You can also start without maintenance:
```
MAINTENANCE_MODE=0 ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

If you see `authority and subject key identifier mismatch`, regenerate dev certs:
```
FORCE_DEV_CERTS=1 ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

## 4) Run the UI (Vite)
```
cd ui
npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
```

Open:
```
http://localhost:5173/
```

### Trust the dev CA in Windows (required for HTTPS)
Copy `dev-ca.crt` to Windows and install it into **Trusted Root Certification Authorities**.
This removes browser TLS warnings and allows `wss://` connections.
Restart the browser after installing the CA (close all windows), or the trust change may not take effect.

## 5) Run a demo agent (optional)
```
./scripts/run-demo-agent.sh
```

Note:
- Default mode is legacy direct enrollment.
- To test the new first-contact approval flow with the same launcher:
```
AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me ENROLLMENT_MODE=pending ./scripts/run-demo-agent.sh
```
- To auto-approve for API-only testing:
```
AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me ENROLLMENT_MODE=pending PENDING_ENROLL_AUTO_APPROVE=1 ./scripts/run-demo-agent.sh
```
- `./scripts/test-pending-enrollment.sh` is still available as a lower-level helper for just the enrollment-profile/request/claim flow.
- To smoke-test first desired-state resolution from enrollment profile labels:
```
AUTH_EMAIL=admin@example.com \
AUTH_PASSWORD=change-me \
PROFILE_DEFAULT_LABELS_JSON='{"site":"factory-a","role":"kiosk"}' \
GROUP_DESIRED_VERSION=v9 \
CHECKIN_AFTER_CLAIM=1 \
./scripts/test-pending-enrollment.sh
```
- Runbook: `docs/development/agent-first-contact-onboarding.md`

## Troubleshooting
### UI shows NetworkError
- Make sure the control‑plane is running on **https://localhost:8080**.
- Ensure Windows trusts `dev-ca.crt`.
- Confirm CORS allowlist includes `http://localhost:5173`.

### WebSocket errors
- Ensure `DISABLE_HTTP2=1` for the control‑plane.
- Confirm browser trusts the dev CA.

### `permission denied` on device certs
```
sudo chown -R parcel:parcel /etc/parcel/agent/certs
sudo systemctl restart parcel-agent
```

### UI fails with `Unexpected token '??='`
This means Node.js is too old. Vite requires Node **18+**.
Check your version:
```
node -v
```
Upgrade Node (recommended with nvm):
```
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc
nvm install 18
nvm use 18
```
Then retry:
```
npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
```

### UI fails with `ERR_SSL_KEY_USAGE_INCOMPATIBLE`
The dev server cert is missing correct KeyUsage. Regenerate dev certs:
```
FORCE_DEV_CERTS=1 ENABLE_TLS=1 ./scripts/run-control-plane.sh
```
Then restart the browser (it can cache TLS sessions).

### Server logs `tls: unknown certificate`
This means the client (browser) does **not trust** the dev CA.

Windows browser:
1) Copy `dev-ca.crt` to Windows (e.g., `cp dev-ca.crt /mnt/c/Users/<you>/Desktop/`).
2) Install into **Trusted Root Certification Authorities**.
3) Restart the browser.

Firefox on Linux (inside WSL GUI):
1) Ensure system trust is used:
   - Open `about:config`
   - Set `security.enterprise_roots.enabled = true`
2) Or import `dev-ca.crt` into Firefox certs.

Verify from WSL:
```
curl --cacert ./dev-ca.crt https://localhost:8080/healthz
```

---

## Demo: Pre-Apply Hook Walkthrough

This walkthrough shows how to demo the pre-apply hook live with the UI and the demo agent.
Run this after steps 1–4 above (dependencies + control-plane + UI + demo agent already running).

### Build and upload demo artifacts

Option A (demo v1 → v2 page flip):
```bash
./scripts/demo-artifacts.sh
```

Option B (all artifact types with pre-apply):
```bash
./scripts/artifact-types.sh
```

### Apply from the UI

1. In the UI, select the demo device.
2. Choose an artifact and click **Apply**.
3. Watch the demo page update at `http://localhost:8081/index.html`.

### What you should see

- The demo page shows **"Pre‑apply"** content pulled from `files/preapply.txt`.
- The UI dashboard displays a **Pre‑apply badge** with the last status.

### Expected output per artifact type

| Artifact type | First apply | Update to newest |
|---|---|---|
| `app_bundle` | Type: app_bundle, Version: `0.1.0-app_bundle` | Version: `0.2.0-app_bundle` |
| `config_bundle` | Type: config_bundle, Version: `0.1.0-config_bundle` | Version: `0.2.0-config_bundle` |
| `data_bundle` | Type: data_bundle, Version: `0.1.0-data_bundle` | Version: `0.2.0-data_bundle` |
| `firmware` | Type: firmware, Version: `0.1.0-firmware` | Version: `0.2.0-firmware` |
| `container_image` | Type: container_image, Version: `0.1.0-container_image` | Version: `0.2.0-container_image` |

The **Pre‑apply** line will show something like:
```
preapply ok
type=<type>
version=<version>
time=<timestamp>
```

> Note: `firmware` and `container_image` only apply in demo mode because
> `run-demo-agent.sh` sets `ALLOW_UNSUPPORTED_APPLY=1` by default.
> Set to `0` to enforce realistic behavior.

### Multiple demo agents

```bash
DEMO_COUNT=3 DEMO_HTTP_PORT=8081 ./scripts/run-demo-agent.sh
```

Ports: 8081, 8082, 8083
