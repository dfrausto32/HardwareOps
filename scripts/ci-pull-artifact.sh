#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}

ARTIFACT_NAME=${ARTIFACT_NAME:-}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
ARTIFACT_SHA256=${ARTIFACT_SHA256:-}
ARTIFACT_SIZE_BYTES=${ARTIFACT_SIZE_BYTES:-}
ARTIFACT_SIGNATURE=${ARTIFACT_SIGNATURE:-}
ARTIFACT_SIGNATURE_KEY_ID=${ARTIFACT_SIGNATURE_KEY_ID:-}
METADATA_JSON=${METADATA_JSON:-{}}

SOURCE_URL=${SOURCE_URL:-}
SOURCE_KIND=${SOURCE_KIND:-}
SOURCE_URI=${SOURCE_URI:-}
SOURCE_CREDENTIAL_REF=${SOURCE_CREDENTIAL_REF:-}

AUTH_TOKEN=${AUTH_TOKEN:-}
CI_SERVICE_TOKEN=${CI_SERVICE_TOKEN:-}

if [ -z "$ARTIFACT_NAME" ] || [ -z "$ARTIFACT_VERSION" ] || [ -z "$ARTIFACT_SHA256" ]; then
  cat <<'USAGE'
Usage:
  ARTIFACT_NAME=agent \
  ARTIFACT_VERSION=1.2.3 \
  ARTIFACT_SHA256=<sha256> \
  SOURCE_URL=https://repo.example.com/path/agent.tar.gz \
  CI_SERVICE_TOKEN=<token> \
  BASE_URL=https://control-plane.example.com \
  ./scripts/ci-pull-artifact.sh

Or adapter-ready source:
  SOURCE_KIND=artifactory \
  SOURCE_URI=https://artifactory.example.com/artifactory/repo/path/agent.tar.gz \
  SOURCE_CREDENTIAL_REF=artifactory-prod
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

if [ -z "$SOURCE_URL" ] && { [ -z "$SOURCE_KIND" ] || [ -z "$SOURCE_URI" ]; }; then
  echo "Provide SOURCE_URL or both SOURCE_KIND and SOURCE_URI." >&2
  exit 1
fi

if [ -n "$SOURCE_URL" ] && { [ -n "$SOURCE_KIND" ] || [ -n "$SOURCE_URI" ]; }; then
  echo "Use SOURCE_URL or SOURCE_KIND/SOURCE_URI, not both." >&2
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

payload=$(
  python3 - <<'PY' \
    "$ARTIFACT_NAME" \
    "$ARTIFACT_VERSION" \
    "$ARTIFACT_TYPE" \
    "$ARTIFACT_SHA256" \
    "$ARTIFACT_SIZE_BYTES" \
    "$ARTIFACT_SIGNATURE" \
    "$ARTIFACT_SIGNATURE_KEY_ID" \
    "$METADATA_JSON" \
    "$SOURCE_URL" \
    "$SOURCE_KIND" \
    "$SOURCE_URI" \
    "$SOURCE_CREDENTIAL_REF"
import json
import sys

payload = {
    "name": sys.argv[1],
    "version": sys.argv[2],
    "type": sys.argv[3],
    "sha256": sys.argv[4],
}
if sys.argv[5]:
    payload["sizeBytes"] = int(sys.argv[5])
if sys.argv[6]:
    payload["signature"] = sys.argv[6]
if sys.argv[7]:
    payload["signatureKeyId"] = sys.argv[7]

metadata = json.loads(sys.argv[8]) if sys.argv[8] else {}
payload["metadata"] = metadata

if sys.argv[9]:
    payload["sourceUrl"] = sys.argv[9]
else:
    source = {"kind": sys.argv[10], "uri": sys.argv[11]}
    if sys.argv[12]:
        source["credentialRef"] = sys.argv[12]
    payload["source"] = source

print(json.dumps(payload))
PY
)

curl -sS --fail "${curl_opts[@]}" \
  -X POST "$BASE_URL/api/v1/artifacts/pull" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$payload"
