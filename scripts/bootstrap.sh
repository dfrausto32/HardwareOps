#!/usr/bin/env bash
set -euo pipefail

missing=()

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    missing+=("$1")
  fi
}

need_cmd go
need_cmd docker
need_cmd node
need_cmd npm

if ! docker compose version >/dev/null 2>&1; then
  missing+=("docker compose")
fi

if [ ${#missing[@]} -ne 0 ]; then
  echo "Missing dependencies:" >&2
  for m in "${missing[@]}"; do
    echo "  - $m" >&2
  done
  exit 1
fi

# Create local env files if missing
if [ ! -f deploy/compose/.env ]; then
  cp deploy/compose/.env.example deploy/compose/.env
  echo "Created deploy/compose/.env from example."
fi

echo "Bootstrap complete."
