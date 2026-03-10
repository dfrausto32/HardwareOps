# Agent Deployment (systemd)

Canonical deployment flow: `deploy.md`  
Use this file for the packaged Linux agent install path, including first-contact approval onboarding.

This is the production path for agents that start without a pre-shipped client certificate.
The same installed service stays up while waiting for approval, writes `device.crt` plus `device-id`
when the request is approved, and then continues into the normal mTLS check-in loop.

## Variables used below

Operator host:

```bash
export PUBLIC_BASE_URL=https://hardwareops.internal
export AGENT_BASE_URL=https://agent.hardwareops.internal
export CA_CERT=/opt/hardwareops/certs/ca.crt
```

Agent host:

```bash
export CONTROL_PLANE_URL=https://agent.hardwareops.internal
export CONTROL_PLANE_CA_CERT_SRC=/opt/hardwareops/certs/ca.crt
```

Use `AGENT_BASE_URL` for agents. `PUBLIC_BASE_URL` is for the UI/operator API.

## 1) Build or extract the bundle

From the repo root:

```bash
./scripts/build-installers.sh
```

Then copy the Linux bundle to the target host and extract it:

```bash
tar -xzf hardwareops-agent-<version>-linux-<arch>.tar.gz
cd hardwareops-agent-<version>-linux-<arch>
```

Install Ubuntu package prerequisites on the agent host:

```bash
sudo ./scripts/install-agent-deps-ubuntu.sh
```

## 2) Establish trust on the agent host

Copy the control-plane trust anchor to the agent host before starting the service:

```bash
sudo mkdir -p /opt/hardwareops/certs
sudo install -m 0644 /path/to/ca.crt /opt/hardwareops/certs/ca.crt
```

If your deployment uses a publicly trusted server certificate instead of a private CA,
the trust-anchor copy step is optional. Run the installer later with `USE_SYSTEM_CA=1`
so it removes `CONTROL_PLANE_CA_CERT_PATH` from `agent.env` and relies on the host trust store.

## 3) Create an enrollment profile and distribute the bootstrap token

UI path:
- Open `PUBLIC_BASE_URL`
- Go to `Security -> Enrollment profiles`
- Create a profile with `Require approval` enabled
- Copy the generated bootstrap token once and deliver it to the operator installing the agent

API path:

```bash
AUTH_TOKEN=$(
  curl --silent --show-error --fail --cacert "$CA_CERT" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@example.com","password":"change-me"}' \
    "$PUBLIC_BASE_URL/api/v1/auth/login" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])'
)

PROFILE_JSON=$(
  curl --silent --show-error --fail --cacert "$CA_CERT" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"factory-floor-a","expiresInSec":2592000,"maxUses":25,"requireApproval":true}' \
    "$PUBLIC_BASE_URL/api/v1/enrollment-profiles"
)

PROFILE_TOKEN=$(printf '%s\n' "$PROFILE_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["bootstrapToken"])')
printf 'Bootstrap token: %s\n' "$PROFILE_TOKEN"
```

Treat the bootstrap token like a secret. The UI/API only returns the raw token at creation or rotation time.

## 4) Install and configure the service for approval mode

Run the packaged installer on the agent host. The installer accepts `KEY=VALUE` arguments after the script
so `sudo` does not need environment passthrough configuration.

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./hardwareops-agent \
  CONTROL_PLANE_URL="$CONTROL_PLANE_URL" \
  CONTROL_PLANE_CA_CERT_SRC="$CONTROL_PLANE_CA_CERT_SRC" \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN="$PROFILE_TOKEN" \
  START_SERVICE=1
```

Public-CA variant:

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=./hardwareops-agent \
  CONTROL_PLANE_URL="$CONTROL_PLANE_URL" \
  USE_SYSTEM_CA=1 \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN="$PROFILE_TOKEN" \
  START_SERVICE=1
```

What this does:
- installs `/usr/local/bin/hardwareops-agent`
- writes `/etc/hardwareops/agent/agent.env`
- stages the control-plane CA at `/etc/hardwareops/agent/certs/ca.crt`
- enables and starts `hardwareops-agent.service`

