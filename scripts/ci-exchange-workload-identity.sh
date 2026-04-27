#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}
WORKLOAD_IDENTITY_PROVIDER=${WORKLOAD_IDENTITY_PROVIDER:-github-actions}
CI_WORKLOAD_IDENTITY_TOKEN=${CI_WORKLOAD_IDENTITY_TOKEN:-}
CI_WORKLOAD_IDENTITY_AUDIENCE=${CI_WORKLOAD_IDENTITY_AUDIENCE:-parcel-ci}
REQUESTED_SCOPES=${REQUESTED_SCOPES:-artifact.publish}
OUTPUT=${OUTPUT:-token}

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi
if [ -n "${CURL_RESOLVE_HOSTS:-}" ]; then
  IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
  for entry in "${resolve_entries[@]}"; do
    entry=$(echo "$entry" | xargs)
    if [ -n "$entry" ]; then
      curl_opts+=(--resolve "$entry")
    fi
  done
fi

if [ -z "$CI_WORKLOAD_IDENTITY_TOKEN" ] && [ -n "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" ] && [ -n "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]; then
  encoded_audience=$(
    python3 - <<'PY' "$CI_WORKLOAD_IDENTITY_AUDIENCE"
import sys, urllib.parse
print(urllib.parse.quote(sys.argv[1], safe=""))
PY
  )
  request_url="${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=${encoded_audience}"
  oidc_json=$(curl -sS --fail \
    -H "Authorization: Bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
    "$request_url")
  CI_WORKLOAD_IDENTITY_TOKEN=$(
    python3 - <<'PY' "$oidc_json"
import json, sys
print(json.loads(sys.argv[1])["value"])
PY
  )
fi

if [ -z "$CI_WORKLOAD_IDENTITY_TOKEN" ]; then
  echo "CI_WORKLOAD_IDENTITY_TOKEN is required (or run inside GitHub Actions with id-token: write)." >&2
  exit 1
fi

payload=$(
  python3 - <<'PY' \
    "$WORKLOAD_IDENTITY_PROVIDER" \
    "$CI_WORKLOAD_IDENTITY_TOKEN" \
    "$REQUESTED_SCOPES"
import json, sys
scopes = [item.strip() for item in sys.argv[3].split(",") if item.strip()]
print(json.dumps({
    "provider": sys.argv[1],
    "idToken": sys.argv[2],
    "scopes": scopes,
}))
PY
)

response=$(curl -sS --fail "${curl_opts[@]}" \
  -X POST "$BASE_URL/api/v1/auth/workload-identity/exchange" \
  -H "Content-Type: application/json" \
  -d "$payload")

case "$OUTPUT" in
  json)
    echo "$response"
    ;;
  token)
    python3 - <<'PY' "$response"
import json, sys
print(json.loads(sys.argv[1])["token"])
PY
    ;;
  *)
    echo "OUTPUT must be token or json" >&2
    exit 1
    ;;
esac
