# Playbook: ISO-Based Fleet Provisioning

## When to Use

Use this playbook when you are deploying Parcel agents to many machines by writing a pre-built OS image (ISO, disk image, or golden image) to each device rather than provisioning each machine individually.

Choose this path when:
- You are flashing bare-metal devices or embedded hardware at scale.
- You want every device to boot network-ready with the Parcel agent pre-installed, requiring no per-device manual setup.
- Devices are identical hardware models and can share a single base image.
- You need consistent, repeatable provisioning without a PXE/netboot infrastructure.

Do **not** use this when:
- Devices already have an OS and you just need to install the agent — use `scripts/agent-install.sh` directly.
- Devices are cloud instances — use cloud-init or EC2 user-data instead.
- You need per-device customization before first boot that cannot be handled by the agent enrollment flow.

---

## How It Works

The ISO contains the agent binary, systemd services, and an enrollment config template. **It does not contain any device identity material.** On first boot each machine:

1. `parcel-machine-id-init.service` runs and ensures `/etc/machine-id` is unique (see below).
2. `parcel-agent.service` starts, detects no device cert exists, and enters the enrollment bootstrap loop.
3. The agent generates a fresh Ed25519 keypair unique to this machine.
4. The agent submits a pending enrollment request to the control plane using the shared enrollment profile token baked into the image.
5. The control plane assigns a UUID as the device ID and issues a signed certificate.
6. The agent saves its identity to disk and transitions to normal check-in.

Every machine gets a unique keypair, a unique certificate, and a unique device ID — regardless of how many machines are cloned from the same ISO.

---

## The machine-id Problem (and the Fix)

Linux machines cloned from the same ISO share an identical `/etc/machine-id`. The Parcel agent reads this file as its hardware fingerprint. If two machines present the same fingerprint, the control plane rejects the second with **409 Conflict** and it cannot enroll.

**The fix is already included in the standard installer:** `parcel-machine-id-init.service` runs once on first boot, detects an uninitialized or cloned machine-id, calls `systemd-machine-id-setup --commit` to generate a unique one, and drops a sentinel file so it never runs again.

This runs **before** `parcel-agent.service`, so the agent always sees a unique hardware identity at enrollment time.

---

## What to Include in the ISO

### Include
- Agent binary (`/usr/local/bin/parcel-agent`)
- `parcel-machine-id-init.sh` (`/usr/local/bin/parcel-machine-id-init.sh`)
- `parcel-machine-id-init.service` (`/etc/systemd/system/parcel-machine-id-init.service`)
- `parcel-agent.service` (`/etc/systemd/system/parcel-agent.service`)
- Agent env file (`/etc/parcel/agent/agent.env`) with:
  - `CONTROL_PLANE_URL=https://agent.your-domain.com`
  - `ENROLLMENT_PROFILE_TOKEN=ep_tok_...`
  - `AGENT_ENROLL_MODE=approval`
- Control plane CA cert (`/etc/parcel/agent/certs/ca.crt`) — or set `USE_SYSTEM_CA=1` if the CA is in the system trust store

### Exclude (must NOT be in the image)
- Device certificate (`/etc/parcel/agent/certs/device.crt`)
- Device private key (`/etc/parcel/agent/certs/device.key`)
- Device ID file (`/var/lib/parcel/agent/device-id`)
- Bootstrap state (`/var/lib/parcel/agent/bootstrap-state.json`)
- Agent state (`/var/lib/parcel/agent/state.json`)
- Machine-id sentinel (`/var/lib/parcel/agent/.machine-id-initialized`)

If any of these exist in the image, devices will either conflict on enrollment or inherit a stale device identity.

---

## Building the Image

### 1. Install the agent into your image build environment

Run the standard installer against your image's chroot or mounted filesystem:

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=/path/to/parcel-agent \
  CONTROL_PLANE_URL=https://agent.your-domain.com \
  CONTROL_PLANE_CA_CERT_SRC=/path/to/ca.crt \
  ENROLLMENT_PROFILE_TOKEN=ep_tok_... \
  AGENT_ENROLL_MODE=approval \
  START_SERVICE=0
```

`START_SERVICE=0` is important — do not start the agent during image build. It should only start on the real device's first boot.

### 2. Verify the data directories are empty

Before sealing the image:

```bash
# These should all be empty or absent:
ls /var/lib/parcel/agent/
ls /etc/parcel/agent/certs/
```

If `device.crt`, `device.key`, `device-id`, `bootstrap-state.json`, `state.json`, or `.machine-id-initialized` are present, remove them before sealing.

### 3. Clear or unset machine-id

Some image build tools automatically reset `/etc/machine-id`. If yours does not:

```bash
# Option A — remove it entirely (systemd regenerates on boot)
rm -f /etc/machine-id

