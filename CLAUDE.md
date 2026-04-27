# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

Parcel is a control-plane platform for deploying software and configuration to autonomous devices in unreliable, bandwidth-constrained, or offline environments. It has three main components:

- **control-plane** — Go 1.25 API server backed by PostgreSQL + MinIO
- **agent** — Go 1.22 device agent (minimal deps, systemd-friendly)
- **ui** — React 18 + Vite 5 web console

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
cd ui && npm install
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
# Opens at http://localhost:5173
```

### 5. Run a demo agent (optional)
```bash
./scripts/run-demo-agent.sh
# Multiple agents: DEMO_COUNT=3 ./scripts/run-demo-agent.sh
```

## Testing

```bash
# UI RBAC unit tests (only automated tests in the project)
cd ui && npm run test:rbac

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
```

Go modules have no dedicated test targets — test with standard `go test ./...` inside `control-plane/` or `agent/`.

### Running tests against the AWS instance

The scripts above default to `https://localhost:8080`. To run against the live AWS instance, set these environment variables before invoking the script:

```bash
export BASE_URL=https://<your-aws-instance-hostname>
export AUTH_EMAIL=admin@example.com
export AUTH_PASSWORD=<admin-password>
# If the AWS instance uses a self-signed or internal CA:
export CA_CERT_PATH=/path/to/ca.crt
# OR to skip TLS verification entirely (not for production use):
export INSECURE=1
```

Then run:

```bash
./scripts/test-ci-feedback-loop.sh
```

All scripts read `BASE_URL`, `AUTH_EMAIL`, `AUTH_PASSWORD`, and `CA_CERT_PATH` / `INSECURE` from the environment and fall back to localhost defaults when unset. The `test-ci-feedback-loop.sh` script also starts a local Python HTTP listener for the webhook delivery test; ensure the AWS instance can reach the machine running the script on `WEBHOOK_PORT` (default `9876`).

## Building

```bash
# On-prem installer bundle
./scripts/build-installers.sh

# Upgrade packages
./scripts/build-upgrade-package.sh

# Package an artifact (tar.gz with manifest.json)
python3 scripts/artifact-pack.py
```

## Architecture

### Control Plane (`control-plane/`)

Go module `github.com/parcel/control-plane`. Key packages:

- `cmd/control-plane/` — entry point
- `httpapi/` — chi v5 router, all HTTP handlers and middleware (JWT auth, rate limiting, mTLS)
- `store/` — data access layer (PostgreSQL via pgx/v5; also an in-memory implementation)
- `auth/` — JWT, local auth, OIDC SSO, service tokens, password recovery
- `artifacts/` + `artifactingest/` — artifact management and CI ingest adapters
- `artifacttrust/` — Ed25519 signing and verification with a trusted key registry
- `certs/` — TLS/mTLS certificate issuance and rotation
- `events/` — WebSocket event streaming (nhooyr.io/websocket)
- `migrate/` — sequential SQL migrations (25 files in `migrations/`)
- `objectstore/` — MinIO S3 wrapper
- `license/` — signed license token enforcement with device cap
- `lifecycle/` — artifact deprecation, restore, prune
- `releaseautoupdate/` — auto-update policies

Authentication layers: JWT (local), service tokens (CI), mTLS client certs (devices), OIDC federation (external IdPs).

### Agent (`agent/`)

Go module at `agent/`. Key packages:

- `cmd/agent/` — entry point
- `client/` — control-plane API client
- `bootstrap/` — enrollment (CSR + token exchange)
- `plan/` + `planexec/` — artifact apply plan with preapply scripts, file rendering, health probes, and rollback
- `state/` — local device state (JSON file)
- `artifacts/` — tar.gz extraction and atomic symlink switching

### UI (`ui/`)

React 18 SPA (Vite). API calls go through `src/api.ts` (TypeScript). RBAC enforcement via `src/rbac.js`. No CSS framework — custom CSS throughout.

### Database

25 sequential SQL migrations in `control-plane/migrations/`. `AUTO_MIGRATE=1` runs them on startup. Tables cover: devices, groups, artifacts, desired state, apply results, audit events, users, service tokens, certs, licenses, enrollment profiles, trusted keys, and more.

### Artifact Format

Artifacts are `tar.gz` bundles containing `manifest.json` + optional `plan.yaml` + files. Types: `app_bundle`, `config_bundle`, `data_bundle` (agent-applied), `firmware`, `container_image` (registered only). Stored in MinIO; signed with Ed25519.

### Deployment

- **Local dev**: Docker Compose (Postgres + MinIO) — `make dev-up`
- **On-prem**: installer bundle, systemd services, Caddy proxy (`deploy/compose/docker-compose.onprem.yml`)
- **AWS**: Terraform in `deploy/aws/terraform/` — separate `customer_stack` module per customer, environments in `envs/dev|staging|prod`

## Key Docs

- `docs/local-dev-wsl.md` — canonical local dev guide
- `docs/icd.md` — full API contract (Integration Control Document)
- `docs/deploy.md` — end-to-end deployment guide
- `docs/operations.md` — day-2 operations runbook
- `docs/development/roadmap.md` — authoritative feature status by phase
