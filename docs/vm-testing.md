# VM Test Setup (Control‑Plane + Agents)

Canonical deployment flow: `deploy.md`  
Use this file for VM-specific validation workflows.

This guide shows how to validate the **on‑prem install flow** without fresh hardware by using VMs.

## VirtualBox Two‑VM Setup (Windows host + WSL build)
This is the **single recommended flow** for your environment (Windows host, WSL2, Ubuntu 22.04 VMs).

### 1) Build installers on WSL
From the repo root:
```
./scripts/build-installers.sh
```
Bundles are written to:
```
dist/installers/<version>/
```
You should see:
- `control-plane-<version>-linux-amd64.tar.gz`
- `hardwareops-agent-<version>-linux-amd64.tar.gz`

If the bundle doesn’t contain a top‑level folder when extracted, rebuild with the latest script.

### 2) Enable SSH in both VMs
On each VM:
```
sudo apt-get update
sudo apt-get install -y openssh-server
sudo systemctl enable --now ssh
```
Find each VM IP:
```
ip a
```

### 3) Copy bundles to each VM (from WSL)
```
scp dist/installers/<version>/control-plane-<version>-linux-amd64.tar.gz <user>@<cp_vm_ip>:~/
scp dist/installers/<version>/hardwareops-agent-<version>-linux-amd64.tar.gz <user>@<agent_vm_ip>:~/
```

### 4) Control‑plane VM setup
Install Docker:
```
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo $VERSION_CODENAME) stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER
newgrp docker
```

Clone the repo (needed for compose + scripts):
```
git clone <YOUR_REPO_URL> hardwareops
cd hardwareops
```

Create CA + server cert:
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

Start the stack:
```
cp deploy/compose/.env.onprem.example deploy/compose/.env.onprem
sed -i 's|CERTS_DIR=.*|CERTS_DIR=/opt/hardwareops/certs|g' deploy/compose/.env.onprem
sed -i 's|PUBLIC_BASE_URL=.*|PUBLIC_BASE_URL=https://hardwareops.internal|g' deploy/compose/.env.onprem

docker compose -f deploy/compose/docker-compose.onprem.yml --env-file deploy/compose/.env.onprem up -d
```

### 5) Agent VM setup
Add DNS entry to resolve `hardwareops.internal`:
```
echo "<CONTROL_PLANE_IP> hardwareops.internal" | sudo tee -a /etc/hosts
```
If you want a real DNS server instead of `/etc/hosts`, run CoreDNS on the control‑plane VM:
```
DNS_IP=<CONTROL_PLANE_IP> ./scripts/run-coredns.sh
```
Then set the **agent VM’s** DNS server to the control‑plane VM IP (see `dns-coredns.md`).
Example:
```
DNS_SERVER=<CONTROL_PLANE_IP> ./scripts/set-dns.sh
```
On the control‑plane VM itself, either use `/etc/hosts` for local checks:
```
echo "127.0.0.1 hardwareops.internal" | sudo tee -a /etc/hosts
```
…or point the resolver at localhost to use CoreDNS:
```
DNS_SERVER=127.0.0.1 MODE=manual ./scripts/set-dns.sh
```
If localhost queries still fail, run CoreDNS with host networking:
```
DNS_IP=<CONTROL_PLANE_IP> HOST_NET=1 ./scripts/run-coredns.sh
```

Create cert directory and copy CA cert from control‑plane VM:
```
sudo mkdir -p /opt/hardwareops/certs
```
From WSL or the control‑plane VM:
```
scp /opt/hardwareops/certs/ca.crt <user>@<agent_vm_ip>:/opt/hardwareops/certs/ca.crt
```

Create an enrollment profile in the UI and copy the bootstrap token, then install + start the agent:
```
tar -xf hardwareops-agent-<version>-linux-amd64.tar.gz
cd hardwareops-agent-<version>-linux-amd64

sudo ./scripts/agent-install.sh \
  AGENT_SRC=./hardwareops-agent \
  CONTROL_PLANE_URL=https://agent.hardwareops.internal \
  CONTROL_PLANE_CA_CERT_SRC=/opt/hardwareops/certs/ca.crt \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
  START_SERVICE=1
```

Approve the pending request in `Security -> Pending enrollments`, then confirm the same service writes
`/etc/hardwareops/agent/certs/device.crt` and starts normal mTLS check-ins.

Verify from the control‑plane VM:
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/api/v1/devices
```

### 1) Install Multipass (Windows PowerShell)
```
winget install Canonical.Multipass
```

### 2) Ensure Hyper-V is enabled (PowerShell, admin)
```
Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
```
Reboot if prompted.

### 3) Create VMs (PowerShell)
```
multipass launch 22.04 --name hwops-cp --cpus 4 --memory 6G --disk 40G
multipass launch 22.04 --name hwops-agent-1 --cpus 2 --memory 2G --disk 20G
```

### 4) Shell into VMs
```
multipass shell hwops-cp
# in a new terminal
multipass shell hwops-agent-1
```

Notes:
- If you are on Windows Home (no Hyper-V), use VirtualBox instead.
- Easiest workflow is to `git clone` the repo inside the control-plane VM.

## Appendix: Single‑VM Quick Test
If you want a minimal test, see the earlier version of this doc in git history or ask and I’ll re‑add it.

## What this validates
- Installer bundle flow
- CA + TLS trust
- Enrollment + mTLS check‑in
- Systemd service behavior

## Notes
- Use **Bridged networking** in VirtualBox so the VMs get LAN IPs.
- If you use NAT, ensure the agent VM can reach the control‑plane VM IP.

## Troubleshooting
### UI loads but API calls fail
- Ensure you browse **https://hardwareops.internal/**, not the IP.
- Ensure host DNS resolves `hardwareops.internal` to the CP VM IP.
- Import `/opt/hardwareops/certs/ca.crt` into the host trust store.

### `Could not resolve host` on curl
- On CP VM, set resolver to localhost:
  ```
  DNS_SERVER=127.0.0.1 MODE=manual ./scripts/set-dns.sh
  ```
- On agent VM, set resolver to CP VM IP:
  ```
  DNS_SERVER=<CONTROL_PLANE_IP> MODE=manual ./scripts/set-dns.sh
  ```

### TLS mismatch (`authority and subject key identifier mismatch`)
Re‑issue certs on CP VM and re‑copy CA to agents:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```
