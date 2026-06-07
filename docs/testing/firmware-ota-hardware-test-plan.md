# Firmware OTA Hardware Test Plan (Bare-Metal / BLE)

Status: Planning · Last updated: 2026-06-03
Design reference: `../development/bare-metal-firmware-ota.md` (roadmap Phase F)
Roadmap items exercised: F1 → F2 → F3 → F4 → F5

This plan covers the **physical equipment to purchase** and the **end-to-end procedure to
test real firmware updates** on bare-metal embedded devices. It is the hardware companion to
the Docker/AWS software test plans (`prod-docker-test-plan.md`, `aws-e2e-test-plan.md`),
which cover everything up to but not including a physical device.

The architecture under test (from the design doc): control plane → Pi-class **gateway** (runs
the agent) → **BLE** → bare-metal **MCU** (Raspberry Pi Pico). A no-touch, over-the-air
firmware update with A/B slots and rollback is the headline acceptance case.

---

## 1) Equipment to purchase

Two tiers. **Tier 1** is the minimum to validate the full gateway→BLE→Pico OTA path. **Tier 2**
adds breadth (a second SoC family, instrumentation, and failure-injection rigs) once Tier 1
passes. Prices are rough USD street estimates for budgeting; buy 2× of consumables (cables,
SD cards, MCUs) for spares.

### Tier 1 — minimum viable OTA test bench

