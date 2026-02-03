# HardwareOps
The platform acts as a control plane for safely deploying software and configuration to autonomous devices operating in unreliable, bandwidth-constrained, and sometimes offline environments.

## Local Development

### Prereqs
- Go 1.22+
- Docker + Docker Compose
- python3 (for artifact packaging script)

### Start dependencies
1) Create env:
   `cp deploy/compose/.env.example deploy/compose/.env`
2) Start Postgres + MinIO:
   `make dev-up`

### Apply DB migrations
Preferred (automated):
```
./scripts/migrate.sh
```

Manual (psql): apply all migrations in order.
```
for f in control-plane/migrations/*.sql; do
  psql "$DATABASE_URL" -f "$f"
done
```

### Create a dev CA (for device enrollment)
The control-plane signs device CSRs when `CA_CERT_PATH` and `CA_KEY_PATH` are set.

Example (openssl):
```
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ./dev-ca.key -out ./dev-ca.crt \
  -days 365 -subj "/CN=HardwareOps Dev CA"
```

Then set:
- `CA_CERT_PATH=./dev-ca.crt`
- `CA_KEY_PATH=./dev-ca.key`

### Enable TLS + mTLS (required for device traffic)
Run the control-plane with HTTPS and verify client certs (mTLS). The device cert issued at enrollment is used to authenticate on every request.

```
ENABLE_TLS=1 ./scripts/run-control-plane.sh
```

Manual (set server cert/key and client CA explicitly):
```
export TLS_CERT_PATH=./dev-server.crt
export TLS_KEY_PATH=./dev-server.key
export TLS_CLIENT_CA_PATH=./dev-ca.crt
```

`ENABLE_TLS=1` generates a dev server cert signed by the dev CA (so the same CA can validate both server and device certs).
Default paths:
- Dev CA: `./dev-ca.crt` + `./dev-ca.key`
- Server cert: `./dev-server.crt` + `./dev-server.key`
The dev server cert includes SANs for `localhost`, `127.0.0.1`, and `host.docker.internal`.

### HTTPS Reverse Proxy (Caddy + automatic certs)
Use Caddy in front of the control-plane for automatic HTTPS (Let’s Encrypt). This is recommended for public domains.

1) Configure the domain + email:
```
cp deploy/compose/.env.example deploy/compose/.env
# edit CADDY_DOMAIN and CADDY_EMAIL
```
Ensure `CADDY_CLIENT_CA_PATH` points to the device CA (same CA used by the control-plane to sign device certs). Default is `../dev-ca.crt`.

2) Start Caddy:
```
./scripts/run-proxy.sh
```
The proxy script will use `deploy/compose/.env` if present and default `CADDY_CLIENT_CA_PATH` to `./dev-ca.crt`.

3) Start the control-plane behind the proxy (trust the proxy client cert header):
```
USE_PROXY=1 ./scripts/run-control-plane.sh
```

4) Point agents to the HTTPS domain:
```
export CONTROL_PLANE_URL=https://your-domain.example
```

Note: Automatic certs require a publicly reachable domain. For local dev, set `CADDY_TLS=internal` in `deploy/compose/.env`
and trust Caddy’s internal CA.

#### Reverse proxy verification (curl)
These commands validate the proxy + mTLS chain end-to-end. Replace `your-domain.example` with your domain.

Health check (proxy TLS):
```
curl -s https://your-domain.example/healthz
```

Enrollment via proxy:
```
curl -s -X POST https://your-domain.example/api/v1/enrollments \
  -H "Content-Type: application/json" \
  -d '{"expiresInSec":3600}' > /tmp/enrollments.json
```

