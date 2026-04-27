#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

echo "Parcel stack installer"
echo "Bundle: $BASE_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Installing Docker..."
  sudo "$BASE_DIR/scripts/install-docker-ubuntu.sh"
fi

echo "Starting stack..."
sudo STACK_DIR="$BASE_DIR" \
  COMPOSE_FILE="${COMPOSE_FILE:-$BASE_DIR/docker-compose.onprem.bundle.yml}" \
  ENV_EXAMPLE="${ENV_EXAMPLE:-$BASE_DIR/.env.onprem.example}" \
  ENV_FILE="${ENV_FILE:-$BASE_DIR/.env.onprem}" \
  CERTS_DIR="${CERTS_DIR:-/opt/parcel/certs}" \
  DOMAIN="${DOMAIN:-parcel.internal}" \
  PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-https://parcel.internal}" \
  AGENT_BASE_URL="${AGENT_BASE_URL:-https://agent.parcel.internal}" \
  PROJECT_NAME="${PROJECT_NAME:-parcel}" \
  LOAD_IMAGES="${LOAD_IMAGES:-1}" \
  GENERATE_CERTS="${GENERATE_CERTS:-1}" \
  ENABLE_TLS="${ENABLE_TLS:-0}" \
  SERVER_CERT_MODE="${SERVER_CERT_MODE:-device-ca}" \
  SERVER_CERT_INPUT="${SERVER_CERT_INPUT:-}" \
  SERVER_KEY_INPUT="${SERVER_KEY_INPUT:-}" \
  FIX_PERMS="${FIX_PERMS:-1}" \
  CHOWN_UID="${CHOWN_UID:-65532}" \
  CHOWN_GID="${CHOWN_GID:-65532}" \
  FORCE="${FORCE:-0}" \
  "$BASE_DIR/scripts/run-stack.sh"

echo ""
echo "Stack is up."
echo "UI: https://parcel.internal"
echo "Health: https://parcel.internal/healthz"
