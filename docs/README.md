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
- `../scripts/aws-demo-image.sh` — Build/push AWS demo-agent image to ECR.
- `../scripts/aws-demo-seed.sh` — Seed signed demo artifacts into an AWS customer stack (manual switching in UI).
- `../scripts/test-artifact-ingest.sh` — Automated local push/pull ingest smoke test.
- `../scripts/setup-artifactory-demo.sh` — Launch local Artifactory OSS container and generate/upload pull-test artifacts.
- `../scripts/test-artifactory-adapter.sh` — End-to-end Artifactory pull adapter test against control-plane.

## UI
- `../ui/README.md` — UI dev workflow and environment settings.

## Development (Roadmap)
- `development/roadmap.md` — Phased roadmap for on‑prem, AWS, RBAC, and artifact ingest.
- `development/deployment-options.md` — On‑prem vs AWS deployment shapes and recommendations.
- `development/aws-cloud-setup-plan.md` — Phased AWS rollout plan (quick demo path + managed customer cloud path).
- `development/aws-customer-deployment-runbook.md` — Step-by-step per-customer AWS deployment + device cert onboarding flow.
- `development/artifact-ingest.md` — Manual, CI push, and repo pull ingest modes.
- `development/push-vs-pull-workflows.md` — Decision guide for CI push vs control-plane pull workflows.
- `development/artifact-ingest-test-plan.md` — End-to-end local validation plan for push + pull ingest.
- `development/auth-secrets-v1.md` — V1 local users + JWT + secrets posture.
- `development/artifact-signing.md` — Current Ed25519 flow and Cosign/Sigstore migration path.
- `development/metrics-health.md` — Prometheus metrics + health summary API + UI panel.
- `development/upgrade-strategy.md` — Safe upgrade/rollback process and compatibility rules.
- `development/license.md` — Signed on‑prem license format + device cap enforcement.
- `development/device-identity-hardening.md` — Hardware identity anti-clone policy (`audit`/`enforce`) for enroll/check-in.
- `development/security-hardening.md` — Security hardening findings + execution order for production controls.