Enroll a device (returns device cert + CA):
```
openssl req -newkey rsa:2048 -nodes \
  -keyout /tmp/device.key -out /tmp/device.csr \
  -subj "/CN=hardwareops-device"

TOKEN=$(python3 - <<'PY'
import json
print(json.load(open("/tmp/enrollments.json"))["token"])
PY
)

CSR=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' /tmp/device.csr)
curl -s -X POST https://your-domain.example/api/v1/devices/enroll \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$TOKEN\",\"csr\":\"$CSR\"}" > /tmp/enroll.json
```

Device check-in via mTLS (proxy forwards client cert):
```
DEVICE_ID=$(python3 - <<'PY'
import json
print(json.load(open("/tmp/enroll.json"))["deviceId"])
PY
)

python3 - <<'PY'
import json
data=json.load(open("/tmp/enroll.json"))
open("/tmp/device.crt","w").write(data["certPem"])
open("/tmp/dev-ca.crt","w").write(data["caCertPem"])
PY

curl -s --cert /tmp/device.crt --key /tmp/device.key \
  https://your-domain.example/api/v1/devices/checkin \
  -H "Content-Type: application/json" \
  -d "{\"deviceId\":\"$DEVICE_ID\",\"agentVersion\":\"0.1.0\",\"current\":{\"softwareVersion\":\"v1\",\"configRev\":\"c1\"}}"
```

If TLS is enabled, use `https://` in clients and provide the CA:
```
curl --cacert ./dev-ca.crt https://localhost:8080/healthz
```

### Run the control-plane
From `control-plane/`:
```
export DATABASE_URL=postgres://hardwareops:hardwareops@localhost:5432/hardwareops?sslmode=disable
export CA_CERT_PATH=../dev-ca.crt
export CA_KEY_PATH=../dev-ca.key
export AUTO_MIGRATE=1

# S3/MinIO (endpoint is host:port, no scheme)
export S3_ENDPOINT=localhost:9000
export S3_BUCKET=artifacts
export S3_ACCESS_KEY=minio
export S3_SECRET_KEY=minio123
export S3_USE_SSL=0
export S3_PRESIGN_TTL=5m

# Rate limits (per IP, per minute; set to 0 to disable)
export ENROLLMENT_TOKEN_RPM=30
export ENROLL_RPM=60
export CHECKIN_RPM=300
export APPLY_RESULT_RPM=300

# Optional: change migrations dir
# export MIGRATIONS_DIR=./migrations

go run ./cmd/control-plane
```

### Run the agent (periodic check-in)
From `agent/`:
```
export CONTROL_PLANE_URL=https://localhost:8080
export ARTIFACT_ROOT=./agent-data
export CHECKIN_INTERVAL=30s
export DEVICE_CERT_PATH=/tmp/device.crt
export DEVICE_KEY_PATH=/tmp/device.key
export CONTROL_PLANE_CA_CERT_PATH=/tmp/dev-ca.crt

go run ./cmd/agent
```

Single check-in (one-shot):
```
go run ./cmd/agent -once
```
Device check-ins require mTLS; run the control-plane with TLS enabled.

### Device Simulator
The simulator enrolls devices (token + CSR) and performs periodic check-ins.

Examples (from `agent/`):
```
# 10 simulated devices, check in every 5 seconds

go run ./cmd/simulator --devices 10 --interval 5s

# Single check-in per device

go run ./cmd/simulator --devices 5 --once

# Skip enrollment (random UUIDs) if CA is not configured

go run ./cmd/simulator --devices 5 --skip-enroll

# HTTPS + mTLS (use CA to verify server cert)

go run ./cmd/simulator --base-url https://localhost:8080 --ca-cert /tmp/dev-ca.crt
```

### Artifacts (v1)
Supported artifact type: **tar.gz bundle** with a `manifest.json` and a `files/` directory.

Bundle layout:
```
manifest.json
files/
  <your files>
```

The `manifest.json` is auto-generated by the packaging script.

#### Package an artifact (local build)
```
./scripts/artifact-pack.py \
  --name agent \
  --version 1.0.0 \
  --input-dir ./build \
  --out /tmp/agent-1.0.0.tar.gz
```

