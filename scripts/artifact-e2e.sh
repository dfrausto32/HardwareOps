#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}
ARTIFACT_PATH=${ARTIFACT_PATH:-/tmp/agent-0.0.1.tar.gz}
ARTIFACT_NAME=${ARTIFACT_NAME:-agent}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-0.0.1}
STATE_PATH=${STATE_PATH:-/tmp/agent-state-e2e.json}
ARTIFACT_ROOT=${ARTIFACT_ROOT:-/tmp/agent-data-e2e}

if [ ! -f "$ARTIFACT_PATH" ]; then
  echo "Artifact not found: $ARTIFACT_PATH" >&2
  exit 1
fi

if [ -n "$CA_CERT_PATH" ] && [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA_CERT_PATH not found at $CA_CERT_PATH; proceeding without --cacert." >&2
  CA_CERT_PATH=""
fi

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

# Health check
curl -s "${curl_opts[@]}" "$BASE_URL/healthz" >/dev/null

# Create enrollment token
TOKEN_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/enrollments" -H "Content-Type: application/json" -d '{"expiresInSec":3600}')
TOKEN=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<<"$TOKEN_JSON")

# Generate CSR + enroll
CSR_DIR=/tmp/hardwareops-e2e
mkdir -p "$CSR_DIR"
openssl req -newkey rsa:2048 -nodes -keyout "$CSR_DIR/device.key" -out "$CSR_DIR/device.csr" -subj "/CN=hardwareops-device"
CSR=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' "$CSR_DIR/device.csr")
ENROLL_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/devices/enroll" -H "Content-Type: application/json" -d "{\"token\":\"$TOKEN\",\"csr\":\"$CSR\"}")
DEVICE_ID=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["deviceId"])' <<<"$ENROLL_JSON")
python3 - <<'PY' "$ENROLL_JSON" "$CSR_DIR/device.crt" "$CSR_DIR/ca.crt"
import json, sys
data=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(data.get("certPem",""))
open(sys.argv[3],"w").write(data.get("caCertPem",""))
PY

if [ ! -s "$CSR_DIR/device.crt" ]; then
  echo "Enrollment failed; device cert missing." >&2
  exit 1
fi

if [[ "$BASE_URL" == https:* ]]; then
  if [ ! -f "$CA_CERT_PATH" ] && [ -s "$CSR_DIR/ca.crt" ]; then
    CA_CERT_PATH="$CSR_DIR/ca.crt"
    curl_opts=()
    if [ -f "$CA_CERT_PATH" ]; then
      curl_opts+=(--cacert "$CA_CERT_PATH")
    elif [ "$INSECURE" = "1" ]; then
      curl_opts+=(-k)
    fi
  fi
fi

# Upload artifact via control-plane
UPLOAD_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" \
  -F "name=$ARTIFACT_NAME" \
  -F "version=$ARTIFACT_VERSION" \
  -F "file=@$ARTIFACT_PATH")

ARTIFACT_ID=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["artifactId"])' <<<"$UPLOAD_JSON")

# Set desired state for device
curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
  -H "Content-Type: application/json" \
  -d "{\"desiredVersion\":\"$ARTIFACT_VERSION\",\"artifactId\":\"$ARTIFACT_ID\"}" >/dev/null

# Write agent state
cat > "$STATE_PATH" <<EOF_STATE
{"deviceId":"$DEVICE_ID","agentVersion":"0.1.0","currentVersion":"","currentConfigRev":""}
EOF_STATE

# Run agent once
CA_ENV=()
if [ -n "$CA_CERT_PATH" ]; then
  CA_ENV=("CONTROL_PLANE_CA_CERT_PATH=$CA_CERT_PATH")
fi

(cd "$(dirname "$0")/../agent" && \
  env CONTROL_PLANE_URL="$BASE_URL" ARTIFACT_ROOT="$ARTIFACT_ROOT" STATE_PATH="$STATE_PATH" \
  DEVICE_CERT_PATH="$CSR_DIR/device.crt" DEVICE_KEY_PATH="$CSR_DIR/device.key" \
  "${CA_ENV[@]}" \
  go run ./cmd/agent -once)

# Verify
if [ ! -L "$ARTIFACT_ROOT/current" ]; then
  echo "Expected symlink at $ARTIFACT_ROOT/current" >&2
  exit 1
fi

echo "E2E artifact flow complete."
