#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/parcel-prod-docker"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
DEMO_COUNT=${DEMO_COUNT:-1}
DEMO_RESET=${DEMO_RESET:-0}
APP_NAMES=${APP_NAMES:-customer,integration}
VERSIONS=${VERSIONS:-0.1.0,0.2.0}
COMPONENT_PREFIX=${COMPONENT_PREFIX:-app:}

if [ ! -f "$ENV_FILE" ]; then
  echo "Lab env file not found: $ENV_FILE" >&2
  echo "Run: ./scripts/testing/prod-lab-init.sh && ./scripts/testing/prod-lab-up.sh" >&2
  exit 1
fi

read_env() {
  local key="$1"
  awk -F= -v k="$key" '$1==k{print substr($0, index($0,$2))}' "$ENV_FILE" | tail -n1
}

PUBLIC_BASE_URL=$(read_env PUBLIC_BASE_URL)
AGENT_BASE_URL=$(read_env AGENT_BASE_URL)
AUTH_EMAIL=$(read_env AUTH_BOOTSTRAP_EMAIL)
AUTH_PASSWORD=$(read_env AUTH_BOOTSTRAP_PASSWORD)
CERTS_DIR=$(read_env CERTS_DIR)
STACK_DIR=$(read_env STACK_DIR)
SIGNING_DIR=${SIGNING_DIR:-"$STACK_DIR/signing"}
CA_CERT_PATH="$CERTS_DIR/ca.crt"
DEMO_CERT_DIR=${DEMO_CERT_DIR:-"$LAB_ROOT/demo/certs"}
DEMO_DATA_DIR=${DEMO_DATA_DIR:-"$LAB_ROOT/demo/data"}

if [ -z "$PUBLIC_BASE_URL" ] || [ -z "$AGENT_BASE_URL" ]; then
  echo "PUBLIC_BASE_URL / AGENT_BASE_URL missing in $ENV_FILE" >&2
  exit 1
fi
if [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; then
  echo "AUTH_BOOTSTRAP_EMAIL / AUTH_BOOTSTRAP_PASSWORD missing in $ENV_FILE" >&2
  exit 1
fi
if [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA cert not found: $CA_CERT_PATH" >&2
  exit 1
fi
if [ ! -d "$SIGNING_DIR" ]; then
  echo "Signing dir not found: $SIGNING_DIR" >&2
  echo "Expected lab signing keys under $STACK_DIR/signing" >&2
  exit 1
fi

host_from_url() {
  local url="$1"
  echo "$url" | sed -e 's#^[a-zA-Z0-9+.-]*://##' -e 's#/.*$##' -e 's/:.*$//'
}

host_resolves() {
  local host="$1"
  getent hosts "$host" >/dev/null 2>&1
}

public_host="$(host_from_url "$PUBLIC_BASE_URL")"
agent_host="$(host_from_url "$AGENT_BASE_URL")"
resolve_entries=()
if ! host_resolves "$public_host"; then
  resolve_entries+=("$public_host:443:127.0.0.1")
fi
if ! host_resolves "$agent_host"; then
  resolve_entries+=("$agent_host:443:127.0.0.1")
fi
if [ ${#resolve_entries[@]} -gt 0 ]; then
  CURL_RESOLVE_HOSTS=$(IFS=','; echo "${resolve_entries[*]}")
  export CURL_RESOLVE_HOSTS
  cat <<EOF
[seed] host resolution fallback enabled via curl --resolve:
  $CURL_RESOLVE_HOSTS
If you want shell-level resolution too:
  echo "127.0.0.1 $public_host $agent_host" | sudo tee -a /etc/hosts
EOF
fi

echo "[seed] launching demo agents (count=$DEMO_COUNT reset=$DEMO_RESET)"
BASE_URL="$PUBLIC_BASE_URL" \
AGENT_BASE_URL="$AGENT_BASE_URL" \
CONTROL_PLANE_CA_CERT_PATH="$CA_CERT_PATH" \
AUTH_EMAIL="$AUTH_EMAIL" \
AUTH_PASSWORD="$AUTH_PASSWORD" \
DEMO_COUNT="$DEMO_COUNT" \
DEMO_RESET="$DEMO_RESET" \
CERT_DIR="$DEMO_CERT_DIR" \
DATA_DIR="$DEMO_DATA_DIR" \
CERTS_RW=1 \
SIGNING_DIR="$SIGNING_DIR" \
"$BASE_DIR/scripts/run-demo-agent.sh"

echo "[seed] uploading signed multi-app artifacts"
BASE_URL="$PUBLIC_BASE_URL" \
CA_CERT_PATH="$CA_CERT_PATH" \
AUTH_EMAIL="$AUTH_EMAIL" \
AUTH_PASSWORD="$AUTH_PASSWORD" \
SIGNING_DIR="$SIGNING_DIR" \
APP_NAMES="$APP_NAMES" \
VERSIONS="$VERSIONS" \
COMPONENT_PREFIX="$COMPONENT_PREFIX" \
"$BASE_DIR/scripts/multi-app-artifacts.sh"

cat <<EOF
Seed complete.

Devices:
  - demo agents: $DEMO_COUNT
Artifacts uploaded:
  - agent bundle via run-demo-agent.sh
  - multi-app artifacts: names=[$APP_NAMES], versions=[$VERSIONS], componentPrefix=$COMPONENT_PREFIX

Recommended next checks:
  1) Open UI -> Devices and confirm active device count.
  2) Open UI -> Artifacts and confirm signatures show as valid.
  3) Set desired components for one group/device (for example $COMPONENT_PREFIX${APP_NAMES%%,*}) and verify apply status.
EOF
