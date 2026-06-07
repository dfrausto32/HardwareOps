# Installers and Fresh-Machine Install

Canonical deployment guide: `deploy.md`
Use this file for bundle build/layout details and the fresh-machine install runbook.

---

## 1) Build installers

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

---

## 2) Bundle contents

### Agent bundles
- `parcel-agent` binary
- `scripts/agent-install.sh` (Linux systemd install + approval-mode bootstrap config)
- `scripts/agent-enroll.sh` (legacy direct-enroll helper)
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
- `parcel-installer.sh` (double‑click launcher script)
- `desktop/parcel-installer.desktop`
- `scripts/run-stack.sh`
- `scripts/apply-upgrade.sh`
- `scripts/setup-control-plane.sh`
- `scripts/bootstrap-ca.sh`
- `scripts/issue-server-cert.sh`
- `scripts/reload-pull-credentials.sh`
- `scripts/install-docker-ubuntu.sh`
- `scripts/run-coredns.sh`
- `scripts/set-dns.sh`

---

## 3) Install control‑plane + UI (stack bundle)

Copy the stack bundle to the control‑plane VM:
```
scp dist/installers/<version>/parcel-stack-<version>-linux-<arch>.tar.gz <user>@<cp_vm_ip>:~/
```

On the control‑plane VM:
```
tar -xzf parcel-stack-<version>-linux-<arch>.tar.gz
cd stack-<version>-linux-<arch>

sudo ./scripts/install-docker-ubuntu.sh
sudo ./scripts/run-stack.sh
```

Alternative (Linux desktop double‑click):
- Open `desktop/parcel-installer.desktop`
- Mark it as **Trusted** when prompted
- It runs `parcel-installer.sh` and starts the stack in a terminal

Verify:
```
curl --cacert /opt/parcel/certs/ca.crt https://parcel.internal/healthz
```
If DNS isn't ready yet, use:
```
curl --cacert /opt/parcel/certs/ca.crt \
  --resolve parcel.internal:443:127.0.0.1 \
  https://parcel.internal/healthz
```

---

## 4) Optional DNS (CoreDNS)

For full DNS setup options, see `dns-coredns.md`.
Quick commands:
- Control-plane VM: `DNS_IP=<CONTROL_PLANE_IP> ./scripts/run-coredns.sh`
- Agent VM(s): `DNS_SERVER=<CONTROL_PLANE_IP> ./scripts/set-dns.sh`

---

## 5) Install agent

Copy the agent bundle to the agent VM:
```
scp dist/installers/<version>/parcel-agent-<version>-linux-<arch>.tar.gz <user>@<agent_vm_ip>:~/
```

On the agent VM:
```
tar -xzf parcel-agent-<version>-linux-<arch>.tar.gz
cd parcel-agent-<version>-linux-<arch>

sudo ./scripts/install-agent-deps-ubuntu.sh
sudo mkdir -p /opt/parcel/certs
```

Copy the CA cert from the control‑plane VM:
```
scp /opt/parcel/certs/ca.crt <user>@<agent_vm_ip>:/opt/parcel/certs/ca.crt
```

Create an enrollment profile in the control-plane UI:
- Open `https://parcel.internal`
- Go to `Security -> Enrollment profiles`
- Create a profile with `Require approval` enabled
- Copy the bootstrap token and deliver it to the operator performing the install

Install + start in approval mode:
```
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./parcel-agent \
  CONTROL_PLANE_URL=https://agent.parcel.internal \
  CONTROL_PLANE_CA_CERT_SRC=/opt/parcel/certs/ca.crt \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
  START_SERVICE=1
```

If the agent endpoint uses a publicly trusted server certificate, omit `CONTROL_PLANE_CA_CERT_SRC`
and add `USE_SYSTEM_CA=1`.

Confirm the service is alive before approval:
```
sudo systemctl status parcel-agent --no-pager
sudo journalctl -u parcel-agent -f
```

Expected log messages:
- `pending enrollment requested request=<id>`
- `pending enrollment awaiting approval request=<id>`

Approve the request:
- UI: `Security -> Pending enrollments -> Approve`
- API: `POST /api/v1/pending-enrollments/{requestId}/approve`

Verify the agent materialized identity and moved into mTLS check-in:
```
sudo test -s /etc/parcel/agent/certs/device.crt
sudo test -s /etc/parcel/agent/certs/device.key
sudo cat /var/lib/parcel/agent/device-id
sudo test ! -e /var/lib/parcel/agent/bootstrap-state.json
sudo journalctl -u parcel-agent -n 50 --no-pager
```

If the agent fails with `permission denied` on `device.key`:
```
sudo chown -R parcel:parcel /etc/parcel/agent/certs
sudo systemctl restart parcel-agent
```

Verify from control‑plane:
```
curl --cacert /opt/parcel/certs/ca.crt https://parcel.internal/api/v1/devices
```

Legacy direct enrollment (when you intentionally need the old token-enroll path):
```
sudo CONTROL_PLANE_URL=https://agent.parcel.internal \
     CA_CERT_PATH=/opt/parcel/certs/ca.crt \
     ./scripts/agent-enroll.sh
sudo systemctl restart parcel-agent
```

For the full approval-mode agent runbook (persistence, CA rotation, re-enrollment), see `agent-systemd.md`.

---

## 6) Build an upgrade package (build machine)

```
./scripts/build-upgrade-package.sh
```

Outputs:
- `dist/upgrades/<version>/parcel-upgrade-<version>-linux-<arch>.tar.gz`

---

## 7) Apply an upgrade (control‑plane VM)

Copy the upgrade bundle to the control‑plane VM:
```
scp dist/upgrades/<version>/parcel-upgrade-<version>-linux-<arch>.tar.gz <user>@<cp_vm_ip>:~/
```

On the control‑plane VM:
```
tar -xzf parcel-upgrade-<version>-linux-<arch>.tar.gz
cd parcel-upgrade-<version>-linux-<arch>
STACK_DIR=$PWD ENV_FILE=.env.onprem.example ./scripts/apply-upgrade.sh
```

For **UI auto‑apply**, extract the upgrade bundle into the same stack directory
mounted to `/stack` (see `STACK_DIR` in `.env.onprem`).
Then enable maintenance in the UI and click **Apply update**.

Alternative: place the upgrade tarball in `/stack/updates` and let the UI detect it:
```
mkdir -p /opt/parcel/stack/updates
cp parcel-upgrade-<version>-linux-<arch>.tar.gz /opt/parcel/stack/updates/
```

---

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

Notes:
- Bundles are **not signed** in v1.
- Windows/macOS bundles are **manual run** (no service install yet).

---

## Troubleshooting

- If the stack bundle doesn't extract into a folder, rebuild with the latest `build-installers.sh`.
- If images won't load, check disk space and Docker status.
- For DNS resolution issues, see `dns-coredns.md`.
- For TLS/certificate issues, see `certs.md`.
- For proxy/header hardening issues, see `deployment-hardening.md`.
- For gateway `502`, check `control-plane` container logs and restart the service.
- If the agent fails with `permission denied` on device certs:
  ```
  sudo chown -R parcel:parcel /etc/parcel/agent/certs
  sudo systemctl restart parcel-agent
  ```
