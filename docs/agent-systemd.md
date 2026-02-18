# Agent Deployment (systemd)

Canonical deployment flow: `deploy.md`  
Use this file for agent-specific install/enroll service details.

This installs the agent as a systemd service.
If you need the binary for a specific OS/arch, build the installer bundles first:
```
./scripts/build-installers.sh
```
See `installers.md`.

## 1) Install the binary
Copy the compiled agent binary onto the device (example path: `/tmp/hardwareops-agent`).

Install prerequisites (Ubuntu):
```
sudo ./scripts/install-agent-deps-ubuntu.sh
```

## 2) Install service + config
```
sudo AGENT_SRC=/tmp/hardwareops-agent ./scripts/agent-install.sh
```

This creates:
- `/etc/hardwareops/agent/agent.env`
- `/etc/systemd/system/hardwareops-agent.service`
- `/var/lib/hardwareops/agent/`

Edit `/etc/hardwareops/agent/agent.env` and set:
```
CONTROL_PLANE_URL=https://hardwareops.internal
CONTROL_PLANE_CA_CERT_PATH=/etc/hardwareops/agent/certs/ca.crt
AUTO_REENROLL=1
```

## 3) Enroll the device
```
sudo CONTROL_PLANE_URL=https://hardwareops.internal \
     CA_CERT_PATH=/opt/hardwareops/certs/ca.crt \
     ./scripts/agent-enroll.sh
```

If you see `permission denied` for `device.key` in the agent logs, fix ownership:
```
sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
sudo systemctl restart hardwareops-agent
```

This writes:
- `/etc/hardwareops/agent/certs/device.crt`
- `/etc/hardwareops/agent/certs/device.key`
- `/etc/hardwareops/agent/certs/ca.crt`
- `/var/lib/hardwareops/agent/device-id`

## 4) Start the service
```
sudo systemctl restart hardwareops-agent
sudo systemctl status hardwareops-agent
```

## 5) Verify
On the control-plane:
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/api/v1/devices
```

## Troubleshooting
### `permission denied` on `device.key`
```
sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
sudo systemctl restart hardwareops-agent
```

### TLS mismatch (`authority and subject key identifier mismatch`)
Re‑copy the latest CA cert from the control‑plane:
```
scp /opt/hardwareops/certs/ca.crt <user>@<agent_vm_ip>:/opt/hardwareops/certs/ca.crt
```
