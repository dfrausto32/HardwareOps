#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/parcel-prod-docker"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
CERTS_DIR=${CERTS_DIR:-"$LAB_ROOT/certs"}

if [ ! -f "$ENV_FILE" ]; then
  echo "Lab env file not found: $ENV_FILE" >&2
  exit 1
fi

public_url=$(awk -F= '/^PUBLIC_BASE_URL=/{print $2}' "$ENV_FILE" | tail -n1)
admin_email=$(awk -F= '/^AUTH_BOOTSTRAP_EMAIL=/{print $2}' "$ENV_FILE" | tail -n1)
admin_password=$(awk -F= '/^AUTH_BOOTSTRAP_PASSWORD=/{print $2}' "$ENV_FILE" | tail -n1)
public_host=$(echo "$public_url" | sed -e 's#^[a-zA-Z0-9+.-]*://##' -e 's#/.*$##' -e 's/:.*$//')
ca_cert="$CERTS_DIR/ca.crt"

if [ ! -f "$ca_cert" ]; then
  echo "CA cert not found: $ca_cert" >&2
  exit 1
fi

echo "[smoke] healthz"
curl --fail --silent --show-error \
  --resolve "$public_host:443:127.0.0.1" \
  --cacert "$ca_cert" \
  "$public_url/healthz" >/dev/null

echo "[smoke] auth login"
login_json=$(curl --fail --silent --show-error --cacert "$ca_cert" \
  --resolve "$public_host:443:127.0.0.1" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$admin_email\",\"password\":\"$admin_password\"}" \
  "$public_url/api/v1/auth/login")

token=$(python3 - <<'PY' "$login_json"
import json
import sys
obj = json.loads(sys.argv[1])
token = obj.get("token", "")
if not token:
    raise SystemExit(1)
print(token)
PY
)

echo "[smoke] devices endpoint"
curl --fail --silent --show-error --cacert "$ca_cert" \
  --resolve "$public_host:443:127.0.0.1" \
  -H "Authorization: Bearer $token" \
  "$public_url/api/v1/devices" >/dev/null

echo "[smoke] metrics endpoint"
curl --fail --silent --show-error \
  --resolve "$public_host:443:127.0.0.1" \
  --cacert "$ca_cert" \
  "$public_url/metrics" >/dev/null

echo "Smoke checks passed."
