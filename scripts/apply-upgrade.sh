#!/usr/bin/env bash
set -eEuo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STACK_DIR=${STACK_DIR:-$BASE_DIR}
UPGRADE_UPDATES_DIR=${UPGRADE_UPDATES_DIR:-$STACK_DIR/updates}
UPDATE_TARBALL=${UPDATE_TARBALL:-}

COMPOSE_FILE=${COMPOSE_FILE:-}
ENV_FILE=${ENV_FILE:-}
PROJECT_NAME=${PROJECT_NAME:-hardwareops}
MIN_DOCKER_API=${MIN_DOCKER_API:-1.44}
ROLLBACK_ON_FAILURE=${ROLLBACK_ON_FAILURE:-1}
UPGRADE_HEALTH_TIMEOUT=${UPGRADE_HEALTH_TIMEOUT:-120}
UPGRADE_HEALTH_INTERVAL=${UPGRADE_HEALTH_INTERVAL:-5}
UPGRADE_HEALTH_URLS=${UPGRADE_HEALTH_URLS:-}

rollback_compose=""
prev_cp_image=""
prev_gw_image=""

cleanup() {
  if [ -n "$rollback_compose" ] && [ -f "$rollback_compose" ]; then
    rm -f "$rollback_compose"
  fi
}

rollback() {
  if [ "$ROLLBACK_ON_FAILURE" != "1" ]; then
    echo "Rollback disabled (ROLLBACK_ON_FAILURE=0)." >&2
    return 0
  fi
  if [ -z "$prev_cp_image" ] && [ -z "$prev_gw_image" ]; then
    echo "No previous images detected; skipping rollback." >&2
    return 0
  fi
  rollback_compose=$(mktemp)
  {
    echo "services:"
    if [ -n "$prev_cp_image" ]; then
      echo "  control-plane:"
      echo "    image: $prev_cp_image"
    fi
    if [ -n "$prev_gw_image" ]; then
      echo "  gateway:"
      echo "    image: $prev_gw_image"
    fi
  } >"$rollback_compose"
  echo "Attempting rollback using $rollback_compose..." >&2
  docker compose -f "$COMPOSE_FILE" -f "$rollback_compose" "${ENV_ARGS[@]}" -p "$PROJECT_NAME" up -d
}

wait_for_url() {
  local url=$1
  local timeout=$2
  local interval=$3
  local start
  start=$(date +%s)
  while true; do
    if "${curl_args[@]}" -L "$url" >/dev/null 2>&1; then
      return 0
    fi
    now=$(date +%s)
    if [ $((now - start)) -ge "$timeout" ]; then
      return 1
    fi
    sleep "$interval"
  done
}

health_gate() {
  local timeout=$1
  local interval=$2
  shift 2
  local urls=("$@")
  if [ "${#urls[@]}" -eq 0 ]; then
    return 0
  fi
  for url in "${urls[@]}"; do
    echo "Waiting for healthy: $url"
    if ! wait_for_url "$url" "$timeout" "$interval"; then
      echo "Health check failed: $url" >&2
      return 1
    fi
  done
}

on_error() {
  echo "Upgrade failed; attempting rollback..." >&2
  rollback || true
  cleanup
  exit 1
}

trap on_error ERR

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
    echo "$bundle_dir" >"$STAGED_DIR/ACTIVE"
  else
    compose_path=$(find "$STAGED_DIR" -maxdepth 3 -type f -name "docker-compose.onprem.bundle.yml" | head -n 1 || true)
    if [ -n "$compose_path" ]; then
      STACK_DIR=$(cd "$(dirname "$compose_path")" && pwd)
      COMPOSE_FILE=""
      ENV_FILE=""
      echo "$STACK_DIR" >"$STAGED_DIR/ACTIVE"
    fi
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

if [ -n "$COMPOSE_FILE" ] && [ ! -f "$COMPOSE_FILE" ]; then
  echo "COMPOSE_FILE not found: $COMPOSE_FILE (will auto-discover in stack dir)" >&2
  COMPOSE_FILE=""
