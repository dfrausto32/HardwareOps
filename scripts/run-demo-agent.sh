#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
CERT_DIR_BASE=${CERT_DIR:-/tmp/hardwareops-demo/certs}
DATA_DIR_BASE=${DATA_DIR:-/tmp/hardwareops-demo/data}
IMAGE_NAME=${IMAGE_NAME:-hardwareops-agent-demo}
CONTAINER_NAME=${CONTAINER_NAME:-hardwareops-demo-agent}
DEMO_HTTP_PORT=${DEMO_HTTP_PORT:-8081}
NO_CACHE=${NO_CACHE:-0}
DEMO_COUNT=${DEMO_COUNT:-1}
ALLOW_UNSUPPORTED_APPLY=${ALLOW_UNSUPPORTED_APPLY:-1}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}
REQUIRE_ARTIFACT_SIGNATURE=${REQUIRE_ARTIFACT_SIGNATURE:-$SIGN_ARTIFACTS}

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
  DEMO_PORT=$((DEMO_HTTP_PORT + i - 1))
  NAME="$CONTAINER_NAME$suffix"

  if docker ps -a --format '{{.Names}}' | grep -q "^$NAME$"; then
    echo "Container $NAME already exists. Remove it with: docker rm -f $NAME" >&2
    exit 1
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

    TOKEN_JSON=$(curl -s "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/enrollments" \
      -H "Content-Type: application/json" \
      -d '{"expiresInSec":3600}')

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

    ENROLL_JSON=$(curl -s "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/devices/enroll" \
      -H "Content-Type: application/json" \
      -d "$ENROLL_PAYLOAD")

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
    -e DEMO_HTTP_PORT="$DEMO_PORT" \
    -e ALLOW_UNSUPPORTED_APPLY="$ALLOW_UNSUPPORTED_APPLY" \
    -v "$DATA_DIR:/data" \
    -v "$DEVICE_CERT_PATH:/certs/device.crt:ro" \
    -v "$DEVICE_KEY_PATH:/certs/device.key:ro" \
    -v "$CERT_DIR/dev-ca.crt:/certs/dev-ca.crt:ro" \
    -v "$CERT_DIR/signing.pub:/certs/signing.pub:ro" \
    "$IMAGE_NAME" >/dev/null

  echo "Demo agent running as $NAME."
  echo "Service URL: http://localhost:${DEMO_PORT}/index.html"
  if [ -s "$DEVICE_ID_PATH" ]; then
    echo "Device ID: $(cat "$DEVICE_ID_PATH")"
  fi
done
