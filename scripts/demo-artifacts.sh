#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
ARTIFACT_NAME=${ARTIFACT_NAME:-demo-service}
V1_VERSION=${V1_VERSION:-1.0.0}
V2_VERSION=${V2_VERSION:-2.0.0}
V1_TYPE=${V1_TYPE:-app_bundle}
V2_TYPE=${V2_TYPE:-app_bundle}
OUT_DIR=${OUT_DIR:-/tmp/parcel-demo/artifacts}
DATA_DIR=${DATA_DIR:-/tmp/parcel-demo/data}
SLEEP_BETWEEN=${SLEEP_BETWEEN:-5}
PREAPPLY_TIMEOUT=${PREAPPLY_TIMEOUT:-120}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}

if [ "$SIGN_ARTIFACTS" = "1" ]; then
  source "$BASE_DIR/scripts/ensure-signing-key.sh"
fi

DEVICE_ID=${DEVICE_ID:-}
if [ -z "$DEVICE_ID" ] && [ -f "$DATA_DIR/device-id" ]; then
  DEVICE_ID=$(cat "$DATA_DIR/device-id")
fi
if [ -z "$DEVICE_ID" ] && [ -f "$DATA_DIR/agent-state.json" ]; then
  DEVICE_ID=$(python3 - <<'PY' "$DATA_DIR/agent-state.json"
import json, sys
print(json.load(open(sys.argv[1])).get("deviceId",""))
PY
)
fi
if [ -z "$DEVICE_ID" ]; then
  echo "Device ID not found. Set DEVICE_ID or run ./scripts/run-demo-agent.sh first." >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

V1_TAR="$OUT_DIR/${ARTIFACT_NAME}-${V1_VERSION}.tar.gz"
V2_TAR="$OUT_DIR/${ARTIFACT_NAME}-${V2_VERSION}.tar.gz"

V1_INPUT="$OUT_DIR/input-v1"
V2_INPUT="$OUT_DIR/input-v2"
rm -rf "$V1_INPUT" "$V2_INPUT"
mkdir -p "$V1_INPUT" "$V2_INPUT"
cp -R "$BASE_DIR/examples/demo-service/v1/." "$V1_INPUT/"
cp -R "$BASE_DIR/examples/demo-service/v2/." "$V2_INPUT/"

for input in "$V1_INPUT" "$V2_INPUT"; do
  cat > "$input/preapply.sh" <<'EOF'
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
  chmod 0755 "$input/preapply.sh"
  cat > "$input/plan.yaml" <<EOF
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: ${PREAPPLY_TIMEOUT}
EOF
done

pack_args_v1=(--name "$ARTIFACT_NAME" --version "$V1_VERSION" --type "$V1_TYPE" --input-dir "$V1_INPUT" --out "$V1_TAR")
if [ "$SIGN_ARTIFACTS" = "1" ]; then
  pack_args_v1+=(--signing-key "$SIGNING_KEY" --signing-key-id "$SIGNING_KEY_ID")
fi
PACK_V1=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${pack_args_v1[@]}")
V1_SIG=$(python3 - <<'PY' "$PACK_V1"
import json, sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
V1_SIG_KEY_ID=$(python3 - <<'PY' "$PACK_V1"
import json, sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

pack_args_v2=(--name "$ARTIFACT_NAME" --version "$V2_VERSION" --type "$V2_TYPE" --input-dir "$V2_INPUT" --out "$V2_TAR")
if [ "$SIGN_ARTIFACTS" = "1" ]; then
  pack_args_v2+=(--signing-key "$SIGNING_KEY" --signing-key-id "$SIGNING_KEY_ID")
fi
PACK_V2=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${pack_args_v2[@]}")
V2_SIG=$(python3 - <<'PY' "$PACK_V2"
import json, sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
V2_SIG_KEY_ID=$(python3 - <<'PY' "$PACK_V2"
import json, sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  CA_CERT_PATH=${CONTROL_PLANE_CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
  if [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  else
    echo "CA cert not found at $CA_CERT_PATH; set CONTROL_PLANE_CA_CERT_PATH or use http." >&2
    exit 1
  fi
fi

form_v1=(-F "name=$ARTIFACT_NAME" -F "version=$V1_VERSION" -F "type=$V1_TYPE" -F "file=@$V1_TAR")
if [ -n "$V1_SIG" ]; then
  form_v1+=(-F "signature=$V1_SIG" -F "signatureKeyId=$V1_SIG_KEY_ID")
fi
UPLOAD_V1=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_v1[@]}")
ARTIFACT_ID_V1=$(python3 - <<'PY' "$UPLOAD_V1"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId",""))
PY
)

form_v2=(-F "name=$ARTIFACT_NAME" -F "version=$V2_VERSION" -F "type=$V2_TYPE" -F "file=@$V2_TAR")
if [ -n "$V2_SIG" ]; then
  form_v2+=(-F "signature=$V2_SIG" -F "signatureKeyId=$V2_SIG_KEY_ID")
fi
UPLOAD_V2=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_v2[@]}")
ARTIFACT_ID_V2=$(python3 - <<'PY' "$UPLOAD_V2"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId",""))
PY
)

if [ -z "$ARTIFACT_ID_V1" ] || [ -z "$ARTIFACT_ID_V2" ]; then
  echo "Failed to upload artifacts." >&2
  exit 1
fi

curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
  -H "Content-Type: application/json" \
  -d "{\"desiredVersion\":\"$V1_VERSION\",\"artifactId\":\"$ARTIFACT_ID_V1\"}" >/dev/null

echo "Set desired version to $V1_VERSION for device $DEVICE_ID."

if [ "$SLEEP_BETWEEN" -gt 0 ]; then
  echo "Waiting $SLEEP_BETWEEN seconds before switching to $V2_VERSION..."
  sleep "$SLEEP_BETWEEN"
fi

curl -s "${curl_opts[@]}" -X PUT "$BASE_URL/api/v1/desired-state/devices/$DEVICE_ID" \
  -H "Content-Type: application/json" \
  -d "{\"desiredVersion\":\"$V2_VERSION\",\"artifactId\":\"$ARTIFACT_ID_V2\"}" >/dev/null

echo "Set desired version to $V2_VERSION for device $DEVICE_ID."
