# Bare-Metal Firmware OTA (Gateway + BLE)

Canonical roadmap: `development/roadmap.md` (Phase F)
Companion design doc: `development/artifact-apply-roadmap.md` (agent-resident firmware + container apply)
Operator references: `agent-systemd.md`, `../icd.md`

This document is the engineering game plan for delivering **true firmware updates to bare-metal
microcontrollers** (e.g. Raspberry Pi Pico / RP2040) that cannot run the Parcel agent and have
no IP networking. The guiding principle is the **most remote update path possible**: devices
should update over the air without any physical or USB touch.

It is a design/planning document. None of the components below are implemented yet; status is
tracked in `roadmap.md` Phase F (items F1–F6).

## 1) Problem and scope

The Parcel agent runs well on embedded *Linux* devices, but bare-metal MCUs are different:

- No Linux userspace, so the Go agent cannot run on the device.
- Often no IP stack — they reach the world over BLE, serial/UART, or a radio.
- Constrained flash/RAM, so on-device cryptographic verification is limited.

There are two distinct firmware delivery models. Keeping them separate avoids conflating very
different architectures:

- **Agent-resident firmware apply** — a Linux-class device *runs the agent* and flashes itself
  or an attached MCU. This is the existing "Firmware Apply Plan (v2)" in
  `artifact-apply-roadmap.md`.
- **Gateway-mediated bare-metal firmware apply** — the target *cannot run the agent*. A
  Pi-class **gateway** runs the agent and relays firmware to MCUs over a constrained link
  (BLE first). **This document covers that model.**

Today firmware apply is unimplemented either way: `agent/internal/artifacts/apply_interfaces.go`
wires `defaultFirmwareApplier = firmwareApplierStub{}`, whose `ApplyFirmware` returns
`ErrApplyNotImplemented{Type: "firmware"}`. And the agent's transport is hard-wired to HTTP/mTLS
(`agent/internal/client/client.go`), so there is no seam for a non-IP transport.

## 2) Architecture: gateway / device-of-devices

A **gateway** is a normal enrolled agent (Pi-class, IP-connected) that additionally represents
N **sub-devices** (MCUs) it can reach over BLE. The gateway fetches firmware from the control
plane over the existing HTTP/mTLS path, verifies it, then relays it to the target over BLE.

```
┌──────────────────────────────────────────────┐
│              Control Plane                     │
│  - Artifact registry (firmware)                │
│  - Sub-device directory + capability inventory │
│  - Firmware targeting + staged rollout         │
└───────────────────────┬────────────────────────┘
              HTTP / mTLS │ (existing transport)
                ┌─────────▼─────────┐
                │   Gateway Agent   │  (Pi-class, runs the agent)
                │  - relays N MCUs  │
                └───┬───────────┬───┘
              BLE   │           │   BLE
            ┌───────▼──┐    ┌───▼───────┐
            │  MCU /   │    │  MCU /    │   ...
            │  Pico A  │    │  Pico B   │
            └──────────┘    └───────────┘
```

The sub-devices appear in the control-plane device directory as devices owned by the gateway.
Their firmware desired-state, capability inventory, and apply-results are **relayed** by the
gateway — the MCU never authenticates to the control plane directly. The gateway holds the mTLS
identity; BLE link security (pairing/bonding + encryption) protects the gateway↔MCU hop.

## 3) Transport abstraction

The single most important enabling change: decouple "how the agent talks to its upstream" from
the rest of the apply/check-in logic. Today `agent/internal/client/client.go`
(`NewWithTLS`, `CheckIn`, `GetArtifact`, `PresignArtifact`, plus apply-result posting) is the
only transport, and it assumes IP + mTLS.

Proposed seam — a `Transport` interface (roadmap item F1):

```go
type Transport interface {
    CheckIn(ctx context.Context, req CheckinRequest) (*CheckinResponse, error)
    GetArtifact(ctx context.Context, artifactID string) (*ArtifactResponse, error)
    PresignArtifact(ctx context.Context, artifactID string) (*PresignResponse, error)
    PostApplyResult(ctx context.Context, deviceID string, result ApplyResultRequest) error
}
```

- The current client becomes `HTTPTransport` (no behavior change).
- A `BLETransport` (gateway → MCU sub-device) implements the same interface for the relayed leg.
- Future `SerialTransport`/radio adapters slot in behind the same interface (F6).

This keeps the check-in loop in `agent/cmd/agent/main.go` and the apply dispatch in
`agent/internal/artifacts/apply.go` transport-agnostic.

## 4) BLE OTA protocol (first transport)

BLE is the first adapter because it is the most broadly available **no-touch** link on small
devices. Design points:

- **GATT firmware service** — a dedicated service/characteristics for: start/offer (with
  metadata + total size + hash), chunk write, progress/ack, commit, and status/result.
