#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}

AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}
SHOW_STATUS=${SHOW_STATUS:-1}

if [ -z "$CA_CERT_PATH" ] && [ -f "$BASE_DIR/dev-ca.crt" ]; then
  CA_CERT_PATH="$BASE_DIR/dev-ca.crt"
fi

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

if [ -z "$AUTH_TOKEN" ] && [ -n "$AUTH_EMAIL" ] && [ -n "$AUTH_PASSWORD" ]; then
  login_payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json
import sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
  login_json=$(curl -sS --fail "${curl_opts[@]}" \
    -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "$login_payload")
  AUTH_TOKEN=$(python3 - <<'PY' "$login_json"
import json
import sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
fi

auth_headers=()
if [ -n "$AUTH_TOKEN" ]; then
  auth_headers=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

echo "Reloading pull credentials: $BASE_URL/api/v1/artifacts/pull-credentials/reload"
curl -sS --fail "${curl_opts[@]}" "${auth_headers[@]}" \
  -X POST "$BASE_URL/api/v1/artifacts/pull-credentials/reload" \
  -H "Content-Type: application/json"
echo

if [ "$SHOW_STATUS" = "1" ]; then
  echo
  echo "Current pull credential resolver status:"
  curl -sS --fail "${curl_opts[@]}" "${auth_headers[@]}" \
    "$BASE_URL/api/v1/artifacts/pull-credentials"
  echo
fi
