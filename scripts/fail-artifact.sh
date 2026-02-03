#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

ARTIFACT_NAME=${ARTIFACT_NAME:-agent}
GOOD_VERSION=${GOOD_VERSION:-1.0.0}
BAD_VERSION=${BAD_VERSION:-99.0.0}
STATE_PATH=${STATE_PATH:-/tmp/agent-state-fail.json}
ARTIFACT_ROOT=${ARTIFACT_ROOT:-/tmp/agent-data-fail}
FAIL_ARTIFACT_SIZE_GB=${FAIL_ARTIFACT_SIZE_GB:-5}
SKIP_BASELINE=${SKIP_BASELINE:-0}

if ! [[ "$FAIL_ARTIFACT_SIZE_GB" =~ ^[0-9]+$ ]]; then
  echo "FAIL_ARTIFACT_SIZE_GB must be an integer (GB)." >&2
  exit 1
fi
if [ "$FAIL_ARTIFACT_SIZE_GB" -lt 5 ]; then
  FAIL_ARTIFACT_SIZE_GB=5
fi

if [ -n "$CA_CERT_PATH" ] && [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA_CERT_PATH not found at $CA_CERT_PATH; proceeding without --cacert." >&2
  CA_CERT_PATH=""
fi
if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
  if command -v realpath >/dev/null 2>&1; then
    CA_CERT_PATH=$(realpath -m "$CA_CERT_PATH")
  elif command -v python3 >/dev/null 2>&1; then
    CA_CERT_PATH=$(python3 - <<'PY' "$CA_CERT_PATH"
import os, sys
print(os.path.abspath(sys.argv[1]))
PY
)
  fi
fi

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

stat_size() {
  if stat -c %s "$1" >/dev/null 2>&1; then
    stat -c %s "$1"
  else
    stat -f %z "$1"
  fi
}

# Health check
curl -s "${curl_opts[@]}" "$BASE_URL/healthz" >/dev/null

# Create enrollment token
TOKEN_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/enrollments" -H "Content-Type: application/json" -d '{"expiresInSec":3600}')
TOKEN=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<<"$TOKEN_JSON")

# Generate CSR + enroll
CSR_DIR=/tmp/hardwareops-fail
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

if [ "$SKIP_BASELINE" != "1" ]; then
  GOOD_DIR=/tmp/hardwareops-good
  rm -rf "$GOOD_DIR"
  mkdir -p "$GOOD_DIR/files"
  dd if=/dev/urandom of="$GOOD_DIR/files/hello.bin" bs=1M count=5 status=none
  GOOD_TAR=/tmp/agent-good-"$GOOD_VERSION".tar.gz
  python3 "$BASE_DIR/scripts/artifact-pack.py" \
    --name "$ARTIFACT_NAME" \
    --version "$GOOD_VERSION" \
    --input-dir "$GOOD_DIR/files" \
    --out "$GOOD_TAR" >/dev/null

  GOOD_UPLOAD_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" \
    -F "name=$ARTIFACT_NAME" \
    -F "version=$GOOD_VERSION" \
    -F "file=@$GOOD_TAR")
  GOOD_ARTIFACT_ID=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["artifactId"])' <<<"$GOOD_UPLOAD_JSON")

  curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
    -H "Content-Type: application/json" \
    -d "{\"desiredVersion\":\"$GOOD_VERSION\",\"artifactId\":\"$GOOD_ARTIFACT_ID\"}" >/dev/null

  cat > "$STATE_PATH" <<EOF_STATE
{"deviceId":"$DEVICE_ID","agentVersion":"0.1.0","currentVersion":"","currentConfigRev":""}
EOF_STATE

  CA_ENV=()
  if [ -n "$CA_CERT_PATH" ]; then
    CA_ENV=("CONTROL_PLANE_CA_CERT_PATH=$CA_CERT_PATH")
  fi

  (cd "$BASE_DIR/agent" && \
    env CONTROL_PLANE_URL="$BASE_URL" ARTIFACT_ROOT="$ARTIFACT_ROOT" STATE_PATH="$STATE_PATH" \
    DEVICE_CERT_PATH="$CSR_DIR/device.crt" DEVICE_KEY_PATH="$CSR_DIR/device.key" \
    "${CA_ENV[@]}" \
    go run ./cmd/agent -once)

  if [ ! -L "$ARTIFACT_ROOT/current" ]; then
    echo "Expected symlink at $ARTIFACT_ROOT/current after baseline apply." >&2
    exit 1
  fi
fi

BAD_DIR=/tmp/hardwareops-bad
rm -rf "$BAD_DIR"
mkdir -p "$BAD_DIR/files"
echo "Creating ${FAIL_ARTIFACT_SIZE_GB}GB artifact (this may take a while)..."
dd if=/dev/urandom of="$BAD_DIR/files/large.bin" bs=1M count=$((FAIL_ARTIFACT_SIZE_GB * 1024)) status=progress
FILE_SIZE=$(stat_size "$BAD_DIR/files/large.bin")
CREATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

cat > "$BAD_DIR/manifest.json" <<EOF_MANIFEST
{
  "name": "$ARTIFACT_NAME",
  "version": "$BAD_VERSION",
  "createdAt": "$CREATED_AT",
  "files": [
    { "path": "files/large.bin", "sha256": "deadbeef", "size": $FILE_SIZE }
  ]
}
EOF_MANIFEST

BAD_TAR=/tmp/agent-bad-"$BAD_VERSION".tar.gz
tar -czf "$BAD_TAR" -C "$BAD_DIR" .

BAD_UPLOAD_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" \
  -F "name=$ARTIFACT_NAME" \
  -F "version=$BAD_VERSION" \
  -F "file=@$BAD_TAR")
BAD_ARTIFACT_ID=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["artifactId"])' <<<"$BAD_UPLOAD_JSON")

curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
  -H "Content-Type: application/json" \
  -d "{\"desiredVersion\":\"$BAD_VERSION\",\"artifactId\":\"$BAD_ARTIFACT_ID\"}" >/dev/null

CA_ENV=()
if [ -n "$CA_CERT_PATH" ]; then
  CA_ENV=("CONTROL_PLANE_CA_CERT_PATH=$CA_CERT_PATH")
fi

echo "Attempting apply of bad artifact (expected to fail)..."
(cd "$BASE_DIR/agent" && \
  env CONTROL_PLANE_URL="$BASE_URL" ARTIFACT_ROOT="$ARTIFACT_ROOT" STATE_PATH="$STATE_PATH" \
  DEVICE_CERT_PATH="$CSR_DIR/device.crt" DEVICE_KEY_PATH="$CSR_DIR/device.key" \
  "${CA_ENV[@]}" \
  go run ./cmd/agent -once) || true

echo "Current symlink:"
readlink -f "$ARTIFACT_ROOT/current" || true
echo "Agent state:"
cat "$STATE_PATH"

echo "Fail-artifact flow complete. Expect lastApplyStatus=error and current version unchanged."