The script prints `sha256` and `sizeBytes` for the bundle.

#### Upload + register (control-plane)
Use the upload endpoint to store the artifact in MinIO and register it in the DB:
```
curl -s -X POST http://localhost:8080/api/v1/artifacts/upload \
  -F "name=agent" \
  -F "version=1.0.0" \
  -F "file=@/tmp/agent-1.0.0.tar.gz"
```
Presigned download URLs are short-lived by default (5 minutes). Adjust with `S3_PRESIGN_TTL`. Agents only receive presigned URLs (no MinIO credentials).

#### Set desired state for device/group
```
DEVICE_ID=$(uuidgen)

curl -s -X PUT http://localhost:8080/api/v1/desired-state/devices/$DEVICE_ID \
  -H "Content-Type: application/json" \
  -d '{"desiredVersion":"1.0.0","artifactId":"<artifactId>"}'
```

The agent will download, verify, extract, and atomically switch `ARTIFACT_ROOT/current` to the new version.
If apply fails, the symlink remains on the previous version. You can rollback by setting desired state to the prior version/artifact.

#### End-to-end artifact test
Assumes `/tmp/agent-0.0.1.tar.gz` exists:
```
./scripts/artifact-e2e.sh
```

### Curl Quickstart
These commands hit the v1 API to validate the control-plane is responding.
Device check-in requires TLS + client certs (mTLS).

1) Health check (verifies API is up):
```
curl -s --cacert ./dev-ca.crt https://localhost:8080/healthz
```

2) Create enrollment token (returns a one-time token used to enroll a device):
```
curl -s --cacert ./dev-ca.crt -X POST https://localhost:8080/api/v1/enrollments \
  -H "Content-Type: application/json" \
  -d '{"expiresInSec":3600}'
```

3) Generate a CSR (simulates a device keypair + CSR):
```
openssl req -newkey rsa:2048 -nodes \
  -keyout /tmp/device.key -out /tmp/device.csr \
  -subj "/CN=device-1"
```

4) Enroll device (exchanges token + CSR for device cert):
```
TOKEN="paste_token_here"
CSR=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' /tmp/device.csr)

curl -s --cacert ./dev-ca.crt -X POST https://localhost:8080/api/v1/devices/enroll \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$TOKEN\",\"csr\":\"$CSR\"}"
```

Save the returned device cert and CA for mTLS:
```
curl -s --cacert ./dev-ca.crt -X POST https://localhost:8080/api/v1/devices/enroll \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$TOKEN\",\"csr\":\"$CSR\"}" > /tmp/enroll.json

python3 - <<'PY'
import json
data=json.load(open("/tmp/enroll.json"))
open("/tmp/device.crt","w").write(data["certPem"])
open("/tmp/dev-ca.crt","w").write(data["caCertPem"])
PY
```

5) Device check-in (posts device heartbeat/state to persist):
```
DEVICE_ID=$(python3 - <<'PY'
import json
print(json.load(open("/tmp/enroll.json"))["deviceId"])
PY
)

curl -s --cacert ./dev-ca.crt --cert /tmp/device.crt --key /tmp/device.key \
  -X POST https://localhost:8080/api/v1/devices/checkin \
  -H "Content-Type: application/json" \
  -d "{\"deviceId\":\"$DEVICE_ID\",\"agentVersion\":\"0.1.0\",\"current\":{\"softwareVersion\":\"v1\",\"configRev\":\"c1\"}}"
```

Automated curl quickstart:
```
./scripts/curl-quickstart.sh
```

TLS-enabled quickstart:
```
BASE_URL=https://localhost:8080 CA_CERT_PATH=./dev-ca.crt ./scripts/curl-quickstart.sh
```

