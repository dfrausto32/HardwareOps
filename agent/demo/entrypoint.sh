#!/usr/bin/env sh
set -euo pipefail

PORT=${DEMO_HTTP_PORT:-8081}
ROOT_DIR=${ARTIFACT_ROOT:-/data/artifacts}
PLACEHOLDER_DIR="$ROOT_DIR/empty"

mkdir -p "$PLACEHOLDER_DIR/files"
if [ ! -L "$ROOT_DIR/current" ]; then
  ln -s "$PLACEHOLDER_DIR" "$ROOT_DIR/current" 2>/dev/null || true
fi

DOC_ROOT="$ROOT_DIR/current/files"
mkdir -p "$DOC_ROOT"

python3 -m http.server "$PORT" --directory "$DOC_ROOT" >/dev/null 2>&1 &

exec /usr/local/bin/agent
