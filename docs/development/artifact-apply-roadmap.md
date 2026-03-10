# Artifact Apply Roadmap (Firmware + Container Image)

Canonical roadmap: `development/roadmap.md`  
Use this file for artifact-apply-specific planning detail.

This document describes how we will evolve artifact apply beyond bundles (`app_bundle`, `config_bundle`, `data_bundle`) and the interfaces already scaffolded in the agent.

## Current v1 Behavior
- **Apply-capable**: `app_bundle`, `config_bundle`, `data_bundle`
  - Download → verify → extract → optional `plan.yaml` → health check → symlink switch.
- **Register-only**: `firmware`, `container_image`
  - Artifact can be uploaded/registered but is not applied by the agent.

## Pre-Apply Safety Hook (all artifact types)
We will add a **pre-apply script** that runs before any artifact apply to put the device into a safe state.
This is required for *all* artifact types and should be optional per artifact.

Proposed approach:
- Add an optional `preApply` entry (or `preapply.sh`) in the artifact bundle.
- The agent executes it **before** any apply logic.
- If it fails or times out, the apply is aborted and an error is reported.
- For containers/firmware, this is where we can stop services, drain traffic, or validate hardware state.

Implementation choice (recommended):
- **`plan.yaml` step type**: `script.preApply`
  - Executed before any apply logic for all artifact types.
  - If it fails or times out, the apply is aborted.

Example `plan.yaml` snippet:
```yaml
version: "v1"
steps:
  - id: safety
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 120
```

### Demo override (visual validation)
For demos, the agent can be configured to apply unsupported types via the bundle flow:
```
ALLOW_UNSUPPORTED_APPLY=1
```
`scripts/run-demo-agent.sh` sets this to `1` by default so all artifact types can show
visible changes at the demo web URL. Set to `0` to enforce realistic behavior.

## Agent Apply Interfaces (Scaffolded)
Location: `agent/internal/artifacts/apply_interfaces.go`

Scaffolded interfaces:
```go
type ApplyContext struct {
  Root       string
  VersionDir string
  Desired    Desired
  Meta       ArtifactMeta
  Manifest   Manifest
  Logger     Logger
}

type FirmwareApplier interface {
  ApplyFirmware(ctx ApplyContext) error
}

type ContainerImageApplier interface {
  ApplyContainerImage(ctx ApplyContext) error
}
```

Default implementations are stubs that return `ErrApplyNotImplemented`, so future
implementations can drop in without changing the call flow.

## Firmware Apply Plan (v2)
### Artifact contents (proposed)
- `files/firmware.bin` (or vendor-specific package)
- `firmware.json` (required):
  - `deviceModel`, `hwRevision`, `currentMinVersion`
  - `checksum` (sha256)
  - `installType` (slotA/slotB, single-image)
  - `rebootRequired` (bool)

### Agent apply steps
1. **Preflight**: power/battery, idle state, current firmware version, free space.
2. **Stage**: verify checksum, validate model/hw revision, copy to staging.
3. **Flash**: call device‑specific flasher (driver/plugin).
4. **Verify**: read back version/hash, check boot health.
5. **Rollback**: revert to last‑good slot or re‑flash prior image.

### Control‑plane changes
- Device capability inventory (model, hw rev).
- Targeting rules (only deliver compatible firmware).
- Staged rollout + failure thresholds.
- Firmware specific status fields and apply results.

## Container Image Apply Plan (v2)
### Artifact contents (proposed)
- `image.tar` (required): output of `docker save` (or OCI archive). The agent will `docker load` it locally.
- `container.json` (required):
  - `imageRef` + `digest` (from the loaded image)
  - `runtime` (`docker` initially; containerd later)
  - `containerName`
  - `ports`, `env`, `volumes`
  - `health` (HTTP/tcp/command)

### Agent apply steps
1. **Load** image archive with `docker load`.
2. **Verify** digest matches `container.json` (optional signature later).
3. **Stop** the existing container (if running).
4. **Start** the new container (same name or versioned name).
5. **Health check**.
6. **Rollback**: stop new container and restart previous image on failure.

### Control‑plane changes
- Artifact registration stores `imageRef`, `digest`, and runtime config metadata.
- Image allowlist + digest enforcement.
- Per‑device overrides for env/ports/secrets.

## Testing Plan (v2)
- Firmware: simulated flasher + intentional failure injection.
- Container image: fake registry + health check failovers.
- Validate rollback behavior and apply-result reporting.

## Security Considerations
- Strict compatibility validation (model/hw revision).
- Signature verification for firmware/images.
- Rate-limited and staged rollouts.