### Desired State
- The control-plane can set desired state via PUT endpoints.
- If no desired state exists, the agent's check-in will set desired to its current state (source=agent).
- Manual PUTs override agent-set desired state.
- Group desired state applies when a device's labels match a group selector (manual device overrides group).
- If `desiredVersion` is empty and you're not updating policy/check-in interval, `artifactId` is required.
- Device labels for group matching can be sent in the check-in payload as `labels` or updated via `PATCH /devices/{deviceId}`.
- Agents report apply results via `POST /api/v1/devices/{deviceId}/apply-result`.
- You can set `checkinIntervalSec` on device or group desired state to control agent check-in cadence (device-level overrides group).

Examples:
```
GROUP_ID=$(uuidgen)
DEVICE_ID=$(uuidgen)

curl -s -X PUT http://localhost:8080/api/v1/groups/$GROUP_ID \
  -H "Content-Type: application/json" \
  -d '{"name":"canary","selector":{"region":"west"}}'

curl -s -X PATCH http://localhost:8080/api/v1/devices/$DEVICE_ID \
  -H "Content-Type: application/json" \
  -d '{"labels":{"region":"west","role":"edge"}}'

curl -s -X PUT http://localhost:8080/api/v1/desired-state/groups/$GROUP_ID \
  -H "Content-Type: application/json" \
  -d '{"desiredVersion":"v2","desiredConfigRev":"c2","checkinIntervalSec":30}'

curl -s -X PUT http://localhost:8080/api/v1/desired-state/devices/$DEVICE_ID \
  -H "Content-Type: application/json" \
  -d '{"desiredVersion":"v3","artifactId":"<artifactId>","checkinIntervalSec":15}'

curl -s http://localhost:8080/api/v1/desired-state
```

### Scripts
- `./scripts/dev-setup.sh` runs `make dev-up` and applies migrations.
- `./scripts/migrate.sh` applies SQL migrations via `go run ./cmd/migrate`.
- `./scripts/run-control-plane.sh` creates dev CA + CSR, auto-migrates, and runs the control-plane.
- `./scripts/curl-quickstart.sh` runs the curl quickstart sequence.
- `./scripts/dev-reset.sh` resets local dev state.
- `./scripts/artifact-e2e.sh` runs end-to-end artifact flow (upload → register → desired → agent apply).
- `./scripts/fail-artifact.sh` creates a large (>=5GB) artifact with a bad manifest, attempts an update, and verifies rollback behavior.
- `./scripts/run-agents.sh` runs multiple agent containers (defaults to 3).
- `./scripts/run-proxy.sh` runs the Caddy reverse proxy (HTTPS + automatic certs).

### Recommended Local Flow (Success Path)
1) `./scripts/dev-setup.sh`
2) `./scripts/run-control-plane.sh`
3) `./scripts/curl-quickstart.sh`

### Agent Containers (multi-agent)
Spin up multiple agents in isolated Docker containers:
```
./scripts/run-agents.sh -d
```

Build and run a single agent container directly:
```
docker build -f agent/Dockerfile -t hardwareops-agent .
docker run --rm \
  -e CONTROL_PLANE_URL=https://host.docker.internal:8080 \
  -e CHECKIN_INTERVAL=30s \
  -v agent-data:/data \
  hardwareops-agent
```

With mTLS, mount the device cert/key and control-plane CA:
```
docker run --rm \
  -e CONTROL_PLANE_URL=https://host.docker.internal:8080 \
  -e DEVICE_CERT_PATH=/certs/device.crt \
  -e DEVICE_KEY_PATH=/certs/device.key \
  -e CONTROL_PLANE_CA_CERT_PATH=/certs/dev-ca.crt \
  -v /tmp/device.crt:/certs/device.crt:ro \
  -v /tmp/device.key:/certs/device.key:ro \
  -v /tmp/dev-ca.crt:/certs/dev-ca.crt:ro \
  -v agent-data:/data \
  hardwareops-agent
```

Scale count (default 3):
```
AGENT_COUNT=5 ./scripts/run-agents.sh -d
```

