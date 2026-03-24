# HardwareOps

HardwareOps is a control-plane platform for deploying software and configuration to autonomous devices operating in unreliable, bandwidth-constrained, or offline environments. It handles device enrollment (mTLS), artifact packaging and delivery, fleet desired-state management, and audit/compliance — all from a single API and web console.

---

## How the repo is organized

```
HardwareOps/
├── control-plane/     Go API server (PostgreSQL + MinIO)
├── agent/             Go device agent (runs on each managed device)
├── ui/                React 18 web console
├── deploy/            Docker Compose, on-prem installer, AWS Terraform
├── migrations/        Sequential SQL migrations (run at startup via AUTO_MIGRATE=1)
├── scripts/           Dev helpers, smoke tests, build tools
└── docs/              All documentation (start with docs/README.md for the map)
```

The three components talk to each other like this:

```
[Device running agent] <──mTLS──> [control-plane API] <──> [PostgreSQL + MinIO]
                                          ^
                               [UI / CI / operator curl]
```

- The **control-plane** is what you'll spend most time in when developing backend features.
- The **agent** runs on managed devices; it enrolls, checks in, and applies desired state.
- The **UI** is a read/write console for operators — no separate backend.

---

## Prerequisites

- **Go 1.22+** — for the control-plane and agent
- **Node.js 20+** — for the UI
- **Docker + Docker Compose** — for local Postgres + MinIO
- **WSL2** (on Windows) — the dev flow is documented for WSL2; native Linux works identically

---

## Step 1 — Start the backing services

The control-plane needs Postgres and MinIO. Docker Compose handles both:

```bash
cp deploy/compose/.env.example deploy/compose/.env
make dev-up
```

This brings up:
- Postgres on `localhost:5432` (user/pass/db: `hardwareops`)
- MinIO on `localhost:9000` (access: `minio` / `minio123`, console: `localhost:9001`)

Verify they're running:
```bash
docker compose -f deploy/compose/docker-compose.yml ps
```

---

## Step 2 — Create a dev CA

The control-plane signs device certificates with an internal CA. For local dev, generate a self-signed one:

```bash
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ./dev-ca.key -out ./dev-ca.crt \
  -days 365 -subj "/CN=HardwareOps Dev CA"
```

This CA cert (`dev-ca.crt`) is what agents and curl commands use to trust the server. You only need to do this once.

---

## Step 3 — Run the control-plane

```bash
export DATABASE_URL=postgres://hardwareops:hardwareops@localhost:5432/hardwareops?sslmode=disable
export CA_CERT_PATH=./dev-ca.crt
export CA_KEY_PATH=./dev-ca.key
export AUTO_MIGRATE=1
export DISABLE_HTTP2=1
export CORS_ALLOWED_ORIGINS=http://localhost:5173

ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

What this does on first run:
1. Runs all SQL migrations in `control-plane/migrations/` against your local Postgres.
2. Starts the HTTP server on `https://localhost:8080`.
3. Maintenance mode is **on by default** — disable it from the UI or set `MAINTENANCE_MODE=0`.

Verify it's up:
```bash
curl --cacert ./dev-ca.crt https://localhost:8080/healthz
# → ok
```

If certs are stale from a previous run: `FORCE_DEV_CERTS=1 ENABLE_TLS=1 ./scripts/run-control-plane.sh`

---

## Step 4 — Run the UI

```bash
cd ui && npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
```

Open `http://localhost:5173`. The default admin login is:
- **Email:** `admin@example.com`
- **Password:** `change-me`

(Set via `AUTH_BOOTSTRAP_EMAIL` / `AUTH_BOOTSTRAP_PASSWORD` in config.)

The UI talks directly to the control-plane over TLS. `VITE_SIMULATE_PROD=1` disables the dev proxy so the browser uses real HTTPS — you may need to trust `dev-ca.crt` in your OS or browser.

---

## Step 5 — Enroll a device (understand the core flow)

This is the most important concept in the platform: devices prove their identity by exchanging a CSR for a signed certificate, then use that certificate for all future communication (mTLS).

Run the automated quickstart to see the full enrollment flow:

```bash
BASE_URL=https://localhost:8080 CA_CERT_PATH=./dev-ca.crt ./scripts/curl-quickstart.sh
```

What it does step by step:
1. Creates an enrollment token (`POST /api/v1/enrollments`)
2. Generates a device keypair + CSR (`openssl req`)
3. Exchanges the token + CSR for a signed device cert (`POST /api/v1/devices/enroll`)
4. Posts a device check-in using the issued cert over mTLS (`POST /api/v1/devices/checkin`)

After the script runs, the enrolled device should appear in the UI under **Devices**.

Cleanup the test device:
```bash
CLEANUP=1 BASE_URL=https://localhost:8080 CA_CERT_PATH=./dev-ca.crt ./scripts/curl-quickstart.sh
```

---

## Step 6 — Deploy an artifact to a device

Artifacts are `tar.gz` bundles containing a `manifest.json` plus optional `plan.yaml` and files. The agent downloads, verifies, extracts, and atomically switches to the new version.

**Package an artifact:**
```bash
python3 scripts/artifact-pack.py \
  --name my-app \
  --version 1.0.0 \
  --type app_bundle \
  --input-dir ./build \
  --out /tmp/my-app-1.0.0.tar.gz
```

**Upload and register it:**
```bash
curl --cacert ./dev-ca.crt -s -X POST https://localhost:8080/api/v1/artifacts/upload \
  -H "Authorization: Bearer <your-jwt>" \
  -F "name=my-app" \
  -F "version=1.0.0" \
  -F "type=app_bundle" \
  -F "file=@/tmp/my-app-1.0.0.tar.gz"
```

