# Upgrade Strategy (Control‑Plane, DB, UI, Agent)

This document defines a **safe, repeatable** upgrade process for HardwareOps in real environments. It is designed for on‑prem first, with a path to cloud later.

## Goals
- **Zero data loss** during upgrades.
- **Predictable downtime** (or rolling upgrades where possible).
- **Clear rollback** path if a release fails.
- **Version compatibility** between control‑plane, DB schema, UI, and agents.

## Versioning & Compatibility Contract
- **Control‑plane ↔ DB schema**: schema is **forward‑only** (no down migrations). Rollback = restore from backup.
- **Control‑plane ↔ Agent**: agents should be **backward‑compatible** with the last N control‑plane versions.
- **Control‑plane ↔ UI**: UI should tolerate additive API changes; breaking changes require a UI bump.
- **Artifacts**: remain valid across versions; add metadata fields rather than changing existing semantics.

## Components & Upgrade Order
1) **Database** (schema migrations)
2) **Control‑plane** (API + behavior)
3) **UI** (optional if API-compatible)
4) **Agents** (rolling, usually last)

## Pre‑Upgrade Checklist
- ✅ Confirm **backup** of Postgres + object store exists.
- ✅ Confirm **maintenance window** and rollout plan.
- ✅ Validate **disk space** and service health.
- ✅ Review **release notes** (migrations, breaking changes).
- ✅ **Enable maintenance mode** (UI toggle or `MAINTENANCE_MODE=1`) to freeze writes.
- ✅ Run **upgrade preflight** in the UI (checks runner, updates dir, docker socket, bundles, disk space).

## Upgrade Paths

### A) Safe, Manual Upgrade (recommended for production)
**Use when:** on‑prem production or any environment where rollback is required.

1) **Freeze writes**
   - Enable maintenance mode (UI toggle).
   - This pauses check‑ins, enrollments, and apply‑result writes.

2) **Backup**
   - Postgres: snapshot/backup
   - Object store: bucket snapshot (if available)

3) **Stage the update**
   - Place the new stack bundle on the host (or update image tags).
   - If `images/*.tar` is present in the bundle, `apply-upgrade.sh` loads them.
   - Set `PULL_IMAGES=1` to force registry pulls instead.
   - For UI auto‑apply, extract the upgrade package **into the mounted stack dir**
     (the same path as `STACK_DIR`, mounted to `/stack` in the control‑plane container).
   - The UI checks for bundles in `/stack/updates` (`UPGRADE_UPDATES_DIR`).
     Place `hardwareops-upgrade-*.tar.gz` there to mark an update available.
   - If using the auto‑apply flow, the control‑plane will run `scripts/apply-upgrade.sh`.

4) **Apply update**
   - From the UI (requires `MAINTENANCE_TOKEN`):
     - Click **Enable maintenance** → **Apply update**.
     - Apply will **fail fast** if preflight has any blocking errors.
   - Or run manually:
     ```bash
     # upgrade package (images + compose)
     tar -xf hardwareops-upgrade-*.tar.gz
     cd hardwareops-upgrade-*
     STACK_DIR=$PWD ENV_FILE=.env.onprem.example ./scripts/apply-upgrade.sh

     # stack bundle (if already on host)
     COMPOSE_FILE=... ENV_FILE=... ./scripts/apply-upgrade.sh
     ```

5) **Post‑upgrade checks**
   - `/healthz` returns 200
   - UI loads
   - Device list endpoint works

6) **Resume traffic**
   - Maintenance mode is disabled automatically by `apply-upgrade.sh`.
   - If token is not set, disable manually in the UI.

7) **Monitor**
   - Check error rates, apply results, device status transitions.

### Upgrade Preflight (UI)
The Settings page includes a **Preflight** panel that verifies:
- Upgrade runner configured
- Updates directory present
- Upgrade bundle present (if expected)
- Docker socket mounted (for containerized stacks)
- Disk space at workdir/updates dir
Use this to catch missing mounts or packages before applying.

### Staged Apply + Rollback Contract (draft)
**Purpose:** define a deterministic apply path with explicit staging and a clear rollback trigger.

**Scope (v1):**
- Docker runner only (`UPGRADE_RUNNER_MODE=docker`).
- On‑prem stack bundles (`hardwareops-upgrade-*.tar.gz`).
- Rollback covers **gateway + control‑plane images** only (DB rollback requires backup restore).

**Inputs (must be present before apply):**
- Upgrade tarball in `/stack/updates` (mounted into control‑plane container).
- Runner image resolved and available locally.
- Env file available (`/stack/.env.onprem` or `.env.onprem.example`).
- Docker socket mounted into control‑plane container.

**Staging step (idempotent):**
1) Extract tarball to `/stack/updates/current/<bundle>/`.
2) Validate bundle contains:
   - `docker-compose.onprem.bundle.yml`
   - `.env.onprem.example`
   - `images/` (optional) or a registry pull plan
3) Mark staged bundle as “active” (e.g. `updates/current/ACTIVE` file).

**Apply step (runner container):**
- Start dedicated runner container with explicit image + env:
  - `--env-file /stack/.env.onprem`
  - `COMPOSE_FILE=/stack/updates/current/<bundle>/docker-compose.onprem.bundle.yml`
  - `PROJECT_NAME=hardwareops`
- Load images from `images/*.tar` (if present), else pull.
- Run `docker compose up -d`.

