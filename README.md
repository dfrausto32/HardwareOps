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
export LOG_INGEST_ADDR=tcp://0.0.0.0:5560
export LOG_DIR=../logs

# Optional: stale device cleanup
export DEVICE_STALE_TTL=1h
export DEVICE_CLEANUP_INTERVAL=5m

# Optional: disable HTTP/2 (required for WebSocket support in dev)
export DISABLE_HTTP2=1

# CORS allowlist (required for browser access in realistic dev)
export CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173

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
Artifacts are **tar.gz bundles** with a `manifest.json` and a `files/` directory.
See `docs/artifact-apply-roadmap.md` for the firmware/container image apply plan and interfaces.

#### Artifact types (taxonomy)
All types share the same bundle format; the `type` field is used for policy/behavior:
- `app_bundle` (agent apply: **yes**)
- `config_bundle` (agent apply: **yes**)
- `data_bundle` (agent apply: **yes**)
- `firmware` (agent apply: **no** in v1, register only)
- `container_image` (agent apply: **no** in v1, register only)

If `type` is omitted, it defaults to `app_bundle`.

Optional `metadata` can be supplied when registering/uploading an artifact (any valid JSON).

Bundle layout:
```
manifest.json
plan.yaml
files/
  <your files>
```

The `manifest.json` is auto-generated by the packaging script.
`plan.yaml` is optional; if present in the input directory, the packer includes it at the bundle root and the agent validates it before apply.

Plan v1 (minimal) example:
```yaml
version: "v1"
health:
  type: probe.http
  url: http://localhost:8080/healthz
  expectStatus: 200
  timeoutSec: 30
  intervalSec: 2
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 120
  - id: render-config
    type: file.render
    onFail: rollback
    params:
      template: files/config/app.tmpl
      dest: /etc/app/config.yaml
  - id: health
    type: probe.http
    onFail: abort
    params:
      url: http://localhost:8080/healthz
```

#### Package an artifact (local build)
```
./scripts/artifact-pack.py \
  --name agent \
  --version 1.0.0 \
  --type app_bundle \
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
  -F "type=app_bundle" \
  -F 'metadata={"platform":"linux","arch":"amd64"}' \
  -F "file=@/tmp/agent-1.0.0.tar.gz"
```
Presigned download URLs are short-lived by default (5 minutes). Adjust with `S3_PRESIGN_TTL`. Agents only receive presigned URLs (no MinIO credentials).

Artifact API fields:
- `type`: one of the taxonomy values above (defaults to `app_bundle`).
- `metadata`: optional JSON blob with extra info (platform, hw targets, etc.).
  - `POST /api/v1/artifacts` expects `metadata` as JSON.
  - `POST /api/v1/artifacts/upload` expects `metadata` as a JSON string field.

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
Set `GENERATE_ARTIFACT=1` to build a demo artifact that includes `script.preApply` and a visible page:
```
GENERATE_ARTIFACT=1 ./scripts/artifact-e2e.sh
```
Set `ARTIFACT_TYPE=app_bundle` (or any valid type) to test type handling.

#### Upload one of each artifact type
```
./scripts/artifact-types.sh
```
By default this generates two versions per type (`0.1.0-<type>` and `0.2.0-<type>`) with distinct payloads.
Override with:
```
VERSIONS=0.1.0 TYPES=app_bundle,config_bundle ./scripts/artifact-types.sh
```
Each generated bundle includes an `index.html` so the demo agent can show a visible change when an
apply-capable type is selected (`app_bundle`, `config_bundle`, `data_bundle`).

### Logging (agent export + CSV)
Agents can export logs to the control-plane over a lightweight TCP stream (JSON lines). The control-plane writes per-device CSV files so they can be opened in Excel.

Control-plane env:
- `LOG_INGEST_ADDR` (default `tcp://0.0.0.0:5560`)
- `LOG_DIR` (default `./logs`)

Agent env:
- `LOG_EXPORT_ADDR` (example `tcp://localhost:5560`)
- `LOG_LEVEL` (`debug|info|warn|error`)

Download logs (CSV):
```
curl -s http://localhost:8080/api/v1/logs/<deviceId> -o /tmp/device-logs.csv
```

Tip: run test scripts with log export enabled:
```
LOG_EXPORT_ADDR=tcp://localhost:5560 ./scripts/artifact-e2e.sh
```

CSV columns:
`timestamp, level, component, deviceId, message, fields`

### Demo: Agent Container With Live Service
This demo runs the control-plane normally, starts an agent container that serves `index.html` from the active artifact, then updates the artifact so the page content changes.
For a full end-to-end walkthrough (including pre-apply), see `docs/preapply-demo.md`.

1. Start dependencies and the control-plane:
```
make dev-up
./scripts/run-control-plane.sh
```

