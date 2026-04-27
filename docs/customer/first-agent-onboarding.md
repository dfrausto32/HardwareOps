# First Agent Onboarding

Use this guide to install the first Linux agent with approval-based onboarding.

## 1. Build or extract the Linux agent bundle

```bash
./scripts/build-installers.sh
```

Bundle output:

```text
dist/installers/<version>/parcel-agent-<version>-linux-<arch>.tar.gz
```

On the agent host:

```bash
tar -xzf parcel-agent-<version>-linux-<arch>.tar.gz
cd parcel-agent-<version>-linux-<arch>
sudo ./scripts/install-agent-deps-ubuntu.sh
```

## 2. Copy the control-plane CA

```bash
sudo mkdir -p /opt/parcel/certs
sudo install -m 0644 /path/to/ca.crt /opt/parcel/certs/ca.crt
```

If the agent endpoint uses a publicly trusted server certificate, this can be skipped and the installer can use `USE_SYSTEM_CA=1`.

## 3. Create an enrollment profile

In the UI:
- open `Security -> Enrollment profiles`
- create a profile with `Require approval` enabled
- copy the bootstrap token

## 4. Install the agent in approval mode

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./parcel-agent \
  CONTROL_PLANE_URL=https://agent.parcel.internal \
  CONTROL_PLANE_CA_CERT_SRC=/opt/parcel/certs/ca.crt \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
  START_SERVICE=1
```

Public CA variant:

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./parcel-agent \
  CONTROL_PLANE_URL=https://agent.parcel.internal \
  USE_SYSTEM_CA=1 \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
  START_SERVICE=1
```

## 5. Verify the pending request

On the agent host:

```bash
sudo systemctl status parcel-agent --no-pager
sudo journalctl -u parcel-agent -f
```

Expected messages:
- `pending enrollment requested`
- `pending enrollment awaiting approval`

## 6. Approve the request

In the UI:
- open `Security -> Pending enrollments`
- find the request
- click `Approve`

## 7. Verify steady-state check-in

On the agent host:

```bash
sudo test -s /etc/parcel/agent/certs/device.crt
sudo test -s /etc/parcel/agent/certs/device.key
sudo cat /var/lib/parcel/agent/device-id
sudo journalctl -u parcel-agent -n 50 --no-pager
```

Expected:
- device certificate and key exist
- device ID exists
- logs show normal `check-in ok`

## 8. Troubleshooting

- If the service stays pending forever, verify the enrollment profile token and approval step.
- If TLS fails, verify the CA certificate or public trust chain on the agent host.
- If the key file shows permission errors, fix ownership:

```bash
sudo chown -R parcel:parcel /etc/parcel/agent/certs
sudo systemctl restart parcel-agent
```
