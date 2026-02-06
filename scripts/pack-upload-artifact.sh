#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

ARTIFACT_NAME=${ARTIFACT_NAME:-}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
INPUT_DIR=${INPUT_DIR:-}
OUT_PATH=${OUT_PATH:-}
METADATA_JSON=${METADATA_JSON:-}

SIGNING_KEY=${SIGNING_KEY:-}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}
SIGNATURE_OUT=${SIGNATURE_OUT:-}

if [ -z "$ARTIFACT_NAME" ] || [ -z "$ARTIFACT_VERSION" ] || [ -z "$INPUT_DIR" ]; then
  cat <<USAGE
Usage:
  ARTIFACT_NAME=demo ARTIFACT_VERSION=1.0.0 INPUT_DIR=/path/to/bundle \
  [ARTIFACT_TYPE=app_bundle] [OUT_PATH=/tmp/demo.tar.gz] \
  [BASE_URL=https://localhost:8080] [CA_CERT_PATH=./dev-ca.crt] [INSECURE=1] \
  [SIGNING_KEY=/path/to/ed25519.key] [SIGNING_KEY_ID=sha256:...] \
  [METADATA_JSON='{"foo":"bar"}'] \
  ./scripts/pack-upload-artifact.sh
USAGE
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

form_args=(
  -F "name=$ARTIFACT_NAME"
  -F "version=$ARTIFACT_VERSION"
  -F "type=$ARTIFACT_TYPE"
  -F "file=@$ARTIFACT_PATH"
)

if [ -n "$SIGNATURE" ]; then
  form_args+=(-F "signature=$SIGNATURE")
fi
if [ -n "$SIGNATURE_KEY_ID" ]; then
  form_args+=(-F "signatureKeyId=$SIGNATURE_KEY_ID")
fi
if [ -n "$METADATA_JSON" ]; then
  form_args+=(-F "metadata=$METADATA_JSON")
fi

curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/artifacts/upload" "${form_args[@]}"
