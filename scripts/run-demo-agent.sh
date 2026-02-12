#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
CERT_DIR_BASE=${CERT_DIR:-/tmp/hardwareops-demo/certs}
DATA_DIR_BASE=${DATA_DIR:-/tmp/hardwareops-demo/data}
IMAGE_NAME=${IMAGE_NAME:-hardwareops-agent-demo}
CONTAINER_NAME=${CONTAINER_NAME:-hardwareops-demo-agent}
DEMO_HTTP_PORT=${DEMO_HTTP_PORT:-8081}
DEMO_AGENT_PORT_STEP=${DEMO_AGENT_PORT_STEP:-1000}
DEMO_COMPONENT_PORT_STEP=${DEMO_COMPONENT_PORT_STEP:-10}
DEMO_COMPONENTS=${DEMO_COMPONENTS:-agent_bundle}
NO_CACHE=${NO_CACHE:-0}
DEMO_COUNT=${DEMO_COUNT:-1}
DEMO_RESET=${DEMO_RESET:-0}
CERTS_RW=${CERTS_RW:-1}
ALLOW_UNSUPPORTED_APPLY=${ALLOW_UNSUPPORTED_APPLY:-1}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}
REQUIRE_ARTIFACT_SIGNATURE=${REQUIRE_ARTIFACT_SIGNATURE:-$SIGN_ARTIFACTS}
UPLOAD_AGENT_BUNDLE=${UPLOAD_AGENT_BUNDLE:-1}
AGENT_ARTIFACT_NAME=${AGENT_ARTIFACT_NAME:-agent}
AGENT_ARTIFACT_VERSION=${AGENT_ARTIFACT_VERSION:-0.1.0}

HOST_URL="$BASE_URL"
if [[ "$BASE_URL" == http:* ]]; then
  echo "Control-plane must be HTTPS for mTLS. Start with ENABLE_TLS=1 ./scripts/run-control-plane.sh" >&2
  exit 1
fi

