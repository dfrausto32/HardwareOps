#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}
PRESIGN_EXPIRES_SECONDS=${PRESIGN_EXPIRES_SECONDS:-900}

ARTIFACT_NAME=${ARTIFACT_NAME:-}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
INPUT_DIR=${INPUT_DIR:-}
OUT_PATH=${OUT_PATH:-}
METADATA_JSON=${METADATA_JSON:-}

SIGNING_KEY=${SIGNING_KEY:-}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}
SIGNATURE_OUT=${SIGNATURE_OUT:-}

AUTH_TOKEN=${AUTH_TOKEN:-}
CI_SERVICE_TOKEN=${CI_SERVICE_TOKEN:-}

if [ -z "$ARTIFACT_NAME" ] || [ -z "$ARTIFACT_VERSION" ] || [ -z "$INPUT_DIR" ]; then
  cat <<USAGE
Usage:
  ARTIFACT_NAME=agent ARTIFACT_VERSION=1.2.3 INPUT_DIR=/path/to/bundle \\
  CI_SERVICE_TOKEN=<service-token> \\
  [ARTIFACT_TYPE=agent_bundle] [OUT_PATH=/tmp/agent-1.2.3.tar.gz] \\
  [BASE_URL=https://localhost:8080] [CA_CERT_PATH=./dev-ca.crt] [INSECURE=1] \\
  [SIGNING_KEY=/path/to/ed25519.key] [SIGNING_KEY_ID=sha256:...] \\
  [METADATA_JSON='{\"channel\":\"stable\"}'] \\
  ./scripts/ci-upload-artifact.sh
USAGE
  exit 1
fi

if [ -z "$AUTH_TOKEN" ] && [ -n "$CI_SERVICE_TOKEN" ]; then
  AUTH_TOKEN="$CI_SERVICE_TOKEN"
fi
if [ -z "$AUTH_TOKEN" ] && [ -z "$CI_SERVICE_TOKEN" ]; then
  AUTH_TOKEN=$("$BASE_DIR/scripts/ci-exchange-workload-identity.sh")
fi
if [ -z "$AUTH_TOKEN" ]; then
  echo "AUTH_TOKEN, CI_SERVICE_TOKEN, or workload identity exchange is required." >&2
  exit 1
fi

if [ -z "$OUT_PATH" ]; then
  OUT_PATH="/tmp/${ARTIFACT_NAME}-${ARTIFACT_VERSION}.tar.gz"
fi

PACK_ARGS=(
  --name "$ARTIFACT_NAME"
  --version "$ARTIFACT_VERSION"
  --type "$ARTIFACT_TYPE"
  --input-dir "$INPUT_DIR"
  --out "$OUT_PATH"
)
if [ -n "$SIGNING_KEY" ]; then
  PACK_ARGS+=(--signing-key "$SIGNING_KEY")
fi
if [ -n "$SIGNATURE_OUT" ]; then
  PACK_ARGS+=(--signature-out "$SIGNATURE_OUT")
fi
if [ -n "$SIGNING_KEY_ID" ]; then
  PACK_ARGS+=(--signing-key-id "$SIGNING_KEY_ID")
fi

PACK_JSON=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${PACK_ARGS[@]}")
ARTIFACT_PATH=$(python3 - <<'PY' "$PACK_JSON"
import json,sys
print(json.loads(sys.argv[1])["artifactPath"])
PY
)
ARTIFACT_SIZE=$(python3 - <<'PY' "$PACK_JSON"
import json,sys
print(int(json.loads(sys.argv[1])["sizeBytes"]))
PY
)
ARTIFACT_SHA=$(python3 - <<'PY' "$PACK_JSON"
import json,sys
print(json.loads(sys.argv[1])["sha256"])
PY
)
SIGNATURE=$(python3 - <<'PY' "$PACK_JSON"
import json,sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
SIGNATURE_KEY_ID=$(python3 - <<'PY' "$PACK_JSON"
import json,sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

if [ ! -f "$ARTIFACT_PATH" ]; then
  echo "Packed artifact not found: $ARTIFACT_PATH" >&2
  exit 1
fi

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")

PRESIGN_PAYLOAD=$(python3 - <<'PY' "$ARTIFACT_PATH" "$PRESIGN_EXPIRES_SECONDS"
import json, os, sys
print(json.dumps({
  "filename": os.path.basename(sys.argv[1]),
  "contentType": "application/gzip",
  "expiresSeconds": int(sys.argv[2]),
}))
PY
)

PRESIGN_JSON=$(curl -sS --fail "${curl_opts[@]}" "${auth_args[@]}" \
  -X POST "$BASE_URL/api/v1/artifacts/presign-upload" \
  -H "Content-Type: application/json" \
  -d "$PRESIGN_PAYLOAD")

ARTIFACT_ID=$(python3 - <<'PY' "$PRESIGN_JSON"
import json,sys
print(json.loads(sys.argv[1])["artifactId"])
PY
)
OBJECT_KEY=$(python3 - <<'PY' "$PRESIGN_JSON"
import json,sys
print(json.loads(sys.argv[1])["objectKey"])
PY
)
UPLOAD_URL=$(python3 - <<'PY' "$PRESIGN_JSON"
import json,sys
print(json.loads(sys.argv[1])["uploadUrl"])
PY
)

curl -sS --fail -X PUT -H "Content-Type: application/gzip" --upload-file "$ARTIFACT_PATH" "$UPLOAD_URL" >/dev/null

if [ -z "$METADATA_JSON" ]; then
  METADATA_JSON='{}'
fi

COMPLETE_PAYLOAD=$(python3 - <<'PY' \
  "$ARTIFACT_ID" "$ARTIFACT_NAME" "$ARTIFACT_VERSION" "$ARTIFACT_TYPE" "$OBJECT_KEY" "$ARTIFACT_SHA" "$ARTIFACT_SIZE" "$SIGNATURE" "$SIGNATURE_KEY_ID" "$METADATA_JSON"
import json, sys
meta = json.loads(sys.argv[10]) if sys.argv[10] else {}
payload = {
  "artifactId": sys.argv[1],
  "name": sys.argv[2],
  "version": sys.argv[3],
  "type": sys.argv[4],
  "objectKey": sys.argv[5],
  "sha256": sys.argv[6],
  "sizeBytes": int(sys.argv[7]),
  "metadata": meta,
}
if sys.argv[8]:
  payload["signature"] = sys.argv[8]
if sys.argv[9]:
  payload["signatureKeyId"] = sys.argv[9]
print(json.dumps(payload))
PY
)

curl -sS --fail "${curl_opts[@]}" "${auth_args[@]}" \
  -X POST "$BASE_URL/api/v1/artifacts/complete" \
  -H "Content-Type: application/json" \
  -d "$COMPLETE_PAYLOAD"
