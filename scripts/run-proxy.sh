#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

ENV_FILE="$BASE_DIR/deploy/compose/.env"
if [ -f "$ENV_FILE" ]; then
  ENV_ARG=(--env-file "$ENV_FILE")
else
  ENV_ARG=()
fi

DEFAULT_CA="$BASE_DIR/dev-ca.crt"
if [ -z "${CADDY_CLIENT_CA_PATH:-}" ]; then
  export CADDY_CLIENT_CA_PATH="$DEFAULT_CA"
fi

if [ ! -f "$CADDY_CLIENT_CA_PATH" ]; then
  echo "CADDY_CLIENT_CA_PATH not found at $CADDY_CLIENT_CA_PATH. Run ./scripts/run-control-plane.sh first." >&2
  exit 1
fi

if [ -d "$CADDY_CLIENT_CA_PATH" ]; then
  echo "CADDY_CLIENT_CA_PATH points to a directory: $CADDY_CLIENT_CA_PATH" >&2
  echo "Remove that directory and ensure it is a file (dev-ca.crt) in the repo root." >&2
  exit 1
fi

docker compose "${ENV_ARG[@]}" -f "$BASE_DIR/deploy/compose/docker-compose.proxy.yml" up -d "$@"
