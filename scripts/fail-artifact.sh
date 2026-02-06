#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

ARTIFACT_NAME=${ARTIFACT_NAME:-agent}
GOOD_VERSION=${GOOD_VERSION:-1.0.0}
BAD_VERSION=${BAD_VERSION:-99.0.0}
GOOD_TYPE=${GOOD_TYPE:-app_bundle}
BAD_TYPE=${BAD_TYPE:-app_bundle}
STATE_PATH=${STATE_PATH:-/tmp/agent-state-fail.json}
ARTIFACT_ROOT=${ARTIFACT_ROOT:-/tmp/agent-data-fail}
FAIL_ARTIFACT_SIZE_GB=${FAIL_ARTIFACT_SIZE_GB:-5}
SKIP_BASELINE=${SKIP_BASELINE:-0}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}
REQUIRE_ARTIFACT_SIGNATURE=${REQUIRE_ARTIFACT_SIGNATURE:-$SIGN_ARTIFACTS}

if [ "$SIGN_ARTIFACTS" = "1" ]; then
  source "$BASE_DIR/scripts/ensure-signing-key.sh"
fi

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

sign_file() {
  if [ "$SIGN_ARTIFACTS" != "1" ]; then
    echo ""
    return
  fi
  local file="$1"
  local sha
  sha=$(sha256sum "$file" | awk '{print $1}')
  python3 - <<'PY' "$SIGNING_KEY" "$sha"
import base64, binascii, pathlib, subprocess, sys, tempfile
key=sys.argv[1]
sha=sys.argv[2]
sha_bytes=binascii.unhexlify(sha)
with tempfile.TemporaryDirectory() as tmp:
    sha_path=pathlib.Path(tmp)/"sha.bin"
    sig_path=pathlib.Path(tmp)/"sig.bin"
    sha_path.write_bytes(sha_bytes)
    subprocess.check_call(["openssl","pkeyutl","-sign","-inkey",key,"-rawin","-in",str(sha_path),"-out",str(sig_path)])
    print(base64.b64encode(sig_path.read_bytes()).decode("utf-8"))
PY
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
  cat > "$GOOD_DIR/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
cat > files/preapply.txt <<EOF_TXT
preapply ok
type=${HWOPS_ARTIFACT_TYPE}
version=${HWOPS_ARTIFACT_VERSION}
time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF_TXT
EOF
  chmod 0755 "$GOOD_DIR/preapply.sh"
  cat > "$GOOD_DIR/plan.yaml" <<'EOF'
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 120
EOF
  GOOD_TAR=/tmp/agent-good-"$GOOD_VERSION".tar.gz
  pack_args=(--name "$ARTIFACT_NAME" --version "$GOOD_VERSION" --type "$GOOD_TYPE" --input-dir "$GOOD_DIR/files" --out "$GOOD_TAR")
  if [ "$SIGN_ARTIFACTS" = "1" ]; then
    pack_args+=(--signing-key "$SIGNING_KEY" --signing-key-id "$SIGNING_KEY_ID")
  fi
  PACK_JSON=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${pack_args[@]}")
  GOOD_SIG=$(python3 - <<'PY' "$PACK_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
  GOOD_SIG_KEY_ID=$(python3 - <<'PY' "$PACK_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

  form_good=(-F "name=$ARTIFACT_NAME" -F "version=$GOOD_VERSION" -F "type=$GOOD_TYPE" -F "file=@$GOOD_TAR")
  if [ -n "$GOOD_SIG" ]; then
    form_good+=(-F "signature=$GOOD_SIG" -F "signatureKeyId=$GOOD_SIG_KEY_ID")
  fi
  GOOD_UPLOAD_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_good[@]}")
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
LOG_ENV=()
if [ -n "${LOG_EXPORT_ADDR:-}" ]; then
  LOG_ENV=("LOG_EXPORT_ADDR=$LOG_EXPORT_ADDR")
fi
SIGN_ENV=()
if [ "$REQUIRE_ARTIFACT_SIGNATURE" = "1" ]; then
  SIGN_ENV=("REQUIRE_ARTIFACT_SIGNATURE=1" "SIGNING_PUB_KEY_PATH=$SIGNING_PUB")
  if [ -n "${SIGNING_KEY_ID:-}" ]; then
    SIGN_ENV+=("SIGNING_KEY_ID=$SIGNING_KEY_ID")
  fi
fi

(cd "$BASE_DIR/agent" && \
  env CONTROL_PLANE_URL="$BASE_URL" ARTIFACT_ROOT="$ARTIFACT_ROOT" STATE_PATH="$STATE_PATH" \
  DEVICE_CERT_PATH="$CSR_DIR/device.crt" DEVICE_KEY_PATH="$CSR_DIR/device.key" \
  "${CA_ENV[@]}" "${LOG_ENV[@]}" "${SIGN_ENV[@]}" \
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
cat > "$BAD_DIR/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
cat > files/preapply.txt <<EOF_TXT
preapply ok
type=${HWOPS_ARTIFACT_TYPE}
version=${HWOPS_ARTIFACT_VERSION}
time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF_TXT
EOF
chmod 0755 "$BAD_DIR/preapply.sh"
cat > "$BAD_DIR/plan.yaml" <<'EOF'
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 120
EOF
FILE_SIZE=$(stat_size "$BAD_DIR/files/large.bin")
CREATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

cat > "$BAD_DIR/manifest.json" <<EOF_MANIFEST
{
  "name": "$ARTIFACT_NAME",
  "version": "$BAD_VERSION",
  "type": "$BAD_TYPE",
  "createdAt": "$CREATED_AT",
  "files": [
    { "path": "files/large.bin", "sha256": "deadbeef", "size": $FILE_SIZE }
  ]
}
EOF_MANIFEST

BAD_TAR=/tmp/agent-bad-"$BAD_VERSION".tar.gz
tar -czf "$BAD_TAR" -C "$BAD_DIR" .

BAD_SIG=$(sign_file "$BAD_TAR")
form_bad=(-F "name=$ARTIFACT_NAME" -F "version=$BAD_VERSION" -F "type=$BAD_TYPE" -F "file=@$BAD_TAR")
if [ -n "$BAD_SIG" ]; then
  form_bad+=(-F "signature=$BAD_SIG" -F "signatureKeyId=$SIGNING_KEY_ID")
fi
BAD_UPLOAD_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_bad[@]}")
BAD_ARTIFACT_ID=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["artifactId"])' <<<"$BAD_UPLOAD_JSON")

curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
  -H "Content-Type: application/json" \
  -d "{\"desiredVersion\":\"$BAD_VERSION\",\"artifactId\":\"$BAD_ARTIFACT_ID\"}" >/dev/null

CA_ENV=()
if [ -n "$CA_CERT_PATH" ]; then
  CA_ENV=("CONTROL_PLANE_CA_CERT_PATH=$CA_CERT_PATH")
fi
LOG_ENV=()
if [ -n "${LOG_EXPORT_ADDR:-}" ]; then
  LOG_ENV=("LOG_EXPORT_ADDR=$LOG_EXPORT_ADDR")
fi
SIGN_ENV=()
if [ "$REQUIRE_ARTIFACT_SIGNATURE" = "1" ]; then
  SIGN_ENV=("REQUIRE_ARTIFACT_SIGNATURE=1" "SIGNING_PUB_KEY_PATH=$SIGNING_PUB")
  if [ -n "${SIGNING_KEY_ID:-}" ]; then
    SIGN_ENV+=("SIGNING_KEY_ID=$SIGNING_KEY_ID")
  fi
fi

echo "Attempting apply of bad artifact (expected to fail)..."
(cd "$BASE_DIR/agent" && \
  env CONTROL_PLANE_URL="$BASE_URL" ARTIFACT_ROOT="$ARTIFACT_ROOT" STATE_PATH="$STATE_PATH" \
  DEVICE_CERT_PATH="$CSR_DIR/device.crt" DEVICE_KEY_PATH="$CSR_DIR/device.key" \
  "${CA_ENV[@]}" "${LOG_ENV[@]}" "${SIGN_ENV[@]}" \
  go run ./cmd/agent -once) || true

echo "Current symlink:"
readlink -f "$ARTIFACT_ROOT/current" || true
echo "Agent state:"
cat "$STATE_PATH"

echo "Device ID: $DEVICE_ID"
if [ -n "${LOG_EXPORT_ADDR:-}" ]; then
  echo "Fetch logs: curl -s $BASE_URL/api/v1/logs/$DEVICE_ID -o /tmp/device-logs.csv"
fi
echo "Fail-artifact flow complete. Expect lastApplyStatus=error and current version unchanged."
