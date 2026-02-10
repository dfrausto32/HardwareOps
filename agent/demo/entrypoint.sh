#!/usr/bin/env sh
set -euo pipefail

BASE_PORT=${DEMO_BASE_PORT:-${DEMO_HTTP_PORT:-8081}}
PORT_STEP=${DEMO_PORT_STEP:-10}
COMPONENTS=${DEMO_COMPONENTS:-agent_bundle}
ROOT_DIR=${ARTIFACT_ROOT:-/data/artifacts}
PLACEHOLDER_DIR="$ROOT_DIR/empty"

mkdir -p "$PLACEHOLDER_DIR/files"

component_list=$(echo "$COMPONENTS" | tr ',' ' ')
idx=0
for comp in $component_list; do
  comp=$(echo "$comp" | xargs)
  [ -z "$comp" ] && continue
  if [ "$comp" = "app_bundle" ]; then
    if [ ! -L "$ROOT_DIR/current" ]; then
      ln -s "$PLACEHOLDER_DIR" "$ROOT_DIR/current" 2>/dev/null || true
    fi
    DOC_ROOT="$ROOT_DIR/current/files"
  else
    comp_root="$ROOT_DIR/components/$comp"
    mkdir -p "$comp_root"
    if [ ! -L "$comp_root/current" ]; then
      ln -s "$PLACEHOLDER_DIR" "$comp_root/current" 2>/dev/null || true
    fi
    DOC_ROOT="$comp_root/current/files"
  fi
  mkdir -p "$DOC_ROOT"
  port=$((BASE_PORT + idx * PORT_STEP))
  python3 -m http.server "$port" --directory "$DOC_ROOT" >/dev/null 2>&1 &
  idx=$((idx + 1))
done

exec /usr/local/bin/agent