2. Start the demo agent container (includes a small web server on port 8081):
```
./scripts/run-demo-agent.sh
```
The demo container runs with `--privileged` and as root for simplicity (demo-only).
`run-demo-agent.sh` sets `ALLOW_UNSUPPORTED_APPLY=1` by default so you can demo
`firmware`/`container_image` artifacts via the web page; set it to `0` to enforce
realistic behavior.
To run multiple demo agents, set `DEMO_COUNT` (ports auto-increment from `DEMO_HTTP_PORT`):
```
DEMO_COUNT=3 DEMO_HTTP_PORT=8081 ./scripts/run-demo-agent.sh
```

3. Build + upload two demo artifacts and flip desired state from v1 to v2:
```
./scripts/demo-artifacts.sh
```

4. Check the service output (you should see the page change after the update):
```
curl -s http://localhost:8081/index.html
```
The page now includes a **Pre-apply** line populated by `script.preApply`.

Environment overrides:
- `BASE_URL` sets the control-plane URL (default `https://localhost:8080`).
- `AGENT_URL` sets the control-plane URL as seen from the container (default replaces localhost with host.docker.internal).
- `CONTROL_PLANE_CA_CERT_PATH` sets the CA cert path for curl and enrollment.
- `DEMO_HTTP_PORT` sets the demo service port (default `8081`).
- `DATA_DIR` stores demo agent state and device ID (default `/tmp/hardwareops-demo/data`).
- `SLEEP_BETWEEN` sets delay between v1 and v2 desired updates (default `5` seconds).

To stop the demo container:
```
docker rm -f hardwareops-demo-agent
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

Cleanup (delete the device created by the quickstart run):
```
CLEANUP=1 BASE_URL=https://localhost:8080 CA_CERT_PATH=./dev-ca.crt ./scripts/curl-quickstart.sh
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
- `./scripts/curl-quickstart.sh` runs the curl quickstart sequence (set `CLEANUP=1` to delete the created device).
- `./scripts/dev-reset.sh` resets local dev state.
- `./scripts/artifact-e2e.sh` runs end-to-end artifact flow (upload → register → desired → agent apply). Set `CLEANUP=1` to delete the device + artifact.
- `./scripts/fail-artifact.sh` creates a large (>=5GB) artifact with a bad manifest, attempts an update, and verifies rollback behavior.
- `./scripts/artifact-types.sh` builds + uploads one artifact for each supported type.
- `./scripts/run-agents.sh` runs multiple agent containers (defaults to 3).
- `./scripts/run-demo-agent.sh` runs a demo agent container with a live web service.
- `./scripts/demo-artifacts.sh` builds and applies demo artifacts to show an update.
- `./scripts/run-proxy.sh` runs the Caddy reverse proxy (HTTPS + automatic certs).
- `./scripts/watch-events.sh` connects to the WebSocket event stream (requires `npx` or `websocat`).

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

Enable agent log export (ships logs to control-plane ingest):
```
LOG_EXPORT=1 ./scripts/run-agents.sh -d
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
- `DELETE /api/v1/devices/{deviceId}` delete device
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
- `DELETE /api/v1/artifacts/{artifactId}` delete artifact
- `POST /api/v1/artifacts/{artifactId}/presign` presigned download URL
- `GET /api/v1/logs/{deviceId}` download device logs (CSV)
- `GET /api/v1/events` WebSocket stream of device check-ins + apply results
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
- Enable log export by setting `LOG_EXPORT_ADDR` on agents and `LOG_INGEST_ADDR` on the control-plane.
- Stale device cleanup: `DEVICE_STALE_TTL` (default `1h`) and `DEVICE_CLEANUP_INTERVAL` (default `5m`).
- Cleanup uses `last_seen`; devices that never checked in are not auto-removed.

### Event Stream (WebSocket)
The control-plane broadcasts device check-ins and apply results on a WebSocket stream.

Note: the WebSocket handler requires HTTP/1.1. If you see `501` on connect, disable HTTP/2.

Example (self-signed TLS, skip verification):
```
npx wscat -c wss://localhost:8080/api/v1/events --no-check
```

Example (plain HTTP for local dev):
```
npx wscat -c ws://localhost:8080/api/v1/events
```

Scripted helper:
```
BASE_URL=https://localhost:8080 INSECURE=1 ./scripts/watch-events.sh
```

### UI: Realistic Dev Mode
To simulate production behavior, avoid the Vite proxy and use real TLS:
1) Trust the dev CA in your OS (or use Caddy with a trusted cert).
2) Set in `ui/.env`:
```
VITE_API_BASE_URL=https://localhost:8080
VITE_SIMULATE_PROD=1
```
3) Ensure the control-plane allows the UI origin:
```
export CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
```
3) Run:
```
cd ui
npm run dev
```

### Dev Reset
Clean your dev setup (stops compose, removes volumes, and clears local artifacts):
```
./scripts/dev-reset.sh
```
