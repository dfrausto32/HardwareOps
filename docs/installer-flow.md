# Installer Flow (Fresh Machines)

Canonical deployment guide: `deploy.md`  
Use this file for installer-specific detail.

This is the installer-focused runbook for installing the control-plane + UI + agents using installer bundles.

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

Install + enroll:
```
sudo ./scripts/agent-install.sh AGENT_SRC=./hardwareops-agent
sudo CONTROL_PLANE_URL=https://hardwareops.internal \
     CA_CERT_PATH=/opt/hardwareops/certs/ca.crt \
     ./scripts/agent-enroll.sh
sudo systemctl restart hardwareops-agent
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