**Set desired state for a device:**
```bash
curl --cacert ./dev-ca.crt -s -X PUT https://localhost:8080/api/v1/desired-state/devices/<deviceId> \
  -H "Authorization: Bearer <your-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"artifactId":"<artifactId>","desiredVersion":"1.0.0"}'
```

On the next check-in, the agent downloads the artifact, runs any `plan.yaml` steps, and atomically switches the active version. Apply results (success/error) are posted back to `POST /api/v1/devices/{deviceId}/apply-result`.

**Run the full end-to-end flow as a smoke test:**
```bash
./scripts/artifact-e2e.sh
# With a demo artifact that includes a preapply script:
GENERATE_ARTIFACT=1 ./scripts/artifact-e2e.sh
```

---

## Step 7 — Run a demo agent

To see the control-plane + agent working together without a real device:

```bash
./scripts/run-demo-agent.sh
```

This starts an agent container that auto-enrolls, runs a small web server on `http://localhost:8081`, and applies whatever desired state you set. Then push an artifact update:

```bash
./scripts/demo-artifacts.sh
# Builds v1 and v2, sets desired state to v2
curl -s http://localhost:8081/index.html
# → page content reflects the new version
```

Run multiple agents at once:
```bash
DEMO_COUNT=3 ./scripts/run-demo-agent.sh
```

---

## Codebase orientation

### Control-plane packages

The entry point is `control-plane/cmd/control-plane/main.go`. It wires together:

| Package | What it does |
|---|---|
| `internal/httpapi/` | Chi v5 router + all HTTP handlers; RBAC middleware (viewer / operator / admin) |
| `internal/store/` | Data access layer — `store.Store` interface with Postgres and in-memory implementations |
| `internal/auth/` | JWT (local), OIDC, LDAP, service tokens, mTLS device identity |
| `internal/artifactingest/` | Pull adapters for S3/GCS, credential manager, pull safety controls |
| `internal/artifacttrust/` | Ed25519 signing/verification, trusted key registry, provenance policy |
| `internal/vulnscan/` | Grype/Trivy artifact scanning, Nessus device scanning |
| `internal/certs/` | CA management, device cert issuance, rotation |
| `internal/events/` | WebSocket event hub (real-time device activity stream) |
| `internal/objectstore/` | MinIO S3 wrapper |
| `internal/config/` | All env var loading — start here to understand what's configurable |

**Where to start reading:**
- `internal/config/config.go` — all configuration in one place
- `internal/store/store.go` — the Store interface defines every data operation
- `internal/httpapi/router.go` — all routes in one place with their auth requirements
- `internal/httpapi/handlers/` — one file per feature area (artifacts, devices, auth, etc.)

### Agent packages

The agent is minimal by design — it runs on embedded/constrained hardware.

| Package | What it does |
|---|---|
| `bootstrap/` | First-contact enrollment: sends CSR, gets back signed cert + device ID |
| `client/` | Control-plane API client (check-in, fetch desired state, post apply results) |
| `plan/` + `planexec/` | Parses `plan.yaml` and executes steps (scripts, file renders, health probes, rollback) |
| `artifacts/` | tar.gz extraction and atomic symlink switching |
| `state/` | Local device state persisted to a JSON file |

### Database migrations

Migrations live in `control-plane/migrations/` and run in numeric order. `AUTO_MIGRATE=1` runs them at startup. To understand the data model, reading the migration files in order is the fastest path — they're plain SQL and heavily commented.

### UI

The UI is a single React 18 SPA. All API calls go through `ui/src/api.ts`. RBAC enforcement is in `ui/src/rbac.js`. Features are organized under `ui/src/features/`.

---

## Key concepts

**Enrollment** — Devices prove identity by submitting a CSR (certificate signing request). The control-plane signs it with the internal CA and returns a device certificate. All subsequent device communication uses that cert for mTLS.

**Artifacts** — Versioned software bundles (`tar.gz` with `manifest.json`). Stored in MinIO. Types: `app_bundle`, `config_bundle`, `data_bundle` (agent-applied), `firmware`, `container_image` (register only). Agents only receive short-lived presigned download URLs — never MinIO credentials.

**Desired State** — Operators set a target artifact version per device or group. Agents pull desired state on each check-in and apply it. Group selectors (label matching) let you roll out to fleets; device-level overrides take precedence.

**Auth layers** — The platform has four: JWT (local/LDAP/OIDC users), service tokens (CI systems), mTLS client certs (devices), and bootstrap tokens (first enrollment). All are independently optional.

---

## Running tests

```bash
# UI RBAC unit tests (the only automated tests in the project)
cd ui && npm run test:rbac

# API smoke test (requires running stack)
./scripts/curl-quickstart.sh

# End-to-end artifact flow (requires running stack)
./scripts/artifact-e2e.sh

# Workload identity flow
./scripts/test-workload-identity.sh

# Artifact trust/signing
./scripts/test-artifact-trust.sh

# Go unit tests
cd control-plane && go test ./...
```

---

## Going deeper

| Topic | Where to read |
|---|---|
| Full API contract | `docs/icd.md` |
| Deployment (on-prem + AWS) | `docs/deploy.md` |
| Day-2 operations (backup, upgrade, cert rotation) | `docs/operations.md` |
| Local dev guide (detailed WSL2 flow) | `docs/local-dev-wsl.md` |
| Artifact signing and provenance | `docs/artifact-provenance.md` |
| Cloud pull adapters (S3/GCS) | `docs/cloud-pull-adapters.md` |
| LDAP/AD authentication | `docs/ldap-auth.md` |
| Vulnerability scanning | `docs/vulnerability-scanning.md` |
| Feature roadmap and status | `docs/development/roadmap.md` |
| Phase C feature internals | `docs/development/phase-c-internals.md` |
| Full documentation map | `docs/README.md` |
