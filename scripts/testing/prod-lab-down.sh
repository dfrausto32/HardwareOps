#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$BASE_DIR/deploy/compose/docker-compose.onprem.yml"}
LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/hardwareops-prod-docker"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
PROJECT_NAME=${PROJECT_NAME:-hwops-prodtest}
WIPE=${WIPE:-0}

if [ ! -f "$ENV_FILE" ]; then
  echo "Lab env file not found: $ENV_FILE" >&2
  exit 1
fi

down_args=(docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" down)
if [ "$WIPE" = "1" ]; then
  down_args+=(-v)
fi
"${down_args[@]}"

if [ "$WIPE" = "1" ]; then
  rm -rf "$LAB_ROOT"
fi

echo "Prod lab stopped (wipe=$WIPE)."
