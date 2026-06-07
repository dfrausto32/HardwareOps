# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Collaboration Style

At the end of every response, recommend the next task. One sentence: what it is and why it's the logical next step given what was just done.

---

## What This Project Is

Parcel is a control-plane platform for deploying software and configuration to autonomous devices in unreliable, bandwidth-constrained, or offline environments. It has four main components:

- **control-plane** — Go 1.25 API server backed by PostgreSQL + MinIO
- **global-plane** — Go 1.25 federation coordinator (hub above regional control planes)
- **agent** — Go 1.22 device agent (minimal deps, systemd-friendly)
- **ui** — Multi-variant React 18 + Vite 5 web console (`ui-generic/`, `ui-healthcare/`, `ui-shared/`)

---

## Local Development (WSL)

### 1. Start dependencies (Postgres + MinIO)
```bash
cp deploy/compose/.env.example deploy/compose/.env
make dev-up
```

### 2. Create a dev CA
```bash
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ./dev-ca.key -out ./dev-ca.crt \
  -days 365 -subj "/CN=Parcel Dev CA"
```

### 3. Run the control-plane
```bash
export DATABASE_URL=postgres://parcel:parcel@localhost:5432/parcel?sslmode=disable
export CA_CERT_PATH=./dev-ca.crt
export CA_KEY_PATH=./dev-ca.key
export AUTO_MIGRATE=1
export DISABLE_HTTP2=1
export CORS_ALLOWED_ORIGINS=http://localhost:5173
ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

`run-control-plane.sh` starts with maintenance mode **enabled** by default. Disable it from the UI or set `MAINTENANCE_MODE=0`.

If certs are stale: `FORCE_DEV_CERTS=1 ENABLE_TLS=1 ./scripts/run-control-plane.sh`

Verify: `curl --cacert ./dev-ca.crt https://localhost:8080/healthz`

### 4. Run the UI
```bash
cd ui-generic && npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
# Opens at http://localhost:5173
```

### 5. Run a demo agent (optional)
```bash
./scripts/run-demo-agent.sh
# Multiple agents: DEMO_COUNT=3 ./scripts/run-demo-agent.sh
```

### 6. Run the global-plane (optional — for E-series federation features)
```bash
cp deploy/global-plane.env.example global-plane.env
# Edit global-plane.env: set GLOBAL_TOKEN_ENCRYPTION_KEY (openssl rand -base64 32)
# Ensure parcel_global database exists: createdb parcel_global
export $(cat global-plane.env | grep -v '^#' | xargs)
go run ./control-plane/cmd/global-plane
# Listens on :8090 by default
```

Verify: `curl http://localhost:8090/healthz`

---

## Testing

```bash
# All Go tests — control-plane
cd control-plane && go test ./...

# All Go tests — agent
cd agent && go test ./...

# UI RBAC unit tests
cd ui-generic && npm run test:rbac

# End-to-end artifact flow (requires running stack)
./scripts/artifact-e2e.sh

# API smoke test
./scripts/curl-quickstart.sh

# Workload identity flow
./scripts/test-workload-identity.sh

# Artifact trust/signing validation
./scripts/test-artifact-trust.sh

# CI/CD feedback loop (webhooks, deploy triggers, deployment status, scoped service tokens)
./scripts/test-ci-feedback-loop.sh

# Pending enrollment flow
./scripts/test-pending-enrollment.sh
```

### Running tests against the AWS instance

All scripts default to `https://localhost:8080`. To run against the live AWS instance:

```bash
export BASE_URL=https://<your-aws-instance-hostname>
export AUTH_EMAIL=admin@example.com
export AUTH_PASSWORD=<admin-password>
export CA_CERT_PATH=/path/to/ca.crt   # or INSECURE=1 to skip TLS verification
./scripts/test-ci-feedback-loop.sh
```

All scripts read `BASE_URL`, `AUTH_EMAIL`, `AUTH_PASSWORD`, and `CA_CERT_PATH` / `INSECURE` from the environment.

---

## Building

```bash
# On-prem installer bundle
./scripts/build-installers.sh

# Healthcare UI variant
UI_VARIANT=healthcare ./scripts/build-installers.sh stack-bundle

# Upgrade packages
./scripts/build-upgrade-package.sh

# Package an artifact (tar.gz with manifest.json)
python3 scripts/artifact-pack.py
```

---

## Architecture

### Control Plane (`control-plane/`)

Go module `github.com/parcel/control-plane`. Key packages:

