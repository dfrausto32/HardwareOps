# Installers (Multi‑OS Bundles)

Canonical deployment guide: `deploy.md`  
Use this file for bundle build/layout details.

This repo ships a build script that creates **portable installer bundles** for common OS/arch targets.

## Build bundles
From the repo root:
```
./scripts/build-installers.sh
```

Build a **standalone upgrade package** (images + compose + apply script):
```
./scripts/build-upgrade-package.sh
```

Prereqs on the build machine:
- Go 1.22+ (for agent/control‑plane binaries)
- Docker (for stack bundle images)

Skip stack bundle if Docker is unavailable:
```
BUILD_STACK=0 ./scripts/build-installers.sh
```

Outputs are written to:
```
dist/installers/<version>/
```

## What’s inside
### Agent bundles
- `hardwareops-agent` binary
- `scripts/agent-install.sh` (Linux systemd install)
- `scripts/agent-enroll.sh`
- `deploy/systemd/` service file + env example
- `README.txt`

### Control‑plane bundles
- `control-plane` binary
- `migrations/`
- `control-plane.env.example`
- `scripts/setup-control-plane.sh`
- `scripts/bootstrap-ca.sh`
- `scripts/issue-server-cert.sh`
- `scripts/reload-pull-credentials.sh`
- `README.txt`

### Stack bundle (control‑plane + UI)
- Prebuilt Docker images (control‑plane + gateway)
- `docker-compose.onprem.bundle.yml`
- `.env.onprem.example`
- `hardwareops-installer.sh` (double‑click launcher script)
- `desktop/hardwareops-installer.desktop`
- `scripts/run-stack.sh`
- `scripts/apply-upgrade.sh`
- `scripts/setup-control-plane.sh`
- `scripts/bootstrap-ca.sh`
- `scripts/issue-server-cert.sh`
- `scripts/reload-pull-credentials.sh`
- `scripts/install-docker-ubuntu.sh`
- `scripts/run-coredns.sh`
- `scripts/set-dns.sh`

## Runtime install flow
Installer runtime steps are documented in:
- `installer-flow.md` (fresh machine flow)
- `deploy.md` (canonical deployment path)

## Troubleshooting
### Bundle extracts with no top‑level directory
Rebuild with the latest `build-installers.sh`. The current script always includes a top‑level folder.

### Missing control‑plane binary in bundle
Ensure Go 1.22+ is installed on the build machine and rebuild:
```
./scripts/build-installers.sh
```

## Target platforms
Defaults:
- Agent: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`
- Control‑plane: `linux/amd64`, `linux/arm64`

Override with:
```
AGENT_PLATFORMS="linux/amd64 windows/amd64" \
CONTROL_PLANE_PLATFORMS="linux/arm64" \
./scripts/build-installers.sh
```

## Notes
- Bundles are **not signed** in v1.
- Windows/macOS bundles are **manual run** (no service install yet).
