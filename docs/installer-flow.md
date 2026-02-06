# Installer Flow (Fresh Machines)

This is the **single source of truth** for installing the control‑plane + UI + agents using the installer bundles.

## 1) Build installers (build machine)
From the repo root:
```
./scripts/build-installers.sh
```

Outputs:
- `dist/installers/<version>/hardwareops-stack-<version>-linux-<arch>.tar.gz`
- `dist/installers/<version>/hardwareops-agent-<version>-linux-<arch>.tar.gz`
- `dist/installers/<version>/control-plane-<version>-linux-<arch>.tar.gz` (binary-only)

**Note:** the **stack bundle** is architecture-specific (build it on the same arch as the target machine).

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
If you get `permission denied` for `/certs/ca.key` in the control‑plane logs, fix cert permissions:
```
sudo FIX_PERMS=1 CHOWN_UID=65532 CHOWN_GID=65532 \
  OUT_DIR=/opt/hardwareops/certs ./scripts/setup-control-plane.sh
docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem restart control-plane
```

If curl reports `authority and subject key identifier mismatch`, reissue certs:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem restart gateway control-plane
```

## 3) Optional DNS (CoreDNS)
On the control‑plane VM:
```
DNS_IP=<CONTROL_PLANE_IP> ./scripts/run-coredns.sh
```

On each agent VM:
```
DNS_SERVER=<CONTROL_PLANE_IP> ./scripts/set-dns.sh
```

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
### `Could not resolve host: hardwareops.internal`
- DNS not set on the VM/host. On the **CP VM**, point resolver to localhost:
  ```
  DNS_SERVER=127.0.0.1 MODE=manual ./scripts/set-dns.sh
  ```
- On agent VMs, point DNS to the CP VM IP:
  ```
  DNS_SERVER=<CONTROL_PLANE_IP> MODE=manual ./scripts/set-dns.sh
  ```
- Avoid `.local` domains; use `.internal` to bypass mDNS conflicts.

### `SSL certificate problem: authority and subject key identifier mismatch`
You are using an old CA or mismatched server cert. Re‑issue certs on the CP VM:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem restart gateway control-plane
```
Then re‑copy `/opt/hardwareops/certs/ca.crt` to agents/hosts.

### `permission denied` for `/certs/ca.key` or `device.key`
- Control‑plane:
  ```
  sudo FIX_PERMS=1 CHOWN_UID=65532 CHOWN_GID=65532 \
    OUT_DIR=/opt/hardwareops/certs ./scripts/setup-control-plane.sh
  docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem restart control-plane
  ```
- Agent:
  ```
  sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
  sudo systemctl restart hardwareops-agent
  ```

### UI loads but API calls fail (NetworkError)
- Make sure you open **https://hardwareops.internal/** (not the IP).
- Ensure your host resolves `hardwareops.internal` to the CP VM IP.
- Import `/opt/hardwareops/certs/ca.crt` into the host trust store.

### `502 Bad Gateway` from nginx
Control‑plane is down or DB not ready:
```
docker compose -p hardwareops -f docker-compose.onprem.bundle.yml --env-file .env.onprem logs --tail=200 control-plane
docker compose -p hardwareops -f docker-compose.onprem.bundle.yml --env-file .env.onprem restart control-plane
```
## Troubleshooting
- If the stack bundle doesn’t extract into a folder, rebuild with the latest `build-installers.sh`.
- If images won’t load, check disk space and Docker status.