fi

if [ -z "$COMPOSE_FILE" ] || [ ! -f "$COMPOSE_FILE" ]; then
  echo "No compose file found. Set COMPOSE_FILE or run from a stack bundle." >&2
  exit 1
fi

if [ -n "$ENV_FILE" ] && [ ! -f "$ENV_FILE" ]; then
  echo "Env file not found: $ENV_FILE (will auto-discover)" >&2
  ENV_FILE=""
fi

if [ -z "$ENV_FILE" ]; then
  if [ -f "$STACK_DIR/.env.onprem" ]; then
    ENV_FILE="$STACK_DIR/.env.onprem"
  elif [ -f "$STACK_DIR/.env.onprem.example" ]; then
    ENV_FILE="$STACK_DIR/.env.onprem.example"
  elif [ -f "$STACK_DIR/control-plane.env" ]; then
    ENV_FILE="$STACK_DIR/control-plane.env"
  elif [ -f "$STACK_DIR/deploy/compose/.env.onprem.example" ]; then
    ENV_FILE="$STACK_DIR/deploy/compose/.env.onprem.example"
  elif [ -f "$BASE_DIR/.env.onprem" ]; then
    ENV_FILE="$BASE_DIR/.env.onprem"
  elif [ -f "$BASE_DIR/control-plane.env" ]; then
    ENV_FILE="$BASE_DIR/control-plane.env"
  elif [ -f "$BASE_DIR/deploy/compose/.env.onprem.example" ]; then
    ENV_FILE="$BASE_DIR/deploy/compose/.env.onprem.example"
  fi
fi

if [ -z "$ENV_FILE" ] && [ "${ALLOW_NO_ENV:-0}" != "1" ]; then
  echo "Env file not set. Set ENV_FILE or ensure /stack/.env.onprem exists." >&2
  exit 1
fi

if command -v docker >/dev/null 2>&1; then
  client_api=$(docker version --format '{{.Client.APIVersion}}' 2>/dev/null || true)
  if [ -n "$client_api" ]; then
    min_ok=$(printf '%s\n' "$MIN_DOCKER_API" "$client_api" | sort -V | head -n 1)
    if [ "$min_ok" != "$MIN_DOCKER_API" ]; then
      echo "Docker client API $client_api is too old (need >= $MIN_DOCKER_API)." >&2
      echo "Rebuild the control-plane image (includes newer docker CLI) or run apply-upgrade.sh on a host with a newer Docker client." >&2
      exit 1
    fi
  else
    echo "Unable to detect docker client API version; continuing." >&2
  fi
else
  echo "Docker CLI not found; apply-upgrade requires docker." >&2
  exit 1
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

cp_cid=$(docker compose -f "$COMPOSE_FILE" "${ENV_ARGS[@]}" -p "$PROJECT_NAME" ps -q control-plane 2>/dev/null || true)
if [ -n "$cp_cid" ]; then
  prev_cp_image=$(docker inspect --format '{{.Config.Image}}' "$cp_cid" 2>/dev/null || true)
fi
gw_cid=$(docker compose -f "$COMPOSE_FILE" "${ENV_ARGS[@]}" -p "$PROJECT_NAME" ps -q gateway 2>/dev/null || true)
if [ -n "$gw_cid" ]; then
  prev_gw_image=$(docker inspect --format '{{.Config.Image}}' "$gw_cid" 2>/dev/null || true)
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

if [ -z "$UPGRADE_HEALTH_URLS" ]; then
  UPGRADE_HEALTH_URLS="$PUBLIC_BASE_URL/healthz,$PUBLIC_BASE_URL/"
fi

IFS=',' read -r -a HEALTH_URLS <<<"$UPGRADE_HEALTH_URLS"

echo "Running post-upgrade checks..."
if ! health_gate "$UPGRADE_HEALTH_TIMEOUT" "$UPGRADE_HEALTH_INTERVAL" "${HEALTH_URLS[@]}"; then
  echo "Post-upgrade health checks failed." >&2
  on_error
fi

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
cleanup
