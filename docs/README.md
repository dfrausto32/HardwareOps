# Documentation Map

This directory contains the guides referenced throughout the repo. Use this index to find the right doc quickly.

## Install + Deploy
- `installer-flow.md` — **Fresh machine install** using installer bundles (stack + agents).
- `onprem-deploy.md` — Full on‑prem deployment walkthrough (repo-based).
- `installers.md` — What each installer bundle contains and how to build them.
- `vm-testing.md` — VM-based testing (VirtualBox two‑VM flow).

## Local Development
- `local-dev-wsl.md` — WSL2 dev setup (control‑plane + UI + agent).

## Networking + TLS
- `dns-coredns.md` — CoreDNS setup for local DNS and VM scenarios.
- `certs.md` — CA + TLS certificate setup and trust instructions.

## Agents + Updates
- `agent-systemd.md` — Agent install via systemd (with enrollment).
- `artifact-apply-roadmap.md` — Artifact apply plan (firmware + container images).
- `preapply-demo.md` — End‑to‑end demo including pre‑apply behavior.

## UI
- `../ui/README.md` — UI dev workflow and environment settings.

## Development (Roadmap)
- `development/roadmap.md` — Phased roadmap for on‑prem, AWS, RBAC, and artifact ingest.
- `development/deployment-options.md` — On‑prem vs AWS deployment shapes and recommendations.
- `development/artifact-ingest.md` — Manual, CI push, and repo pull ingest modes.
- `development/auth-rbac.md` — Auth modes, fixed roles, and break‑glass access.
- `development/auth-secrets-v1.md` — V1 local users + JWT + secrets posture.
- `development/artifact-signing.md` — Current Ed25519 flow and Cosign/Sigstore migration path.
- `development/upgrade-strategy.md` — Safe upgrade/rollback process and compatibility rules.
- `development/license.md` — Signed on‑prem license format + device cap enforcement.