- `cmd/control-plane/` — entry point
- `httpapi/` — chi v5 router, all HTTP handlers and middleware (JWT auth, rate limiting, mTLS)
- `store/` — data access layer (PostgreSQL via pgx/v5; also an in-memory implementation for tests)
- `auth/` — JWT, local auth, OIDC SSO, service tokens, TOTP MFA, password recovery
- `artifacts/` + `artifactingest/` — artifact management and CI ingest adapters
- `artifacttrust/` — Ed25519 signing and verification with a trusted key registry
- `certs/` — TLS/mTLS certificate issuance and rotation
- `events/` — WebSocket event streaming (nhooyr.io/websocket)
- `migrate/` — sequential SQL migrations (36 files in `migrations/`)
- `objectstore/` — MinIO S3 wrapper
- `license/` — signed license token enforcement with device cap
- `lifecycle/` — artifact deprecation, restore, prune
- `releaseautoupdate/` — auto-update policies

Authentication layers: JWT (local), service tokens (CI), mTLS client certs (devices), OIDC federation (external IdPs), TOTP MFA.

### Global Plane (`control-plane/cmd/global-plane/`, `control-plane/internal/globalplane/`)

Separate binary (`cmd/global-plane`) that runs as a federation coordinator above one or more regional control planes. Agents are unchanged — they always talk to their regional plane.

Key packages under `internal/globalplane/`:
- `store.go` — `Store` interface and all global types (`RegionalPlane`, `GlobalGroup`, `GlobalEnrollmentProfile`, `GlobalPendingEnrollment`, `PolicySyncStatus`, etc.)
- `store_postgres*.go` — PostgreSQL implementations (one file per domain)
- `httpapi/` — chi router + handlers for planes, groups, desired state, enrollment profiles, pending enrollments, artifact federation
- `sync/` — background goroutines:
  - `planeWorker` — polls each regional plane (devices, artifacts, health) on its configured interval
  - `replicationReconciler` — confirms artifact blobs arrived in regional planes
  - `policyReconciler` — re-pushes stale/missed desired-state policies with exponential backoff
  - `pendingEnrollmentReconciler` — polls pending enrollment queues from all regional planes
  - `client.go` — HTTP client wrapping all regional plane API calls

Global migrations are in `control-plane/migrations/global/` (9 files, separate from regional migrations).

The global plane uses its own database (`parcel_global` in dev) and its own `Store` interface — it does **not** import `internal/store` (the regional store).

### Agent (`agent/`)

Go module at `agent/`. Key packages:

- `cmd/agent/` — entry point
- `client/` — control-plane API client
- `bootstrap/` — enrollment (CSR + token exchange)
- `plan/` + `planexec/` — artifact apply plan with preapply scripts, file rendering, health probes, and rollback
- `state/` — local device state (JSON file)
- `artifacts/` — tar.gz extraction and atomic symlink switching

### UI (`ui-generic/`, `ui-healthcare/`, `ui-shared/`)

Multi-variant React 18 SPA (Vite 5). **Shared source** lives in `ui-shared/src/`:
- `AppShell.jsx` — the main application shell, parameterized via `@variant` alias
- `api.ts`, `rbac.js` — API client and role/permission logic
- `components/modals/` — 7 reusable modal components
- `features/` — all feature page components (devices, groups, security, settings, global)
- `index.css` — CSS custom properties for theming

**Per-variant apps** (`ui-generic/`, `ui-healthcare/`):
- Each has `vite.config.js` with `@shared → ../ui-shared/src` and `@variant → ./src/variant.js` aliases
- `src/variant.js` controls: `brandName`, `allowedViews` (which nav items), `extraViews` (customer-specific pages), `themeOverrides`, `featureFlags`
- HC-specific pages in `ui-healthcare/src/features/healthcare/`
- Generic runs on port 5173; healthcare on 5174

Build a specific variant: `cd ui-generic && npm run build` or `cd ui-healthcare && npm run build`.

### Database

**Regional migrations**: 36 sequential SQL files in `control-plane/migrations/`. `AUTO_MIGRATE=1` runs them on startup.

**Global migrations**: 9 SQL files in `control-plane/migrations/global/`. Applied on global-plane startup.

### Service Token Scopes

| Scope | Purpose |
|---|---|
| `artifact.read` | Read artifact metadata and download |
| `artifact.publish` | Upload and publish artifacts (CI ingest) |
| `device.read` | Read device list and status |
| `deployment.trigger` | Trigger immediate device re-checkin |
| `webhook.manage` | Create and manage outbound webhooks |
| `federation.push` | Push telemetry from regional → global; also allows global to poll pending enrollments and proxy approve/deny |
| `federation.manage` | Administrative operations on the global-plane (register/manage regional planes) |