| # | Item | Qty | Purpose | Est. each |
|---|---|---|---|---|
| 1 | Raspberry Pi 5 (4–8 GB) + official PSU | 1 | The **gateway** device (runs the Parcel agent + BLE adapter) | $80 |
| 2 | microSD card 32 GB (A2) | 2 | Gateway OS + a spare | $10 |
| 3 | Raspberry Pi Pico **W** (RP2040 + on-board wireless/BLE) | 3 | The **target MCU**. The "W" gives BLE; buy 3 (one to brick, two to keep) | $7 |
| 4 | Raspberry Pi Debug Probe (or a 2nd Pico as picoprobe) | 1 | SWD debug + UART console for recovery/provisioning and ground-truth flash readback | $12 |
| 5 | USB-A/C to micro-USB cables | 4 | Power + initial UF2 provisioning of the Picos | $5 |
| 6 | Solderless breadboard + jumper wires (M/F, M/M) | 1 kit | Wire SWD/UART between probe and Pico | $15 |
| 7 | Powered USB hub | 1 | Stable power for multiple targets off the gateway/laptop | $20 |
| 8 | USB BLE 5.x dongle (if not using the Pi's built-in radio) | 1 | Deterministic, swappable BLE controller on the gateway | $15 |

**Tier 1 total: ~$200.**

### Tier 2 — breadth, instrumentation, failure injection

| # | Item | Qty | Purpose | Est. each |
|---|---|---|---|---|
| 9 | Nordic nRF52840 dev kit (or ESP32-C3/-S3 board) | 2 | Second SoC/BLE stack — proves the design isn't RP2040-specific | $30–50 |
| 10 | USB programmable power supply / relay (e.g. USB power switch) | 1 | **Power-loss-mid-flash** injection (the key bare-metal failure case) | $25 |
| 11 | Logic analyzer (8-ch, sigrok-compatible) | 1 | Capture SWD/UART/SPI flash traffic; verify slot writes | $15 |
| 12 | BLE sniffer (nRF52840 dongle w/ sniffer firmware) | 1 | Verify the BLE link is paired/bonded + encrypted (security acceptance) | $12 |
| 13 | Faraday bag / RF attenuator | 1 | Force a **lossy/dropped BLE link** to test resumable transfer | $20 |
| 14 | Spare Pico W / nRF boards | 4 | Brick budget for destructive rollback tests | $7–40 |

**Tier 2 total: ~$250–400.** Grand total Tier 1 + Tier 2: **~$450–600.**

> Buying note: the Pico **W** (not the plain Pico) is required for BLE. If early bring-up will
> use **serial/UART** instead of BLE (roadmap F6), the plain Pico + the Debug Probe is enough
> and you can defer the BLE-specific items (8, 12, 13).

---

## 2) Test environment

- **Control plane:** the existing Docker lab (`prod-docker-lab.md`) or an AWS dev stack. No
  new control-plane infra is needed — firmware is just an artifact with `type=firmware`.
- **Gateway:** Raspberry Pi 5 running the Parcel agent, enrolled normally (mTLS), with the BLE
  transport adapter (F4). Until F4 exists, the gateway runs a **harness script** that performs
  the BLE transfer so the bench is usable before the production adapter lands.
- **Target:** Pico W flashed once over USB (UF2) with the **OTA bootloader + slot-A app**
  (F5). After that first provisioning, all updates are over the air.
- **Ground truth:** the Debug Probe (SWD) reads back flash contents independently of the
  agent, so "the device reports success" can be cross-checked against "the bytes are actually
  there."

---

## 3) Pre-hardware gate (do this first, no equipment needed)

Before any device arrives, validate the software-only layers so hardware time is spent on
real integration, not avoidable bugs:

1. **F1 merged** — `Transport` interface exists; `cd agent && go test ./...` green.
2. **Firmware artifact packs/registers** — `python3 scripts/artifact-pack.py` with
   `type=firmware` + a `firmware.json` (per design doc §6); it uploads, signs (Ed25519), and
   shows `verificationStatus=verified` in the UI.
3. **SHA-256 + signature verify path** — reuse `downloadAndVerify` / `verifyEd25519`
   (`agent/internal/artifacts/apply.go`) against a sample firmware blob in a unit test.
4. **Simulated flasher** — the design doc's "simulated flasher + failure injection"
   (testing plan §v2) passes in software before touching a real Pico.

---

## 4) Hardware test cases

Severity: **P0** = must pass before claiming the feature; **P1** = important; **P2** = breadth.

### Bring-up

| ID | Case | Steps | Expected | Sev |
|---|---|---|---|---|
| HW-01 | Gateway enrolls | Flash Pi 5, install agent, enroll via approval mode | Gateway appears online in UI; mTLS check-in succeeds | P0 |
| HW-02 | Pico provisioned | UF2-flash OTA bootloader + slot-A v1.0.0 over USB | Pico boots v1.0.0; SWD readback confirms slot A populated | P0 |
| HW-03 | Sub-device visible | Gateway discovers Pico over BLE, registers it as a sub-device (F3) | Pico shows in device directory owned by the gateway, with `targetSoc=rp2040` | P0 |

### Happy-path OTA (the headline case)

| ID | Case | Steps | Expected | Sev |
|---|---|---|---|---|
| HW-10 | No-touch OTA update | Register firmware v1.1.0; set desired-state for the Pico; do **not** touch the device | Gateway fetches+verifies, streams over BLE to slot B, Pico reboots into v1.1.0; UI shows applied; **no physical interaction** | P0 |
| HW-11 | Ground-truth verify | After HW-10, SWD-read both slots | Slot B = v1.1.0 and active; slot A = prior v1.0.0 retained | P0 |
| HW-12 | Verified-only enforcement | Register firmware with a bad/missing signature; target the Pico | Gateway refuses to transfer; apply-result = signature error; device stays on prior version | P0 |
| HW-13 | Compatibility gating | Target the Pico with firmware whose `firmware.json` `targetSoc`/`hwRevision` mismatches | Update refused before any flash; clear "incompatible" apply-result | P0 |

### Rollback & resilience (where bare-metal earns its keep)

| ID | Case | Steps | Expected | Sev |
|---|---|---|---|---|
| HW-20 | Bad-image rollback | Push a firmware that fails its post-boot health check | Bootloader reverts to last-good slot; device returns to prior version; apply-result = rolled back | P0 |
| HW-21 | Power loss mid-flash | Cut USB power (item 10) during slot-B write | On repower, device boots last-good slot A intact (no brick); update retried | P0 |
| HW-22 | Dropped BLE link | Faraday bag / attenuator (item 13) mid-transfer | Transfer **resumes** (not restart-from-zero) when link returns; final image verifies | P1 |
| HW-23 | Anti-rollback | Attempt to push a lower version than `bootloaderMinVersion` | Downgrade refused | P1 |

### Security

| ID | Case | Steps | Expected | Sev |
|---|---|---|---|---|
| HW-30 | Encrypted BLE link | Sniff the transfer (item 12) | Firmware bytes are not readable in clear; link is paired/bonded + encrypted | P0 |
| HW-31 | Unpaired rejection | Attempt transfer from an unbonded BLE central | Target rejects the OTA session | P1 |

### Breadth (Tier 2)

| ID | Case | Steps | Expected | Sev |
|---|---|---|---|---|
| HW-40 | Second SoC | Repeat HW-10/HW-20 on nRF52840 or ESP32 (item 9) | Same OTA + rollback behavior on a different BLE stack | P2 |
| HW-41 | Multi-target fan-out | Stage an update to 2–3 Picos behind one gateway | Staged rollout with per-device apply-results; failure threshold halts the wave | P2 |
| HW-42 | Serial/UART path | If F6 in scope: same update over UART via Debug Probe | Verified update over a non-BLE transport | P2 |

---

## 5) Exit criteria

The bare-metal OTA capability is "demonstrated" when, on Tier-1 hardware, **all P0 cases pass**:
a signed firmware image is delivered fully over the air to a Pico W (HW-10), verified against
ground truth (HW-11), refuses bad/incompatible images (HW-12/13), rolls back a bad image
(HW-20), survives power-loss-mid-flash without bricking (HW-21), and transfers over an
encrypted link (HW-30). P1/P2 broaden confidence and SoC coverage.

Record results in a dated run log alongside this file (e.g.
`firmware-ota-hardware-run-2026-07-01.md`), matching how `aws-e2e-test-plan.md` runs are
captured. Update the F4/F5 roadmap **Acceptance** notes with the run reference when P0 passes.

---

## 6) Procurement checklist (copy/paste)

```
Tier 1 (~$200) — order first:
[ ] Raspberry Pi 5 (4–8GB) + PSU            x1
[ ] microSD 32GB A2                          x2
[ ] Raspberry Pi Pico W                      x3
[ ] Raspberry Pi Debug Probe                 x1
[ ] micro-USB + USB-C cables                 x4
[ ] Breadboard + jumper kit                  x1
[ ] Powered USB hub                          x1
[ ] USB BLE 5.x dongle                       x1

Tier 2 (~$250–400) — after Tier 1 P0 passes:
[ ] nRF52840 DK or ESP32-C3/S3 board         x2
[ ] USB programmable power switch/relay      x1
[ ] Logic analyzer (sigrok)                  x1
[ ] nRF52840 dongle (BLE sniffer fw)         x1
[ ] Faraday bag / RF attenuator              x1
[ ] Spare MCUs (brick budget)                x4
```