CA_CERT_PATH=${CONTROL_PLANE_CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
if [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA cert not found at $CA_CERT_PATH. Run ./scripts/run-control-plane.sh first." >&2
  exit 1
fi

# Ensure signing keys are available for demo verification.
source "$BASE_DIR/scripts/ensure-signing-key.sh"

if [ "$UPLOAD_AGENT_BUNDLE" = "1" ]; then
  AGENT_BUNDLE_DIR="$BASE_DIR/.tmp/agent-bundle"
  mkdir -p "$AGENT_BUNDLE_DIR/files"
  cat > "$AGENT_BUNDLE_DIR/files/readme.txt" <<EOF
HardwareOps agent bundle demo
version=${AGENT_ARTIFACT_VERSION}
EOF
  cat > "$AGENT_BUNDLE_DIR/files/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "agent preapply ok ${ts}" > files/preapply.txt
echo "${ts}" > files/last_applied.txt
echo "${ts} artifact=${HWOPS_ARTIFACT_ID:-} version=${HWOPS_ARTIFACT_VERSION:-}" >> files/apply.log
pid_file="${HWOPS_ARTIFACT_ROOT}/heartbeat.pid"
log_file="${HWOPS_ARTIFACT_DIR}/files/heartbeat.log"
if [ -f "$pid_file" ]; then
  old_pid=$(cat "$pid_file" 2>/dev/null || true)
  if [ -n "${old_pid:-}" ] && kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
fi
( while true; do
    beat_ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "$beat_ts" >> "$log_file"
    sleep 10
  done ) >/dev/null 2>&1 &
echo $! > "$pid_file"
EOF
  chmod 0755 "$AGENT_BUNDLE_DIR/files/preapply.sh"
  cat > "$AGENT_BUNDLE_DIR/files/plan.yaml" <<'EOF'
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 60
EOF
  ARTIFACT_NAME="$AGENT_ARTIFACT_NAME" \
  ARTIFACT_VERSION="$AGENT_ARTIFACT_VERSION" \
  ARTIFACT_TYPE="agent_bundle" \
  INPUT_DIR="$AGENT_BUNDLE_DIR" \
  BASE_URL="$BASE_URL" \
  CA_CERT_PATH="$CA_CERT_PATH" \
  SIGNING_KEY="$SIGNING_KEY" \
  SIGNING_KEY_ID="$SIGNING_KEY_ID" \
  "$BASE_DIR/scripts/pack-upload-artifact.sh" >/dev/null
  echo "Uploaded agent bundle artifact ${AGENT_ARTIFACT_NAME}:${AGENT_ARTIFACT_VERSION}"
fi

curl_opts=(--cacert "$CA_CERT_PATH")
if ! curl -s "${curl_opts[@]}" "$HOST_URL/healthz" >/dev/null; then
  echo "Control-plane not reachable at $HOST_URL" >&2
  exit 1
fi

AGENT_URL=${AGENT_URL:-$BASE_URL}
AGENT_URL=${AGENT_URL/localhost/host.docker.internal}
AGENT_URL=${AGENT_URL/127.0.0.1/host.docker.internal}

build_args=()
if [ "$NO_CACHE" = "1" ]; then
  build_args+=(--no-cache)
fi
docker build "${build_args[@]}" -f "$BASE_DIR/agent/Dockerfile.demo" -t "$IMAGE_NAME" "$BASE_DIR"

if ! [[ "$DEMO_COUNT" =~ ^[0-9]+$ ]] || [ "$DEMO_COUNT" -lt 1 ]; then
  echo "DEMO_COUNT must be a positive integer." >&2
  exit 1
fi

for i in $(seq 1 "$DEMO_COUNT"); do
  suffix=""
  if [ "$DEMO_COUNT" -gt 1 ] && [ "$i" -gt 1 ]; then
    suffix="-$i"
  fi

  CERT_DIR="$CERT_DIR_BASE$suffix"
  DATA_DIR="$DATA_DIR_BASE$suffix"
  DEMO_PORT=$((DEMO_HTTP_PORT + (i - 1) * DEMO_AGENT_PORT_STEP))
  NAME="$CONTAINER_NAME$suffix"

  if docker ps -a --format '{{.Names}}' | grep -q "^$NAME$"; then
    if [ "$DEMO_RESET" = "1" ]; then
      docker rm -f "$NAME" >/dev/null
    else
      echo "Container $NAME already exists. Remove it with: docker rm -f $NAME (or set DEMO_RESET=1)" >&2
      exit 1
    fi
  fi

  if [ "$DEMO_RESET" = "1" ]; then
    rm -rf "$CERT_DIR" "$DATA_DIR"
  fi

  mkdir -p "$CERT_DIR" "$DATA_DIR"
  # Ensure the container user (uid 10001) can write to /data when bind-mounted.
  chmod 0777 "$DATA_DIR"

  DEVICE_KEY_PATH="$CERT_DIR/device.key"
  DEVICE_CSR_PATH="$CERT_DIR/device.csr"
  DEVICE_CERT_PATH="$CERT_DIR/device.crt"
  DEVICE_ID_PATH="$DATA_DIR/device-id"

  if [ ! -s "$DEVICE_CERT_PATH" ] || [ ! -s "$DEVICE_KEY_PATH" ]; then
    if [ ! -s "$DEVICE_CSR_PATH" ] || [ ! -s "$DEVICE_KEY_PATH" ]; then
      openssl req -newkey rsa:2048 -nodes \
        -keyout "$DEVICE_KEY_PATH" -out "$DEVICE_CSR_PATH" \
        -subj "/CN=hardwareops-device"
    fi

    token_resp=$(curl -sS "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/enrollments" \
      -H "Content-Type: application/json" \
      -d '{"expiresInSec":3600}' \
      -w $'\n%{http_code}')
    token_status=${token_resp##*$'\n'}
    TOKEN_JSON=${token_resp%$'\n'*}
    if [ -z "$TOKEN_JSON" ] || [ "$token_status" != "200" ]; then
      echo "Failed to create enrollment token (status=$token_status)." >&2
      if [ -n "$TOKEN_JSON" ]; then
        echo "$TOKEN_JSON" >&2
      fi
      exit 1
    fi

    TOKEN=$(python3 - <<'PY' "$TOKEN_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("token",""))
PY
)
    if [ -z "$TOKEN" ]; then
      echo "Failed to create enrollment token." >&2
      exit 1
    fi

    ENROLL_PAYLOAD=$(python3 - <<'PY' "$TOKEN" "$DEVICE_CSR_PATH"
import json, sys
print(json.dumps({"token": sys.argv[1], "csr": open(sys.argv[2]).read()}))
PY
)

    enroll_resp=$(curl -sS "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/devices/enroll" \
      -H "Content-Type: application/json" \
      -d "$ENROLL_PAYLOAD" \
      -w $'\n%{http_code}')
    enroll_status=${enroll_resp##*$'\n'}
    ENROLL_JSON=${enroll_resp%$'\n'*}
    if [ -z "$ENROLL_JSON" ] || [ "$enroll_status" != "200" ]; then
      echo "Enrollment failed (status=$enroll_status)." >&2
      if [ -n "$ENROLL_JSON" ]; then
        echo "$ENROLL_JSON" >&2
      fi
      exit 1
    fi

    python3 - <<'PY' "$ENROLL_JSON" "$DEVICE_CERT_PATH" "$DEVICE_ID_PATH"
import json, sys
resp=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(resp.get("certPem",""))
open(sys.argv[3],"w").write(resp.get("deviceId",""))
PY

    if [ ! -s "$DEVICE_CERT_PATH" ]; then
      echo "Enrollment failed; device cert not created." >&2
      exit 1
    fi
  fi

  cp -f "$CA_CERT_PATH" "$CERT_DIR/dev-ca.crt"
  chmod 0644 "$DEVICE_CERT_PATH" "$DEVICE_KEY_PATH" || true
  cp -f "$SIGNING_PUB" "$CERT_DIR/signing.pub"

  cert_mount_mode="ro"
  if [ "$CERTS_RW" = "1" ]; then
    cert_mount_mode="rw"
  fi

  docker run -d \
    --name "$NAME" \
    --network host \
    --add-host host.docker.internal:host-gateway \
    --privileged \
    --user 0:0 \
    -e CONTROL_PLANE_URL="$AGENT_URL" \
    -e DEVICE_CERT_PATH=/certs/device.crt \
    -e DEVICE_KEY_PATH=/certs/device.key \
    -e CONTROL_PLANE_CA_CERT_PATH=/certs/dev-ca.crt \
    -e SIGNING_PUB_KEY_PATH=/certs/signing.pub \
    -e SIGNING_KEY_ID="$SIGNING_KEY_ID" \
    -e REQUIRE_ARTIFACT_SIGNATURE="$REQUIRE_ARTIFACT_SIGNATURE" \
    -e LOG_EXPORT_ADDR="${LOG_EXPORT_ADDR:-}" \
    -e LOG_LEVEL="${LOG_LEVEL:-}" \
    -e DEMO_BASE_PORT="$DEMO_PORT" \
    -e DEMO_PORT_STEP="$DEMO_COMPONENT_PORT_STEP" \
    -e DEMO_COMPONENTS="$DEMO_COMPONENTS" \
    -e ALLOW_UNSUPPORTED_APPLY="$ALLOW_UNSUPPORTED_APPLY" \
    -v "$DATA_DIR:/data" \
    -v "$DEVICE_CERT_PATH:/certs/device.crt:${cert_mount_mode}" \
    -v "$DEVICE_KEY_PATH:/certs/device.key:ro" \
    -v "$CERT_DIR/dev-ca.crt:/certs/dev-ca.crt:ro" \
    -v "$CERT_DIR/signing.pub:/certs/signing.pub:ro" \
    "$IMAGE_NAME" >/dev/null

  echo "Demo agent running as $NAME."
  echo "Service URLs:"
  IFS=',' read -r -a component_list <<<"$DEMO_COMPONENTS"
  idx=0
  for comp in "${component_list[@]}"; do
    comp=$(echo "$comp" | xargs)
    [ -z "$comp" ] && continue
    port=$((DEMO_PORT + idx * DEMO_COMPONENT_PORT_STEP))
    echo "  ${comp}: http://localhost:${port}/index.html"
    idx=$((idx + 1))
  done
  if [ -s "$DEVICE_ID_PATH" ]; then
    echo "Device ID: $(cat "$DEVICE_ID_PATH")"
  fi
done
