#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$BASE_DIR/deploy/compose/docker-compose.onprem.yml"}
LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/hardwareops-prod-docker"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
PROJECT_NAME=${PROJECT_NAME:-hwops-prodtest}
LAB_BUILD=${LAB_BUILD:-1}

if [ ! -f "$ENV_FILE" ]; then
  echo "Prod lab env not found at $ENV_FILE; initializing now."
  "$BASE_DIR/scripts/testing/prod-lab-init.sh"
fi

compose_args=(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" up -d)
if [ "$LAB_BUILD" = "1" ]; then
  compose_args+=(--build)
fi
"${compose_args[@]}"

docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" ps

public_url=$(awk -F= '/^PUBLIC_BASE_URL=/{print $2}' "$ENV_FILE" | tail -n1)
admin_email=$(awk -F= '/^AUTH_BOOTSTRAP_EMAIL=/{print $2}' "$ENV_FILE" | tail -n1)
admin_password=$(awk -F= '/^AUTH_BOOTSTRAP_PASSWORD=/{print $2}' "$ENV_FILE" | tail -n1)
certs_dir=$(awk -F= '/^CERTS_DIR=/{print $2}' "$ENV_FILE" | tail -n1)
ca_cert="$certs_dir/ca.crt"
public_host=$(echo "$public_url" | sed -e 's#^[a-zA-Z0-9+.-]*://##' -e 's#/.*$##' -e 's/:.*$//')

if [ -f "$ca_cert" ]; then
  echo "Waiting for control-plane healthz..."
  ready=0
  for _ in $(seq 1 60); do
    if curl --silent --show-error --fail \
      --resolve "$public_host:443:127.0.0.1" \
      --cacert "$ca_cert" \
      "$public_url/healthz" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 1
  done
  if [ "$ready" != "1" ]; then
    echo "Control-plane did not become healthy within timeout." >&2
    docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" logs control-plane --tail=200 >&2
    exit 1
  fi
fi

cat <<EOF
Prod lab is up.

URL:       $public_url
Email:     $admin_email
Password:  $admin_password

Next step:
  scripts/testing/prod-lab-smoke.sh
EOF
