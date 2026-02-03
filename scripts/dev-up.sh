#!/usr/bin/env bash
set -euo pipefail

if [ ! -f deploy/compose/.env ]; then
  echo "Missing deploy/compose/.env. Run ./scripts/bootstrap.sh first." >&2
  exit 1
fi

( cd deploy/compose && docker compose up -d )
