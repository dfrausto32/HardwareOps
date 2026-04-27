#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

COMPOSE_FILE=${COMPOSE_FILE:-$BASE_DIR/docker-compose.onprem.bundle.yml}
ENV_EXAMPLE=${ENV_EXAMPLE:-$BASE_DIR/.env.onprem.example}
ENV_FILE=${ENV_FILE:-$BASE_DIR/.env.onprem}
CERTS_DIR=${CERTS_DIR:-/opt/parcel/certs}
DOMAIN=${DOMAIN:-parcel.internal}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-https://parcel.internal}
AGENT_BASE_URL=${AGENT_BASE_URL:-}
PROJECT_NAME=${PROJECT_NAME:-parcel}
LOAD_IMAGES=${LOAD_IMAGES:-1}
GENERATE_CERTS=${GENERATE_CERTS:-1}
ENABLE_TLS=${ENABLE_TLS:-0}
SERVER_CERT_MODE=${SERVER_CERT_MODE:-device-ca}
SERVER_CERT_INPUT=${SERVER_CERT_INPUT:-}
SERVER_KEY_INPUT=${SERVER_KEY_INPUT:-}
FIX_PERMS=${FIX_PERMS:-1}
CHOWN_UID=${CHOWN_UID:-65532}
CHOWN_GID=${CHOWN_GID:-65532}
FORCE=${FORCE:-0}

agent_domain_from_url() {
  local url="$1"
  local host="${url#*://}"
  host="${host%%/*}"
  host="${host%%:*}"
  if [ -z "$host" ]; then
    echo ""
    return
  fi
  echo "agent.$host"
}

if [ -z "$AGENT_BASE_URL" ]; then
  derived_agent_host=$(agent_domain_from_url "$PUBLIC_BASE_URL")
  if [ -n "$derived_agent_host" ]; then
    AGENT_BASE_URL="https://$derived_agent_host"
  else
    AGENT_BASE_URL="https://agent.parcel.internal"
  fi
fi

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
    OUT_DIR="$CERTS_DIR" DOMAIN="$DOMAIN" AGENT_DOMAIN="$(agent_domain_from_url "$PUBLIC_BASE_URL")" ENABLE_TLS="$ENABLE_TLS" FORCE="$FORCE" \
    SERVER_CERT_MODE="$SERVER_CERT_MODE" SERVER_CERT_INPUT="$SERVER_CERT_INPUT" SERVER_KEY_INPUT="$SERVER_KEY_INPUT" \
    "$BASE_DIR/scripts/setup-control-plane.sh"
fi

if [ ! -f "$ENV_EXAMPLE" ]; then
  echo "Env example not found: $ENV_EXAMPLE" >&2
  exit 1
fi

if [ ! -f "$ENV_FILE" ] || [ "$FORCE" = "1" ]; then
  cp "$ENV_EXAMPLE" "$ENV_FILE"
fi
python3 - "$ENV_FILE" "$CERTS_DIR" "$PUBLIC_BASE_URL" "$AGENT_BASE_URL" "$SERVER_CERT_MODE" "$SERVER_CERT_INPUT" "$SERVER_KEY_INPUT" "$BASE_DIR" "$FORCE" <<'PY'
import sys, re
path, certs, base, agent_base, server_cert_mode, server_cert_input, server_key_input, stack_dir, force = sys.argv[1:10]
force = force == "1"
def set_kv(lines, key, val, override=False):
    out = []
    found = False
    for line in lines:
        if re.match(rf"^{re.escape(key)}=", line):
            found = True
            current = line.split("=", 1)[1] if "=" in line else ""
            if override or force or current.strip() == "":
                out.append(f"{key}={val}")
            else:
                out.append(line)
        else:
            out.append(line)
    if not found:
        out.append(f"{key}={val}")
    return out

with open(path, "r") as f:
    raw = f.read()
if "\\n" in raw and "\n" not in raw:
    raw = raw.replace("\\n", "\n")
lines = raw.splitlines()
lines = set_kv(lines, "CERTS_DIR", certs)
lines = set_kv(lines, "PUBLIC_BASE_URL", base)
lines = set_kv(lines, "AGENT_BASE_URL", agent_base)
lines = set_kv(lines, "SERVER_CERT_MODE", server_cert_mode)
lines = set_kv(lines, "SERVER_CERT_INPUT", server_cert_input)
lines = set_kv(lines, "SERVER_KEY_INPUT", server_key_input)
lines = set_kv(lines, "CORS_ALLOWED_ORIGINS", base)
lines = set_kv(lines, "STACK_DIR", stack_dir)
lines = set_kv(lines, "UPGRADE_APPLY_CMD", "/app/scripts/apply-upgrade.sh")
lines = set_kv(lines, "UPGRADE_WORK_DIR", "/stack")
lines = set_kv(lines, "UPGRADE_LOG_DIR", "/var/lib/parcel/logs")
lines = set_kv(lines, "UPGRADE_UPDATES_DIR", "/stack/updates")
lines = set_kv(lines, "UPGRADE_RUNNER_MODE", "remote")
lines = set_kv(lines, "UPGRADE_RUNNER_URL", "http://maintenance-runner:8090")
lines = set_kv(lines, "UPGRADE_RUNNER_TOKEN", "change-me-maintenance-runner")
lines = set_kv(lines, "CA_BUNDLE_PATH", "/certs/ca-bundle.crt")
lines = set_kv(lines, "ACTIVE_CA_CERT_PATH", "/certs/ca-active.crt")
lines = set_kv(lines, "ACTIVE_CA_KEY_PATH", "/certs/ca-active.key")
lines = set_kv(lines, "CERT_ROTATION_GRACE_PERIOD", "168h")
lines = set_kv(lines, "BACKUP_CMD", "/app/scripts/backup-stack.sh")
lines = set_kv(lines, "RESTORE_CMD", "/app/scripts/restore-stack.sh")
lines = set_kv(lines, "BACKUP_DIR", "/stack/backups")
lines = set_kv(lines, "BACKUP_WORK_DIR", "/stack")
lines = set_kv(lines, "BACKUP_LOG_DIR", "/var/lib/parcel/logs")
lines = set_kv(lines, "BACKUP_RUNNER_MODE", "remote")
lines = set_kv(lines, "BACKUP_RUNNER_URL", "http://maintenance-runner:8090")
lines = set_kv(lines, "BACKUP_RUNNER_TOKEN", "change-me-maintenance-runner")
lines = set_kv(lines, "AUTH_MODE", "local")
lines = set_kv(lines, "BOOTSTRAP_TOKEN", "change-me-bootstrap")
lines = set_kv(lines, "MAINTENANCE_MODE", "0")
lines = set_kv(lines, "MAINTENANCE_MESSAGE", "", override=True)
with open(path, "w") as f:
    f.write("\n".join(lines) + "\n")
PY

mkdir -p "$STACK_DIR/updates"

docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" up -d

echo "Stack started. Check status:"
docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" -p "$PROJECT_NAME" ps
