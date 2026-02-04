#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

COUNT="${AGENT_COUNT:-1}"
BASE_FILE="deploy/compose/docker-compose.agents.yml"
MTLS_FILE="deploy/compose/docker-compose.agents.mtls.yml"

CERT_DIR=${CERT_DIR:-/tmp/hardwareops}
CA_CERT_DEFAULT="$BASE_DIR/dev-ca.crt"
DEVICE_KEY_DEFAULT="$CERT_DIR/device.key"
DEVICE_CSR_DEFAULT="$CERT_DIR/device.csr"
DEVICE_CERT_DEFAULT="$CERT_DIR/device.crt"

HTTP_FALLBACK=""
if [ -n "${CONTROL_PLANE_URL:-}" ]; then
  if [[ "$CONTROL_PLANE_URL" == http:* ]]; then
    HTTP_FALLBACK="$CONTROL_PLANE_URL"
    HOST_URL="${CONTROL_PLANE_URL/http:\/\//https:\/\/}"
    AGENT_URL="$HOST_URL"
  else
    HOST_URL="$CONTROL_PLANE_URL"
    AGENT_URL="$CONTROL_PLANE_URL"
  fi
else
  HOST_URL="https://localhost:8080"
  AGENT_URL="https://host.docker.internal:8080"
fi
CA_CERT_EXPLICIT=0
if [ -n "${CONTROL_PLANE_CA_CERT_PATH:-}" ]; then
  CA_CERT_PATH="$CONTROL_PLANE_CA_CERT_PATH"
  CA_CERT_EXPLICIT=1
else
  CA_CERT_PATH="$CA_CERT_DEFAULT"
fi
DEVICE_KEY_PATH=${DEVICE_KEY_PATH:-$DEVICE_KEY_DEFAULT}
DEVICE_CSR_PATH=${DEVICE_CSR_PATH:-$DEVICE_CSR_DEFAULT}
DEVICE_CERT_PATH=${DEVICE_CERT_PATH:-$DEVICE_CERT_DEFAULT}

mkdir -p "$CERT_DIR"

curl_opts=()
USE_CA_CERT=0
if [[ "$HOST_URL" == https:* ]]; then
  if [ "$CA_CERT_EXPLICIT" = "1" ]; then
    USE_CA_CERT=1
  else
    case "$HOST_URL" in
      *localhost*|*127.0.0.1*|*host.docker.internal*)
        USE_CA_CERT=1
        ;;
    esac
  fi
fi

if [ "$USE_CA_CERT" = "1" ]; then
  if [ ! -f "$CA_CERT_PATH" ]; then
    echo "CA cert not found at $CA_CERT_PATH. Run ./scripts/run-control-plane.sh first." >&2
    exit 1
  fi
  curl_opts+=(--cacert "$CA_CERT_PATH")
fi

if ! curl -s "${curl_opts[@]}" "$HOST_URL/healthz" >/dev/null; then
  if [ -n "$HTTP_FALLBACK" ]; then
    if curl -s "$HTTP_FALLBACK/healthz" >/dev/null; then
      echo "Control-plane is running over HTTP at $HTTP_FALLBACK. mTLS requires HTTPS." >&2
      echo "Start it with TLS (ENABLE_TLS=1 ./scripts/run-control-plane.sh) or use the proxy." >&2
      exit 1
    fi
  fi
  echo "Control-plane not reachable at $HOST_URL (check TLS/CA and that it is running)." >&2
  exit 1
fi

if [[ "$HOST_URL" != https:* ]]; then
  echo "CONTROL_PLANE_URL must be https:// for mTLS device check-ins." >&2
  exit 1
fi

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
data=json.loads(sys.argv[1])
print(data.get("token",""))
PY
)
  if [ -z "$TOKEN" ]; then
    echo "Failed to create enrollment token." >&2
    exit 1
  fi

  ENROLL_PAYLOAD=$(python3 - <<'PY' "$TOKEN" "$DEVICE_CSR_PATH"
import json, sys
token=sys.argv[1]
csr=open(sys.argv[2]).read()
print(json.dumps({"token": token, "csr": csr}))
PY
)

  ENROLL_JSON=$(curl -s "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/devices/enroll" \
    -H "Content-Type: application/json" \
    -d "$ENROLL_PAYLOAD")

  CA_CERT_WRITE=""
  if [ "$USE_CA_CERT" = "1" ]; then
    CA_CERT_WRITE="$CA_CERT_PATH"
  fi
  python3 - <<'PY' "$ENROLL_JSON" "$DEVICE_CERT_PATH" "$CA_CERT_WRITE"
import json, sys
data=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(data.get("certPem",""))
if sys.argv[3]:
    open(sys.argv[3],"w").write(data.get("caCertPem",""))
PY

if [ ! -s "$DEVICE_CERT_PATH" ]; then
    echo "Enrollment failed; device cert not created." >&2
    exit 1
  fi
fi

# Ensure the container user can read the cert/key (dev-only convenience).
chmod 0644 "$DEVICE_CERT_PATH" "$DEVICE_KEY_PATH" || true

export CONTROL_PLANE_URL="$AGENT_URL"
export DEVICE_CERT_PATH
export DEVICE_KEY_PATH
if [ "$USE_CA_CERT" = "1" ]; then
  export CONTROL_PLANE_CA_CERT_PATH="$CA_CERT_PATH"
fi
if [ -z "${LOG_EXPORT_ADDR:-}" ] && [ "${LOG_EXPORT:-0}" = "1" ]; then
  export LOG_EXPORT_ADDR="tcp://host.docker.internal:5560"
fi
if [ -n "${LOG_LEVEL:-}" ]; then
  export LOG_LEVEL
fi
export MTLS=1

docker compose -f "$BASE_FILE" -f "$MTLS_FILE" up --build --scale agent="$COUNT" "$@"
