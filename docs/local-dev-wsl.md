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
  -days 365 -subj "/CN=HardwareOps Dev CA"
```

## 3) Run the control‑plane (TLS + mTLS)
```
export DATABASE_URL=postgres://hardwareops:hardwareops@localhost:5432/hardwareops?sslmode=disable
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

Note: `run-control-plane.sh` starts with maintenance mode **enabled** by default.
To disable from the UI, set `VITE_MAINTENANCE_TOKEN=dev-token` (already in `ui/.env`) and click **Disable maintenance**.
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
sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
sudo systemctl restart hardwareops-agent
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