### Artifact Format

Artifacts are `tar.gz` bundles containing `manifest.json` + optional `plan.yaml` + files. Types: `app_bundle`, `config_bundle`, `data_bundle` (agent-applied), `firmware`, `container_image` (registered only). Stored in MinIO; signed with Ed25519.

### Deployment

- **Local dev**: Docker Compose (Postgres + MinIO) — `make dev-up`
- **On-prem**: installer bundle, systemd services, Caddy proxy (`deploy/compose/docker-compose.onprem.yml`)
- **AWS**: Terraform in `deploy/aws/terraform/` — separate `customer_stack` module per customer, environments in `envs/dev|staging|prod`; `UI_VARIANT` build arg selects which UI to bundle

---

## Test Patterns

All handler tests use the **in-memory store** (`internal/store/memory`) and `net/http/httptest`. No database required.

### Regional control-plane handler test

```go
package handlers

import (
    "bytes"
    "encoding/json"
    "log"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/parcel/control-plane/internal/store/memory"
)

func TestMyHandler(t *testing.T) {
    logger := log.New(&bytes.Buffer{}, "", 0)
    mem := memory.New()

    // Seed state if needed.
    // _ = mem.UpsertDevice(store.Device{...})

    req := httptest.NewRequest(http.MethodPost, "/api/v1/foo", bytes.NewReader(body))
    w := httptest.NewRecorder()

    MyHandler(logger, mem, false).ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
    }
}
```

To inject chi URL parameters (e.g., `{deviceId}`):
```go
req = withURLParam(req, "deviceId", "some-id")  // defined in testutil_test.go
```

### Global-plane handler test

Global-plane handlers use **narrow local interfaces** (e.g., `groupStore`, `planesStore`) rather than the full `globalplane.Store`. Tests define a fake struct implementing only the interface required by the handler under test. See `internal/globalplane/httpapi/handlers/testutil_test.go` for the reusable fakes.

### Sync / reconciler test

Use `httptest.NewServer` as a fake regional plane, and a `stubStore` implementing only the methods the reconciler calls (rest panic). See `internal/globalplane/sync/reconciler_test.go`.

---

## Dev Workflows

### Adding a regional migration

1. Find the last file: `ls control-plane/migrations/ | sort | tail -3`
2. Create the next: `control-plane/migrations/0037_my_feature.sql`
3. Write idempotent SQL (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`)
4. `AUTO_MIGRATE=1` picks it up automatically on next server start

### Adding a global-plane migration

Same pattern but in `control-plane/migrations/global/`:
1. `ls control-plane/migrations/global/ | sort | tail -3`
2. Create: `control-plane/migrations/global/0010_my_feature.sql`

### Adding a regional API endpoint

1. Write the handler in `control-plane/internal/httpapi/handlers/my_feature.go`
   - Handler functions take `(logger, store, trustProxy)` and return `http.HandlerFunc`
   - Use a narrow interface: `type myStore interface { ... }` — only methods you need
2. Wire it in `control-plane/internal/httpapi/router.go`
   - Use `r.With(operator).Post("/my-resource", handlers.MyHandler(logger, deps.Store, deps.TrustProxy))`
3. Write tests in `control-plane/internal/httpapi/handlers/my_feature_test.go`
   - Use `memory.New()` and `httptest`
   - Add to `testutil_test.go` if new shared helpers are needed

### Adding a global-plane API endpoint

1. Add types to `internal/globalplane/store.go` (Store interface + struct types)
2. Implement in a new `internal/globalplane/store_postgres_myfeature.go`
3. Write the handler in `internal/globalplane/httpapi/handlers/my_feature.go`
   - Use a narrow local `myStore` interface
4. Wire in `internal/globalplane/httpapi/router.go`
5. Write tests using the fake store pattern in `handlers/testutil_test.go`

### Adding a UI feature (shared)

1. Add the page component under `ui-shared/src/features/my_feature/`
2. Import it in `AppShell.jsx` or add it to `extraViews` in a specific `variant.js`
3. Add API calls through `api.ts`
4. Gate with `permissions` from `rbac.js` if needed

---

## Key Docs

- `docs/local-dev-wsl.md` — canonical local dev guide
- `docs/icd.md` — full API contract (Integration Control Document)
- `docs/deploy.md` — end-to-end deployment guide
- `docs/operations.md` — day-2 operations runbook
- `docs/development/roadmap.md` — authoritative feature status by phase
- `docs/global-plane.md` — global-plane operator runbook
