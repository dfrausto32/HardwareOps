# Installer Flow (Fresh Machines)

Canonical deployment guide: `deploy.md`  
Use this file for installer-specific detail.

This is the installer-focused runbook for installing the control-plane + UI + agents using installer bundles.
For the agent bundle, the approval-mode bootstrap path is the primary production flow.

## 1) Build installers (build machine)
From the repo root:
```
./scripts/build-installers.sh
```
For bundle contents, naming, and target platform details, see `installers.md`.

## 2) Install control‑plane + UI (stack bundle)
Copy the stack bundle to the control‑plane VM:
```
scp dist/installers/<version>/hardwareops-stack-<version>-linux-<arch>.tar.gz <user>@<cp_vm_ip>:~/
```

On the control‑plane VM:
```
tar -xzf hardwareops-stack-<version>-linux-<arch>.tar.gz
cd stack-<version>-linux-<arch>

sudo ./scripts/install-docker-ubuntu.sh
sudo ./scripts/run-stack.sh
```

Alternative (Linux desktop double‑click):
- Open `desktop/hardwareops-installer.desktop`
- Mark it as **Trusted** when prompted
- It runs `hardwareops-installer.sh` and starts the stack in a terminal

Verify:
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/healthz
```
If DNS isn’t ready yet, use:
```
curl --cacert /opt/hardwareops/certs/ca.crt \
  --resolve hardwareops.internal:443:127.0.0.1 \
  https://hardwareops.internal/healthz
```

## 3) Optional DNS (CoreDNS)
For full DNS setup options, use `dns-coredns.md`.
Quick commands:
- Control-plane VM: `DNS_IP=<CONTROL_PLANE_IP> ./scripts/run-coredns.sh`
- Agent VM(s): `DNS_SERVER=<CONTROL_PLANE_IP> ./scripts/set-dns.sh`

## 4) Install agent
Copy the agent bundle to the agent VM:
```
scp dist/installers/<version>/hardwareops-agent-<version>-linux-<arch>.tar.gz <user>@<agent_vm_ip>:~/
```

On the agent VM:
```
tar -xzf hardwareops-agent-<version>-linux-<arch>.tar.gz
cd hardwareops-agent-<version>-linux-<arch>

sudo ./scripts/install-agent-deps-ubuntu.sh
sudo mkdir -p /opt/hardwareops/certs
```

Copy the CA cert from the control‑plane VM:
```
scp /opt/hardwareops/certs/ca.crt <user>@<agent_vm_ip>:/opt/hardwareops/certs/ca.crt
```

Create an enrollment profile in the control-plane UI:
- Open `https://hardwareops.internal`
- Go to `Security -> Enrollment profiles`
- Create a profile with `Require approval` enabled
- Copy the bootstrap token and deliver it to the operator performing the install

Install + start in approval mode:
```
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./hardwareops-agent \
  CONTROL_PLANE_URL=https://agent.hardwareops.internal \
  CONTROL_PLANE_CA_CERT_SRC=/opt/hardwareops/certs/ca.crt \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
  START_SERVICE=1
```

If the agent endpoint uses a publicly trusted server certificate, omit `CONTROL_PLANE_CA_CERT_SRC`
and add `USE_SYSTEM_CA=1`.

Confirm the service is alive before approval:
```
sudo systemctl status hardwareops-agent --no-pager
sudo journalctl -u hardwareops-agent -f
```

Expected log messages:
- `pending enrollment requested request=<id>`
- `pending enrollment awaiting approval request=<id>`

Approve the request:
- UI: `Security -> Pending enrollments -> Approve`
- API: `POST /api/v1/pending-enrollments/{requestId}/approve`

Verify the same installed service materializes identity and moves into mTLS check-in:
```
sudo test -s /etc/hardwareops/agent/certs/device.crt
sudo test -s /etc/hardwareops/agent/certs/device.key
sudo cat /var/lib/hardwareops/agent/device-id
sudo test ! -e /var/lib/hardwareops/agent/bootstrap-state.json
sudo journalctl -u hardwareops-agent -n 50 --no-pager
```

If the agent fails with `permission denied` on `device.key`:
```
sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
sudo systemctl restart hardwareops-agent
```

Verify from control‑plane:
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/api/v1/devices
```

Legacy direct enrollment is still available when you intentionally need the old token-enroll path:
```
sudo CONTROL_PLANE_URL=https://agent.hardwareops.internal \
     CA_CERT_PATH=/opt/hardwareops/certs/ca.crt \
     ./scripts/agent-enroll.sh
sudo systemctl restart hardwareops-agent
```

## 5) Build an upgrade package (build machine)
From the repo root:
```
./scripts/build-upgrade-package.sh
```

Outputs:
- `dist/upgrades/<version>/hardwareops-upgrade-<version>-linux-<arch>.tar.gz`

## 6) Apply an upgrade (control‑plane VM)
Copy the upgrade bundle to the control‑plane VM:
```
scp dist/upgrades/<version>/hardwareops-upgrade-<version>-linux-<arch>.tar.gz <user>@<cp_vm_ip>:~/
```

On the control‑plane VM:
```
tar -xzf hardwareops-upgrade-<version>-linux-<arch>.tar.gz
cd hardwareops-upgrade-<version>-linux-<arch>
STACK_DIR=$PWD ENV_FILE=.env.onprem.example ./scripts/apply-upgrade.sh
```

For **UI auto‑apply**, extract the upgrade bundle **into the same stack directory**
that is mounted to `/stack` (see `STACK_DIR` in `.env.onprem`).
Then enable maintenance in the UI and click **Apply update**.

Alternative: place the upgrade tarball in `/stack/updates` and let the UI detect it:
```
mkdir -p /opt/hardwareops/stack/updates
cp hardwareops-upgrade-<version>-linux-<arch>.tar.gz /opt/hardwareops/stack/updates/
```

## Troubleshooting
- If the stack bundle doesn’t extract into a folder, rebuild with the latest `build-installers.sh`.
- If images won’t load, check disk space and Docker status.
- For DNS resolution issues, use `dns-coredns.md`.
- For TLS/certificate issues, use `certs.md`.
- For proxy/header hardening issues, use `deployment-hardening.md`.
- For gateway `502`, check `control-plane` container logs and restart the service.