- **Chunked + resumable transfer** — the link is bandwidth-constrained and lossy; transfer in
  bounded chunks with per-chunk acknowledgement and the ability to resume after a dropped
  connection rather than restarting.
- **Verification at the gateway boundary** — the gateway reuses the existing
  SHA-256 verification (`downloadAndVerify`) and Ed25519 signature verification (`verifyEd25519`
  in `agent/internal/artifacts/apply.go`) **before** streaming any bytes to the MCU. Where the
  MCU bootloader can verify a hash/signature itself, do so as a second check; where it cannot
  (RAM/flash limits), the gateway is the trust boundary and the BLE link must be encrypted.
- **Link security** — require BLE pairing/bonding with encryption; never transfer firmware over
  an unauthenticated link.

## 5) Bare-metal flashing (RP2040 / Pi Pico)

For a *remote, no-touch* update the device must accept and apply firmware while running — the
RP2040 USB BOOTSEL/UF2 path requires physically holding a button and is therefore **not** a
remote path. The plan:

- **Application-level OTA bootloader** — a small resident bootloader that receives an image into
  an inactive slot, validates it, and switches over on next boot. (`picotool` and UF2 remain
  useful for *initial* factory provisioning, not for remote updates.)
- **A/B firmware slots** — write to the inactive slot, verify a readback hash, mark it active,
  reboot into it.
- **Verify-readback + rollback** — confirm the new image boots healthy; if not, the bootloader
  reverts to the last-good slot. This mirrors the agent's existing staged-apply + rollback
  contract for bundles.

## 6) Firmware artifact format

Extend the proposed `firmware.json` from `artifact-apply-roadmap.md` with MCU/transport fields.
Firmware metadata is opaque to the control plane today (stored in the artifact `MetadataJSON`
column — `control-plane/internal/store/store.go`), so no schema migration is required to carry it.

```json
{
  "deviceModel": "pico-w",
  "targetSoc": "rp2040",
  "hwRevision": "1.0",
  "currentMinVersion": "0.4.0",
  "checksum": "<sha256>",
  "transport": "ble",
  "bleService": "0000fe59-0000-1000-8000-00805f9b34fb",
  "slot": "b",
  "bootloaderMinVersion": "1.2.0",
  "rebootRequired": true
}
```

`transport` is `ble` initially (`serial` and others reserved for F6). `slot` drives the A/B
flashing strategy in section 5.

## 7) Control-plane changes

- **Sub-device directory + capability inventory** — represent MCUs as devices owned by a
  gateway, with `model`, `targetSoc`, and `hwRevision` capability fields. The artifact `Type`
  is already `"firmware"` and is register-only today (`store.go`); these changes add targeting
  and relayed status on top.
- **Firmware targeting rules** — only deliver firmware whose `firmware.json` compatibility
  (`deviceModel`/`targetSoc`/`hwRevision`/`currentMinVersion`) matches the sub-device.
- **Staged rollout + failure thresholds** — roll firmware out in waves with automatic halt on
  failure-rate thresholds (especially important for hard-to-recover bare-metal fleets).
- **Relayed apply-results** — the gateway posts per-sub-device apply-results; artifact
  registration and presign endpoints (`../icd.md`) are unchanged.

## 8) Security considerations

- **Strict compatibility gating** — refuse to flash if model/soc/hw-revision/min-version do not
  match; a wrong image can brick a remote device.
- **Signature verification** — Ed25519 (and the existing Cosign/ECDSA/RSA paths in `apply.go`)
  verified at the gateway, and on the MCU where feasible.
- **Encrypted, bonded BLE** — the gateway↔MCU hop is the trust boundary; require pairing/bonding
  and link encryption.
- **Anti-rollback / slot integrity** — prevent downgrade attacks and partial-write corruption
  via slot validation and minimum-version enforcement.
- **Rate-limited, staged rollouts** — bound blast radius; never fan a firmware change to an
  entire fleet at once.

## 9) Phasing summary (maps to roadmap Phase F)

| Item | Deliverable | Depends on |
|---|---|---|
| F1 | Agent transport abstraction (`Transport` interface; HTTP becomes one impl) | — |
| F2 | Agent-resident firmware apply (implement `FirmwareApplier`; A/B + rollback) | apply interfaces |
| F3 | Gateway / device-of-devices model (sub-device directory + relayed status) | F1 |
| F4 | BLE OTA transport adapter (no-touch, chunked/resumable, verified) | F1, F3 |
| F5 | RP2040 / Pi Pico bare-metal target (OTA bootloader, A/B slots, rollback) | F4 |
| F6 | Additional constrained transports (serial/UART, radio) behind F1 interface | F1 |

See `development/roadmap.md` Phase F for status and acceptance criteria of each item.
