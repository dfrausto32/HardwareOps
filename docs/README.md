# Documentation Map

This directory contains the guides referenced throughout the repo. Use this index to find the right doc quickly.

## Install + Deploy
- `installer-flow.md` — **Fresh machine install** using installer bundles (stack + agents).
- `installers.md` — What each installer bundle contains and how to build them.
- `vm-testing.md` — VM-based testing (VirtualBox two‑VM flow).
- `backup-restore.md` — Postgres + MinIO backup/restore runbook.

## Local Development
- `local-dev-wsl.md` — WSL2 dev setup (control‑plane + UI + agent).

## Networking + TLS
- `dns-coredns.md` — CoreDNS setup for local DNS and VM scenarios.
- `certs.md` — CA + TLS certificate setup and trust instructions.
  - Includes **CA rotation** explanation, runbook, and hot‑reload steps.

## Agents + Updates
- `agent-systemd.md` — Agent install via systemd (with enrollment).
- `artifact-apply-roadmap.md` — Artifact apply plan (firmware + container images).
- `preapply-demo.md` — End‑to‑end demo including pre‑apply behavior.
- `../scripts/multi-app-artifacts.sh` — Build/upload multiple app bundle artifacts for multi‑component testing.
  - Writes `files/last_applied.txt` + `files/apply.log` on apply so you can verify updates without the demo web UI.
  - Starts a heartbeat loop that appends timestamps to `files/heartbeat.log` every 10s.

## UI
- `../ui/README.md` — UI dev workflow and environment settings.

## Development (Roadmap)
- `development/roadmap.md` — Phased roadmap for on‑prem, AWS, RBAC, and artifact ingest.
- `development/deployment-options.md` — On‑prem vs AWS deployment shapes and recommendations.
- `development/artifact-ingest.md` — Manual, CI push, and repo pull ingest modes.
- `development/auth-secrets-v1.md` — V1 local users + JWT + secrets posture.
- `development/artifact-signing.md` — Current Ed25519 flow and Cosign/Sigstore migration path.
- `development/metrics-health.md` — Prometheus metrics + health summary API + UI panel.
- `development/upgrade-strategy.md` — Safe upgrade/rollback process and compatibility rules.
- `development/license.md` — Signed on‑prem license format + device cap enforcement.
