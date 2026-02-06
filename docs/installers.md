# Installers (Multi‑OS Bundles)

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
- `scripts/install-docker-ubuntu.sh`
- `scripts/run-coredns.sh`
- `scripts/set-dns.sh`

## Quick cert + env setup (control‑plane bundle)
Inside the control‑plane bundle:
```
./scripts/setup-control-plane.sh
```
This generates:
- `/opt/hardwareops/certs/ca.crt`
- `/opt/hardwareops/certs/ca.key`
- `/opt/hardwareops/certs/server.crt`
- `/opt/hardwareops/certs/server.key`

It also writes `control-plane.env` with CA paths (and TLS paths if `ENABLE_TLS=1`).

## Quick stack setup (stack bundle)
```
sudo ./scripts/install-docker-ubuntu.sh
sudo ./scripts/run-stack.sh
```

### Double‑click installer (Linux desktop)
Inside the stack bundle:
1) Open `desktop/hardwareops-installer.desktop`
2) Mark it as **Trusted** (first‑time prompt)
3) It runs `hardwareops-installer.sh` in a terminal and starts the stack

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
