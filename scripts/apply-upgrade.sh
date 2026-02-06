#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STACK_DIR=${STACK_DIR:-$BASE_DIR}
UPGRADE_UPDATES_DIR=${UPGRADE_UPDATES_DIR:-$STACK_DIR/updates}
UPDATE_TARBALL=${UPDATE_TARBALL:-}

COMPOSE_FILE=${COMPOSE_FILE:-}
ENV_FILE=${ENV_FILE:-}
PROJECT_NAME=${PROJECT_NAME:-hardwareops}

if [ -z "$UPDATE_TARBALL" ] && [ -d "$UPGRADE_UPDATES_DIR" ]; then
  UPDATE_TARBALL=$(ls -t "$UPGRADE_UPDATES_DIR"/hardwareops-upgrade-*.tar.gz 2>/dev/null | head -n 1 || true)
fi

if [ -n "$UPDATE_TARBALL" ] && [ -f "$UPDATE_TARBALL" ]; then
  echo "Found upgrade tarball: $UPDATE_TARBALL"
  STAGED_DIR="$UPGRADE_UPDATES_DIR/current"
  rm -rf "$STAGED_DIR"
  mkdir -p "$STAGED_DIR"
  tar -xzf "$UPDATE_TARBALL" -C "$STAGED_DIR"
  bundle_dir=$(find "$STAGED_DIR" -maxdepth 1 -type d -name "hardwareops-upgrade-*" | head -n 1 || true)
  if [ -n "$bundle_dir" ]; then
    STACK_DIR="$bundle_dir"
    COMPOSE_FILE=""
    ENV_FILE=""
  fi
fi

if [ -z "$COMPOSE_FILE" ]; then
  if [ -f "$STACK_DIR/docker-compose.onprem.bundle.yml" ]; then
    COMPOSE_FILE="$STACK_DIR/docker-compose.onprem.bundle.yml"
  elif [ -f "$STACK_DIR/deploy/compose/docker-compose.onprem.yml" ]; then
    COMPOSE_FILE="$STACK_DIR/deploy/compose/docker-compose.onprem.yml"
  elif [ -f "$BASE_DIR/docker-compose.onprem.bundle.yml" ]; then
    COMPOSE_FILE="$BASE_DIR/docker-compose.onprem.bundle.yml"
  elif [ -f "$BASE_DIR/deploy/compose/docker-compose.onprem.yml" ]; then
    COMPOSE_FILE="$BASE_DIR/deploy/compose/docker-compose.onprem.yml"
  fi
fi

if [ -z "$COMPOSE_FILE" ] || [ ! -f "$COMPOSE_FILE" ]; then
  echo "No compose file found. Set COMPOSE_FILE or run from a stack bundle." >&2
  exit 1
fi

if [ -z "$ENV_FILE" ]; then
  if [ -f "$STACK_DIR/.env.onprem" ]; then
    ENV_FILE="$STACK_DIR/.env.onprem"
  elif [ -f "$STACK_DIR/deploy/compose/.env.onprem.example" ]; then
    ENV_FILE="$STACK_DIR/deploy/compose/.env.onprem.example"
  elif [ -f "$BASE_DIR/.env.onprem" ]; then
    ENV_FILE="$BASE_DIR/.env.onprem"
  elif [ -f "$BASE_DIR/deploy/compose/.env.onprem.example" ]; then
    ENV_FILE="$BASE_DIR/deploy/compose/.env.onprem.example"
  fi
fi

ENV_ARGS=()
if [ -n "$ENV_FILE" ] && [ -f "$ENV_FILE" ]; then
  ENV_ARGS=(--env-file "$ENV_FILE")
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-https://localhost}
CERTS_DIR=${CERTS_DIR:-/opt/hardwareops/certs}
CA_CERT=${CA_CERT_PATH:-$CERTS_DIR/ca.crt}
MAINTENANCE_TOKEN=${MAINTENANCE_TOKEN:-}

bundle_dir=$(cd "$(dirname "$COMPOSE_FILE")" && pwd)
has_images_dir=0
if [ -d "$bundle_dir/images" ]; then
  has_images_dir=1
  echo "Loading staged images from $bundle_dir/images..."
  for img in "$bundle_dir"/images/*.tar; do
    [ -f "$img" ] || continue
    docker load -i "$img"
  done
fi

if [ -z "${PULL_IMAGES:-}" ]; then
  if [ "$has_images_dir" = "1" ]; then
    PULL_IMAGES=0
  else
    PULL_IMAGES=1
  fi
fi

echo "Applying upgrade via compose file: $COMPOSE_FILE"
if [ "$PULL_IMAGES" = "1" ]; then
  docker compose -f "$COMPOSE_FILE" "${ENV_ARGS[@]}" -p "$PROJECT_NAME" pull
fi
docker compose -f "$COMPOSE_FILE" "${ENV_ARGS[@]}" -p "$PROJECT_NAME" up -d

curl_args=(-fsS)
if [ -f "$CA_CERT" ]; then
  curl_args+=(--cacert "$CA_CERT")
else
  curl_args+=(-k)
fi

echo "Running post-upgrade checks..."
"${curl_args[@]}" "$PUBLIC_BASE_URL/healthz" >/dev/null
"${curl_args[@]}" -L "$PUBLIC_BASE_URL/" >/dev/null

set_kv() {
  local file=$1
  local key=$2
  local val=$3
  [ -f "$file" ] || return 0
  if grep -q "^${key}=" "$file"; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$file"
  else
    printf "\n%s=%s\n" "$key" "$val" >>"$file"
  fi
}

if [ -n "$ENV_FILE" ] && [ -f "$ENV_FILE" ]; then
  set_kv "$ENV_FILE" "MAINTENANCE_MODE" "0"
  set_kv "$ENV_FILE" "MAINTENANCE_MESSAGE" ""
fi

if [ -n "$MAINTENANCE_TOKEN" ]; then
  echo "Disabling maintenance mode..."
  payload='{"enabled":false,"message":""}'
  "${curl_args[@]}" -H "Content-Type: application/json" \
    -H "X-Maintenance-Token: ${MAINTENANCE_TOKEN}" \
    -d "$payload" \
    "$PUBLIC_BASE_URL/api/v1/maintenance" >/dev/null
else
  echo "MAINTENANCE_TOKEN not set; disable maintenance manually when ready."
fi

echo "Upgrade apply complete."
