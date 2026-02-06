#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

COMPOSE_FILE=${COMPOSE_FILE:-$BASE_DIR/docker-compose.onprem.bundle.yml}
ENV_EXAMPLE=${ENV_EXAMPLE:-$BASE_DIR/.env.onprem.example}
ENV_FILE=${ENV_FILE:-$BASE_DIR/.env.onprem}
CERTS_DIR=${CERTS_DIR:-/opt/hardwareops/certs}
DOMAIN=${DOMAIN:-hardwareops.internal}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-https://hardwareops.internal}
PROJECT_NAME=${PROJECT_NAME:-hardwareops}
LOAD_IMAGES=${LOAD_IMAGES:-1}
GENERATE_CERTS=${GENERATE_CERTS:-1}
ENABLE_TLS=${ENABLE_TLS:-0}
FIX_PERMS=${FIX_PERMS:-1}
CHOWN_UID=${CHOWN_UID:-65532}
CHOWN_GID=${CHOWN_GID:-65532}
FORCE=${FORCE:-0}

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Install it first (see scripts/install-docker-ubuntu.sh)." >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "Compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

if [ "$LOAD_IMAGES" = "1" ] && [ -d "$BASE_DIR/images" ]; then
  for img in "$BASE_DIR"/images/*.tar; do
    [ -f "$img" ] || continue
    docker load -i "$img"
  done
fi

if [ "$GENERATE_CERTS" = "1" ]; then
  if [ ! -x "$BASE_DIR/scripts/setup-control-plane.sh" ]; then
    echo "setup-control-plane.sh not found in $BASE_DIR/scripts" >&2
    exit 1
  fi
  FIX_PERMS="$FIX_PERMS" CHOWN_UID="$CHOWN_UID" CHOWN_GID="$CHOWN_GID" \
    OUT_DIR="$CERTS_DIR" DOMAIN="$DOMAIN" ENABLE_TLS="$ENABLE_TLS" FORCE="$FORCE" \
    "$BASE_DIR/scripts/setup-control-plane.sh"
fi

if [ ! -f "$ENV_EXAMPLE" ]; then
  echo "Env example not found: $ENV_EXAMPLE" >&2
  exit 1
fi

if [ ! -f "$ENV_FILE" ] || [ "$FORCE" = "1" ]; then
  cp "$ENV_EXAMPLE" "$ENV_FILE"
  python3 - "$ENV_FILE" "$CERTS_DIR" "$PUBLIC_BASE_URL" "$BASE_DIR" <<'PY'
import sys, re
path, certs, base, stack_dir = sys.argv[1:5]
def set_kv(lines, key, val):
    out = []
    found = False
    for line in lines:
        if re.match(rf"^{re.escape(key)}=", line):
            out.append(f"{key}={val}")
            found = True
        else:
            out.append(line)
    if not found:
        out.append(f"{key}={val}")
    return out

with open(path, "r") as f:
    lines = f.read().splitlines()
lines = set_kv(lines, "CERTS_DIR", certs)
lines = set_kv(lines, "PUBLIC_BASE_URL", base)
lines = set_kv(lines, "CORS_ALLOWED_ORIGINS", base)
lines = set_kv(lines, "STACK_DIR", stack_dir)
lines = set_kv(lines, "UPGRADE_APPLY_CMD", "/app/scripts/apply-upgrade.sh")
lines = set_kv(lines, "UPGRADE_WORK_DIR", "/stack")
lines = set_kv(lines, "UPGRADE_LOG_DIR", "/var/lib/hardwareops/logs")
lines = set_kv(lines, "UPGRADE_UPDATES_DIR", "/stack/updates")
with open(path, "w") as f:
    f.write("\n".join(lines) + "\n")
PY
fi

docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" up -d

echo "Stack started. Check status:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" ps