Approval-mode defaults written into `agent.env`:
- `DEVICE_CERT_PATH=/etc/hardwareops/agent/certs/device.crt`
- `DEVICE_KEY_PATH=/etc/hardwareops/agent/certs/device.key`
- `DEVICE_ID_PATH=/var/lib/hardwareops/agent/device-id`
- `BOOTSTRAP_STATE_PATH=/var/lib/hardwareops/agent/bootstrap-state.json`

## 5) Confirm the service is waiting for approval

On the agent host:

```bash
sudo systemctl status hardwareops-agent --no-pager
sudo journalctl -u hardwareops-agent -f
```

Expected log sequence before approval:
- `pending enrollment requested request=<id>`
- `pending enrollment awaiting approval request=<id>`

The service should stay `active (running)` while those messages repeat.

## 6) Approve the request

UI path:
- Open `PUBLIC_BASE_URL`
- Go to `Security -> Pending enrollments`
- Match the request to the installing host
- Click `Approve`

API path:

```bash
REQUEST_ID=$(
  curl --silent --show-error --fail --cacert "$CA_CERT" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    "$PUBLIC_BASE_URL/api/v1/pending-enrollments?status=pending" \
  | python3 -c 'import json,sys; items=json.load(sys.stdin)["items"]; print(items[0]["requestId"] if items else "")'
)

curl --silent --show-error --fail --cacert "$CA_CERT" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' \
  "$PUBLIC_BASE_URL/api/v1/pending-enrollments/$REQUEST_ID/approve"
```

## 7) Verify the same installed service transitions into mTLS check-in

On the agent host:

```bash
sudo test -s /etc/hardwareops/agent/certs/device.crt
sudo test -s /etc/hardwareops/agent/certs/device.key
sudo cat /var/lib/hardwareops/agent/device-id
sudo test ! -e /var/lib/hardwareops/agent/bootstrap-state.json
```

Expected journal entries after approval:
- `approval bootstrap complete device=<uuid>`
- `check-in ok ...`

Operator-side verification:

```bash
DEVICE_ID=$(sudo cat /var/lib/hardwareops/agent/device-id)

curl --silent --show-error --fail --cacert "$CA_CERT" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  "$PUBLIC_BASE_URL/api/v1/devices/$DEVICE_ID"
```

Expected result:
- the device exists
- status is `active`
- subsequent check-ins continue without rerunning the installer

## Legacy direct enrollment

The direct token path is still available when you intentionally want the older flow:

```bash
sudo CONTROL_PLANE_URL="$CONTROL_PLANE_URL" \
     CA_CERT_PATH="$CA_CERT" \
     ./scripts/agent-enroll.sh
sudo systemctl restart hardwareops-agent
```

## Troubleshooting

### Service exits immediately in approval mode

Check `/etc/hardwareops/agent/agent.env` for:
- `CONTROL_PLANE_URL`
- `AGENT_ENROLL_MODE=approval`
- `ENROLLMENT_PROFILE_TOKEN`
- `CONTROL_PLANE_CA_CERT_PATH` if using a private CA

### Request never appears in the queue

Verify the agent can reach `AGENT_BASE_URL`, not `PUBLIC_BASE_URL`.
In split-host on-prem deployments the `agent.*` host handles runtime enrollment and mTLS traffic.

### `permission denied` on `device.key`

```bash
sudo chown -R hardwareops:hardwareops /etc/hardwareops/agent/certs
sudo systemctl restart hardwareops-agent
```

### TLS mismatch or untrusted server cert

Re-copy the latest control-plane CA and rerun the installer to rewrite `agent.env` if needed:

```bash
sudo ./scripts/agent-install.sh \
  AGENT_SRC=/usr/local/bin/hardwareops-agent \
  CONTROL_PLANE_URL="$CONTROL_PLANE_URL" \
  CONTROL_PLANE_CA_CERT_SRC="$CONTROL_PLANE_CA_CERT_SRC" \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN="$PROFILE_TOKEN"
```
