#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

echo "HardwareOps stack installer"
echo "Bundle: $BASE_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Installing Docker..."
  sudo "$BASE_DIR/scripts/install-docker-ubuntu.sh"
fi

echo "Starting stack..."
sudo STACK_DIR="$BASE_DIR" "$BASE_DIR/scripts/run-stack.sh"

echo ""
echo "Stack is up."
echo "UI: https://hardwareops.internal"
echo "Health: https://hardwareops.internal/healthz"