**Health gate (blocking):**
- Check **gateway** and **control‑plane** health:
  - `https://<PUBLIC_BASE_URL>/healthz`
  - `https://<PUBLIC_BASE_URL>/` (UI served)
- Optional (if managed locally):
  - Postgres TCP check
  - MinIO `/minio/health/ready`
- Timeout: **120s**, retry every **5s**.
- If health gate fails → rollback.

**Health gate envs (optional):**
- `UPGRADE_HEALTH_TIMEOUT` (seconds, default 120)
- `UPGRADE_HEALTH_INTERVAL` (seconds, default 5)
- `UPGRADE_HEALTH_URLS` (comma‑separated URLs; defaults to `/healthz` + `/`)

**Rollback contract:**
- Capture **previous image tags** for `control-plane` + `gateway` before apply.
- If health gate fails:
  - Re‑apply compose with overrides for previous images.
  - Re‑run health gate to confirm rollback success.
  - Leave **maintenance enabled** if rollback occurred.
 - Staged bundle path is recorded in `updates/current/ACTIVE` for traceability.

**Observability:**
- Runner writes logs to `/var/lib/hardwareops/logs/upgrade-<ts>.log`.
- Upgrade status endpoint reports:
  - `state`, `running`, `exitCode`, `logPath`, `error`.

**Acceptance criteria (for implementation later):**
- Applying a broken bundle triggers rollback to prior images.
- Rollback leaves UI + control‑plane healthy.
- Preflight blocks apply when runner image or env file missing.

### B) Auto‑Migrate on Startup (dev / small env)
**Use when:** development, staging, or single‑node environments.

- Set `AUTO_MIGRATE=1` (default for local scripts)
- Control‑plane applies migrations automatically on boot.

**Risk:** a bad migration affects prod immediately; no rollback without restore.

## Rollback Strategy
**Fast rollback (images only):** `apply-upgrade.sh` will attempt to rollback
control‑plane + gateway images to the previously running versions if the upgrade
fails. This does **not** undo DB migrations.

**Full rollback:** Because schema migrations are forward‑only, full rollback requires
**restoring backups**:
1) Stop control‑plane + UI.
2) Restore Postgres and object store from snapshot.
3) Re‑deploy previous control‑plane + UI versions.
4) Resume traffic.

## Agent Upgrade Strategy
Agents are upgraded **via artifacts**, not via control‑plane stack upgrades.

**Recommended flow (systemd agents):**
1) Build an `app_bundle` artifact that contains the new agent binary and a `preapply.sh`:
   - `preapply.sh` should stop the agent service cleanly.
2) The agent apply step replaces the binary, updates permissions, and restarts the service.
3) Post‑apply health check confirms the agent re‑connected.

**Recommended flow (containerized agents):**
1) Build a `container_image` artifact (docker save tar).
2) Agent apply loads the image, stops the running container, and starts the new one.

**Rollout guidance:**
- Use group desired state with canary labels (small batch first).
- Stagger via agent check‑in jitter to avoid load spikes.
- Keep the agent compatible with the last N control‑plane versions.

## Operational Safeguards
- **Feature flags** for new behavior where possible.
- **Metrics dashboards** before major releases.
- **Smoke tests** after upgrade:
  - `/healthz` (control‑plane)
  - `GET /api/v1/devices`
  - Apply a small test artifact to a canary device

## Maintenance + Auto‑Apply Requirements
- Control‑plane env must include (docker runner only):
  - `MAINTENANCE_MODE=1` (start in maintenance)
  - `MAINTENANCE_TOKEN=...` (UI toggle + upgrade apply)
  - `UPGRADE_APPLY_CMD=/path/to/scripts/apply-upgrade.sh`
  - `UPGRADE_RUNNER_MODE=docker` (runs apply in a separate container)
  - `UPGRADE_RUNNER_IMAGE=...` (image used for the upgrade runner)
- UI build args must include:
  - `VITE_MAINTENANCE_TOKEN` matching `MAINTENANCE_TOKEN`
- If the control‑plane runs **in Docker**, the upgrade runner needs:
  - `/var/run/docker.sock` mounted into the container
  - the stack directory mounted (for compose + scripts)
  - `UPGRADE_WORK_DIR` + `UPGRADE_APPLY_CMD` pointing at the mounted script
- If running in containers, the upgrade runner needs access to Docker
  (e.g., mount `/var/run/docker.sock` into the control‑plane container).
- Docker client API must be **>= 1.44** (Docker 25+). Older clients will fail
  to load staged images; rebuild the control‑plane image if needed.

## Recommended On‑Prem Workflow
1) **Launch normally** (maintenance off).
   - `MAINTENANCE_MODE=0` in `.env.onprem`.
   - The upgrade runner should be configured but idle.
2) **Stage an upgrade**
   - Copy the upgrade tarball into `/stack/updates`.
3) **Enable maintenance in UI** (freeze writes).
4) **Apply upgrade** (UI spawns dedicated runner container).
5) **Verify + exit maintenance** (UI or auto‑disable).

## Future Enhancements
- Advisory DB migration lock to prevent concurrent migrations.
- Blue/green control‑plane deployment with DB compatibility gates.
- Automated migration rollback (if we introduce down migrations).

---

## Quick Reference: Commands
- DB migrate:
  ```bash
  DATABASE_URL=... go run ./cmd/migrate -dir ./migrations
  ```
- Control‑plane health:
  ```bash
  curl --cacert ./dev-ca.crt https://localhost:8080/healthz
  ```
