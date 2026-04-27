#!/usr/bin/env bash
set -euo pipefail

# Stop compose and remove volumes
( cd deploy/compose && docker compose down -v )

# Remove local dev artifacts
rm -f ./dev-ca.crt ./dev-ca.key
rm -f ./agent/agent-state.json ./agent-state.json

# Remove temp artifacts from curl quickstart
rm -rf /tmp/parcel

# Remove agent artifact data
rm -rf ./agent-data

echo "Dev reset complete."