# Option B — write the uninitialized placeholder
echo "uninitialized" > /etc/machine-id
```

`parcel-machine-id-init.service` handles both cases. Either way, each booted machine will get a unique id.

### 4. Seal and distribute the image

Proceed with your normal ISO build / disk image toolchain (`mkisofs`, `dd`, etc.). The services are already enabled via `systemctl enable` run by the installer.

---

## Enrollment Profile Token Management

The enrollment profile token baked into the ISO is a shared credential. Treat it accordingly:

- **Create a dedicated enrollment profile** for ISO-provisioned devices. Do not reuse the same profile for manually-provisioned devices.
- **Set `MaxUses`** on the profile if you know the exact fleet size. This limits exposure if the token is extracted from the image.
- **Rotate the token** between ISO versions using the token rotation API. The old token remains valid through its grace period so devices already in-flight can complete enrollment.
- If the token is compromised, rotate it immediately and re-image affected devices.

---

## Rate Limit Tuning for Large Fleet Deployments

When hundreds of machines boot simultaneously from the same network, all enrollment requests originate from a small number of source IPs (or a single NAT IP). The default rate limits may throttle them. Adjust these env vars on the control plane ECS task before a large provisioning event:

| Variable | Default | Recommended for bulk boot |
|---|---|---|
| `ENROLL_RPM` | 60 | 300–600 |
| `PENDING_ENROLL_REQUEST_RPM_PER_SOURCE` | low | 200+ (if all devices share a NAT IP) |
| `PENDING_ENROLL_MAX_ACTIVE` | low | set to expected fleet size |

The agent handles 429 responses gracefully — it backs off and retries automatically. Higher limits just reduce the time to full enrollment.

---

## License Cap Planning

`MaxDevices` in your Parcel license is a hard cap on enrolled devices. New enrollments are rejected with 403 once the cap is reached.

- Plan your license cap at or above your expected fleet size, including growth headroom.
- Inactive or decommissioned devices still count toward the cap until manually removed.
- Check the current device count in the UI: **Settings → License**.
- There is no automatic cleanup of offline devices.

---

## Monitoring Enrollment Progress

During a bulk provisioning event, monitor enrollment in the control plane UI or via API:

```bash
# Count devices in pending enrollment state
curl -s -H "Authorization: Bearer $TOKEN" \
  "$CONTROL_PLANE_URL/api/v1/pending-enrollments" | jq '.total'

# Count fully enrolled devices
curl -s -H "Authorization: Bearer $TOKEN" \
  "$CONTROL_PLANE_URL/api/v1/devices" | jq '.total'
```

If a device is stuck in `conflict` state, check its machine-id:
```bash
# On the device
cat /etc/machine-id
cat /var/lib/parcel/agent/.machine-id-initialized  # Should exist after first boot
```

If `.machine-id-initialized` is absent, `parcel-machine-id-init.service` did not run. Check:
```bash
systemctl status parcel-machine-id-init.service
journalctl -u parcel-machine-id-init.service
```

---

## First Boot Sequence

On a correctly built ISO, the boot sequence is:

```
systemd-remount-fs.service
  └─ parcel-machine-id-init.service   ← regenerates /etc/machine-id if needed, drops sentinel
       └─ parcel-agent.service         ← enrolls with unique hardware fingerprint, receives UUID
            └─ (normal check-in loop)
```

`parcel-machine-id-init.service` uses `ConditionPathExists=!/var/lib/parcel/agent/.machine-id-initialized` so it is a no-op on every subsequent boot after the sentinel is written.

---

## Verification

Test with two VMs booted from the same image before production rollout:

1. Boot both VMs simultaneously.
2. Confirm `parcel-machine-id-init.service` ran successfully on both:
   ```bash
   systemctl status parcel-machine-id-init.service
   ```
3. Confirm `/etc/machine-id` differs between the two VMs.
4. Confirm `.machine-id-initialized` exists on both:
   ```bash
   ls /var/lib/parcel/agent/.machine-id-initialized
   ```
5. Approve both pending enrollments in the Parcel UI.
6. Confirm both appear as distinct devices with different UUIDs.
7. Confirm no `409 Conflict` errors in agent logs on either machine:
   ```bash
   journalctl -u parcel-agent.service | grep -i conflict
   ```

---

## Reference

- Machine-id init script: `scripts/parcel-machine-id-init.sh`
- Machine-id init service: `deploy/systemd/parcel-machine-id-init.service`
- Agent service: `deploy/systemd/parcel-agent.service`
- Agent installer: `scripts/agent-install.sh`
- Agent systemd guide: `docs/reference/agent-systemd.md`
- First agent onboarding: `docs/customer/first-agent-onboarding.md`
