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

### B) Auto‑Migrate on Startup (dev / small env)
**Use when:** development, staging, or single‑node environments.

- Set `AUTO_MIGRATE=1` (default for local scripts)
- Control‑plane applies migrations automatically on boot.

**Risk:** a bad migration affects prod immediately; no rollback without restore.

## Rollback Strategy
Because schema migrations are forward‑only, rollback requires **restoring backups**:
1) Stop control‑plane + UI.
2) Restore Postgres and object store from snapshot.
3) Re‑deploy previous control‑plane + UI versions.
4) Resume traffic.

## Agent Upgrade Strategy
- Prefer **rolling upgrades** (batch by batch).
- Stagger with **check‑in jitter** to avoid thundering herd.
- Maintain compatibility with the current control‑plane APIs.

## Operational Safeguards
- **Feature flags** for new behavior where possible.
- **Metrics dashboards** before major releases.
- **Smoke tests** after upgrade:
  - `/healthz` (control‑plane)
  - `GET /api/v1/devices`
  - Apply a small test artifact to a canary device

## Maintenance + Auto‑Apply Requirements
- Control‑plane env must include:
  - `MAINTENANCE_MODE=1` (start in maintenance)
  - `MAINTENANCE_TOKEN=...` (UI toggle + upgrade apply)
  - `UPGRADE_APPLY_CMD=/path/to/scripts/apply-upgrade.sh`
- UI build args must include:
  - `VITE_MAINTENANCE_TOKEN` matching `MAINTENANCE_TOKEN`
- If the control‑plane runs **in Docker**, the upgrade runner needs:
  - `/var/run/docker.sock` mounted into the container
  - the stack directory mounted (for compose + scripts)
  - `UPGRADE_WORK_DIR` + `UPGRADE_APPLY_CMD` pointing at the mounted script
- If running in containers, the upgrade runner needs access to Docker
  (e.g., mount `/var/run/docker.sock` into the control‑plane container).

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