Override the agent check-in interval:
```
CHECKIN_INTERVAL=10s ./scripts/run-agents.sh -d
```

Run agents with mTLS (mount device cert/key + CA):
```
MTLS=1 \
DEVICE_CERT_PATH=/tmp/device.crt \
DEVICE_KEY_PATH=/tmp/device.key \
CONTROL_PLANE_CA_CERT_PATH=/tmp/dev-ca.crt \
./scripts/run-agents.sh -d
```

`./scripts/run-agents.sh` will auto-enroll a device and write certs to `/tmp/hardwareops/device.crt` and `/tmp/hardwareops/device.key`
if they do not already exist.

When using the Caddy reverse proxy, point agents to the proxy domain:
```
CONTROL_PLANE_URL=https://your-domain.example ./scripts/run-agents.sh -d
```

The agents will check in periodically, apply desired state if present, and stay running.
`./scripts/run-agents.sh` auto-enrolls a dev device using the local control-plane and mounts the certs into the containers.
If you previously exported `CONTROL_PLANE_URL=http://...`, unset it or set it to `https://...` before running the script.
List device IDs from the control-plane to set desired state:
```
curl -s http://localhost:8080/api/v1/devices
```

If your control-plane runs elsewhere, override the URL:
```
CONTROL_PLANE_URL=http://host.docker.internal:8080 ./scripts/run-agents.sh -d
```

### Endpoints (v1)
- `POST /api/v1/enrollments` create enrollment token
- `POST /api/v1/devices/enroll` exchange token + CSR for device cert
- `POST /api/v1/devices/checkin` device heartbeat + state
- `POST /api/v1/devices/{deviceId}/apply-result` agent apply result (success/error)
- `GET /api/v1/devices` list devices
- `GET /api/v1/devices/{deviceId}` device detail
- `PATCH /api/v1/devices/{deviceId}` update device labels/metadata
- `GET /api/v1/groups` list groups
- `PUT /api/v1/groups/{groupId}` create/update group selector
- `GET /api/v1/desired-state` list desired state
- `PUT /api/v1/desired-state/groups/{groupId}` set desired for group
- `PUT /api/v1/desired-state/devices/{deviceId}` set desired for device
- `POST /api/v1/artifacts` register artifact
- `POST /api/v1/artifacts/upload` upload + register artifact
- `GET /api/v1/artifacts` list artifacts
- `GET /api/v1/artifacts/{artifactId}` artifact detail
- `POST /api/v1/artifacts/{artifactId}/presign` presigned download URL
- `GET /healthz`

### Notes
- `DATABASE_URL` is required for the control-plane to start.
- If `AUTO_MIGRATE=1` is set, migrations are applied at startup.
- If `CA_CERT_PATH` / `CA_KEY_PATH` are not set, `/api/v1/devices/enroll` returns an error.
- Agents default to a 30s check-in interval; override with `CHECKIN_INTERVAL=15s` or `CHECKIN_INTERVAL_SEC=15`.
- TLS is enabled when `TLS_CERT_PATH` and `TLS_KEY_PATH` are set. Client certs are verified against `TLS_CLIENT_CA_PATH` (defaults to `CA_CERT_PATH`).
- When mTLS is enabled, device identity is derived from the client cert fingerprint; `deviceId` in the payload must match (or can be omitted).
- If running behind the reverse proxy, set `TRUST_PROXY=1` (the proxy forwards the client cert via `X-Client-Cert`).
- Enrollment CSR validation: CN required, no wildcard CN/SANs, DNS/IP SANs only (no URI/email SANs).
- Rate limits (per IP, per minute): `ENROLLMENT_TOKEN_RPM`, `ENROLL_RPM`, `CHECKIN_RPM`, `APPLY_RESULT_RPM` (set to `0` to disable).

### Dev Reset
Clean your dev setup (stops compose, removes volumes, and clears local artifacts):
```
./scripts/dev-reset.sh
```
