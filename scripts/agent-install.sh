#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  sudo ./scripts/agent-install.sh KEY=VALUE ...

Common keys:
  AGENT_SRC=/path/to/parcel-agent
  CONTROL_PLANE_URL=https://agent.parcel.internal
  CONTROL_PLANE_CA_CERT_SRC=/path/to/ca.crt
  USE_SYSTEM_CA=1
  AGENT_ENROLL_MODE=approval
  ENROLLMENT_PROFILE_TOKEN=ep_tok_...
  START_SERVICE=1

Notes:
  - KEY=VALUE arguments are accepted after the script so sudo does not need env_keep.
  - Legacy direct enrollment is still available via scripts/agent-enroll.sh.
EOF
}

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
    *=*)
      export "${arg%%=*}=${arg#*=}"
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

AGENT_SRC=${AGENT_SRC:-}
AGENT_BIN=${AGENT_BIN:-/usr/local/bin/parcel-agent}
AGENT_USER=${AGENT_USER:-parcel}
AGENT_GROUP=${AGENT_GROUP:-parcel}
SERVICE_NAME=${SERVICE_NAME:-parcel-agent}
SERVICE_UNIT_PATH=${SERVICE_UNIT_PATH:-/etc/systemd/system/${SERVICE_NAME}.service}
MACHINE_ID_INIT_SERVICE_NAME=${MACHINE_ID_INIT_SERVICE_NAME:-parcel-machine-id-init}
MACHINE_ID_INIT_UNIT_PATH=${MACHINE_ID_INIT_UNIT_PATH:-/etc/systemd/system/${MACHINE_ID_INIT_SERVICE_NAME}.service}
MACHINE_ID_INIT_BIN=${MACHINE_ID_INIT_BIN:-/usr/local/bin/parcel-machine-id-init.sh}
SYSTEMCTL_BIN=${SYSTEMCTL_BIN:-systemctl}
ENABLE_SERVICE=${ENABLE_SERVICE:-1}
START_SERVICE=${START_SERVICE:-0}
CONFIG_DIR=${CONFIG_DIR:-/etc/parcel/agent}
ENV_FILE=${ENV_FILE:-$CONFIG_DIR/agent.env}
DATA_DIR=${DATA_DIR:-/var/lib/parcel/agent}
CERT_DIR=${CERT_DIR:-/etc/parcel/agent/certs}
STATE_PATH=${STATE_PATH:-$DATA_DIR/state.json}
ARTIFACT_ROOT=${ARTIFACT_ROOT:-$DATA_DIR/artifacts}
DEVICE_CERT_PATH=${DEVICE_CERT_PATH:-$CERT_DIR/device.crt}
DEVICE_KEY_PATH=${DEVICE_KEY_PATH:-$CERT_DIR/device.key}
DEVICE_ID_PATH=${DEVICE_ID_PATH:-$DATA_DIR/device-id}
BOOTSTRAP_STATE_PATH=${BOOTSTRAP_STATE_PATH:-$DATA_DIR/bootstrap-state.json}
CONTROL_PLANE_URL=${CONTROL_PLANE_URL:-}
CONTROL_PLANE_CA_CERT_PATH=${CONTROL_PLANE_CA_CERT_PATH:-$CERT_DIR/ca.crt}
CONTROL_PLANE_CA_CERT_SRC=${CONTROL_PLANE_CA_CERT_SRC:-}
USE_SYSTEM_CA=${USE_SYSTEM_CA:-0}
AGENT_ENROLL_MODE=${AGENT_ENROLL_MODE:-}
ENROLLMENT_PROFILE_TOKEN=${ENROLLMENT_PROFILE_TOKEN:-}
CHECKIN_INTERVAL=${CHECKIN_INTERVAL:-}
BOOTSTRAP_POLL_INTERVAL=${BOOTSTRAP_POLL_INTERVAL:-}
BOOTSTRAP_RETRY_INTERVAL=${BOOTSTRAP_RETRY_INTERVAL:-}
LOG_LEVEL=${LOG_LEVEL:-}
AUTO_REENROLL=${AUTO_REENROLL:-}
ENV_TEMPLATE_PATH=${ENV_TEMPLATE_PATH:-}
SERVICE_TEMPLATE_PATH=${SERVICE_TEMPLATE_PATH:-}

if [ -n "$ENROLLMENT_PROFILE_TOKEN" ] && [ -z "$AGENT_ENROLL_MODE" ]; then
  AGENT_ENROLL_MODE=approval
fi
if [ -z "$CONTROL_PLANE_URL" ]; then
  CONTROL_PLANE_URL=https://agent.parcel.internal
fi
if [ -z "$ENV_TEMPLATE_PATH" ]; then
  if [ -f "$BASE_DIR/deploy/systemd/agent.env.example" ]; then
    ENV_TEMPLATE_PATH=$BASE_DIR/deploy/systemd/agent.env.example
  elif [ -f "$BASE_DIR/agent.env.example" ]; then
    ENV_TEMPLATE_PATH=$BASE_DIR/agent.env.example
  else
    echo "agent.env.example not found beside installer script." >&2
    exit 1
  fi
fi
if [ -z "$SERVICE_TEMPLATE_PATH" ]; then
  if [ -f "$BASE_DIR/deploy/systemd/parcel-agent.service" ]; then
    SERVICE_TEMPLATE_PATH=$BASE_DIR/deploy/systemd/parcel-agent.service
  elif [ -f "$BASE_DIR/parcel-agent.service" ]; then
    SERVICE_TEMPLATE_PATH=$BASE_DIR/parcel-agent.service
  else
    echo "parcel-agent.service not found beside installer script." >&2
    exit 1
  fi
fi

MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH=${MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH:-}
if [ -z "$MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH" ]; then
  if [ -f "$BASE_DIR/deploy/systemd/parcel-machine-id-init.service" ]; then
    MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH=$BASE_DIR/deploy/systemd/parcel-machine-id-init.service
  elif [ -f "$BASE_DIR/parcel-machine-id-init.service" ]; then
    MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH=$BASE_DIR/parcel-machine-id-init.service
  fi
fi

MACHINE_ID_INIT_SCRIPT_SRC=${MACHINE_ID_INIT_SCRIPT_SRC:-}
if [ -z "$MACHINE_ID_INIT_SCRIPT_SRC" ]; then
  if [ -f "$BASE_DIR/scripts/parcel-machine-id-init.sh" ]; then
    MACHINE_ID_INIT_SCRIPT_SRC=$BASE_DIR/scripts/parcel-machine-id-init.sh
  elif [ -f "$BASE_DIR/parcel-machine-id-init.sh" ]; then
    MACHINE_ID_INIT_SCRIPT_SRC=$BASE_DIR/parcel-machine-id-init.sh
  fi
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

upsert_env() {
  local file=$1
  local key=$2
  local value=$3
  local tmp
  tmp=$(mktemp)
  if [ -f "$file" ]; then
    awk -F= -v key="$key" -v value="$value" '
      BEGIN { replaced = 0 }
      $1 == key {
        print key "=" value
        replaced = 1
        next
      }
      { print }
      END {
        if (!replaced) {
          print key "=" value
        }
      }
    ' "$file" >"$tmp"
  else
    printf '%s=%s\n' "$key" "$value" >"$tmp"
  fi
  install -m 0644 "$tmp" "$file"
  rm -f "$tmp"
}

remove_env() {
  local file=$1
  local key=$2
  local tmp
  tmp=$(mktemp)
  if [ -f "$file" ]; then
    awk -F= -v key="$key" '$1 != key { print }' "$file" >"$tmp"
    install -m 0644 "$tmp" "$file"
  fi
  rm -f "$tmp"
}

if ! id -u "$AGENT_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$AGENT_USER"
fi

mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$CERT_DIR"
mkdir -p "$(dirname "$AGENT_BIN")"
mkdir -p "$(dirname "$ENV_FILE")"
chown -R "$AGENT_USER:$AGENT_GROUP" "$DATA_DIR"
chown -R "$AGENT_USER:$AGENT_GROUP" "$CERT_DIR"
chmod 0700 "$CERT_DIR"

if [ -z "$AGENT_SRC" ]; then
  echo "AGENT_SRC is required (path to parcel-agent binary)." >&2
  exit 1
fi

install -m 0755 "$AGENT_SRC" "$AGENT_BIN"

if [ ! -f "$ENV_FILE" ]; then
  install -m 0644 "$ENV_TEMPLATE_PATH" "$ENV_FILE"
fi

if [ "$USE_SYSTEM_CA" != "1" ] && [ -n "$CONTROL_PLANE_CA_CERT_SRC" ]; then
  if [ ! -f "$CONTROL_PLANE_CA_CERT_SRC" ]; then
    echo "CONTROL_PLANE_CA_CERT_SRC not found: $CONTROL_PLANE_CA_CERT_SRC" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$CONTROL_PLANE_CA_CERT_PATH")"
  install -m 0644 "$CONTROL_PLANE_CA_CERT_SRC" "$CONTROL_PLANE_CA_CERT_PATH"
fi

upsert_env "$ENV_FILE" "CONTROL_PLANE_URL" "$CONTROL_PLANE_URL"
upsert_env "$ENV_FILE" "STATE_PATH" "$STATE_PATH"
upsert_env "$ENV_FILE" "ARTIFACT_ROOT" "$ARTIFACT_ROOT"
upsert_env "$ENV_FILE" "DEVICE_CERT_PATH" "$DEVICE_CERT_PATH"
upsert_env "$ENV_FILE" "DEVICE_KEY_PATH" "$DEVICE_KEY_PATH"
upsert_env "$ENV_FILE" "DEVICE_ID_PATH" "$DEVICE_ID_PATH"
upsert_env "$ENV_FILE" "BOOTSTRAP_STATE_PATH" "$BOOTSTRAP_STATE_PATH"
if [ "$USE_SYSTEM_CA" = "1" ]; then
  remove_env "$ENV_FILE" "CONTROL_PLANE_CA_CERT_PATH"
else
  upsert_env "$ENV_FILE" "CONTROL_PLANE_CA_CERT_PATH" "$CONTROL_PLANE_CA_CERT_PATH"
fi

if [ -n "$AGENT_ENROLL_MODE" ]; then
  upsert_env "$ENV_FILE" "AGENT_ENROLL_MODE" "$AGENT_ENROLL_MODE"
fi
if [ -n "$ENROLLMENT_PROFILE_TOKEN" ] || [ "$AGENT_ENROLL_MODE" = "approval" ]; then
  upsert_env "$ENV_FILE" "ENROLLMENT_PROFILE_TOKEN" "$ENROLLMENT_PROFILE_TOKEN"
fi
if [ -n "$CHECKIN_INTERVAL" ]; then
  upsert_env "$ENV_FILE" "CHECKIN_INTERVAL" "$CHECKIN_INTERVAL"
fi
if [ -n "$BOOTSTRAP_POLL_INTERVAL" ]; then
  upsert_env "$ENV_FILE" "BOOTSTRAP_POLL_INTERVAL" "$BOOTSTRAP_POLL_INTERVAL"
fi
if [ -n "$BOOTSTRAP_RETRY_INTERVAL" ]; then
  upsert_env "$ENV_FILE" "BOOTSTRAP_RETRY_INTERVAL" "$BOOTSTRAP_RETRY_INTERVAL"
fi
if [ -n "$LOG_LEVEL" ]; then
  upsert_env "$ENV_FILE" "LOG_LEVEL" "$LOG_LEVEL"
fi
if [ -n "$AUTO_REENROLL" ]; then
  upsert_env "$ENV_FILE" "AUTO_REENROLL" "$AUTO_REENROLL"
fi

# Install machine-id init script and service (used for ISO/cloned-image deployments).
if [ -n "$MACHINE_ID_INIT_SCRIPT_SRC" ]; then
  install -m 0755 "$MACHINE_ID_INIT_SCRIPT_SRC" "$MACHINE_ID_INIT_BIN"
fi
if [ -n "$MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH" ]; then
  mkdir -p "$(dirname "$MACHINE_ID_INIT_UNIT_PATH")"
  install -m 0644 "$MACHINE_ID_INIT_SERVICE_TEMPLATE_PATH" "$MACHINE_ID_INIT_UNIT_PATH"
fi

mkdir -p "$(dirname "$SERVICE_UNIT_PATH")"
install -m 0644 "$SERVICE_TEMPLATE_PATH" "$SERVICE_UNIT_PATH"

if id -u "$AGENT_USER" >/dev/null 2>&1; then
  chown -R "$AGENT_USER:$AGENT_GROUP" "$DATA_DIR" "$CERT_DIR"
  if [ "$USE_SYSTEM_CA" != "1" ] && [ -f "$CONTROL_PLANE_CA_CERT_PATH" ]; then
    chown "$AGENT_USER:$AGENT_GROUP" "$CONTROL_PLANE_CA_CERT_PATH"
  fi
fi

if [ "$ENABLE_SERVICE" = "1" ]; then
  "$SYSTEMCTL_BIN" daemon-reload
  if [ -f "$MACHINE_ID_INIT_UNIT_PATH" ]; then
    "$SYSTEMCTL_BIN" enable "$MACHINE_ID_INIT_SERVICE_NAME"
  fi
  "$SYSTEMCTL_BIN" enable "$SERVICE_NAME"
  if [ "$START_SERVICE" = "1" ]; then
    "$SYSTEMCTL_BIN" restart "$SERVICE_NAME"
  fi
fi

echo "Installed parcel-agent."
echo "Binary: $AGENT_BIN"
echo "Env file: $ENV_FILE"

if [ "$AGENT_ENROLL_MODE" = "approval" ]; then
  echo "Approval bootstrap configured:"
  echo "  Control plane URL: $CONTROL_PLANE_URL"
  if [ "$USE_SYSTEM_CA" = "1" ]; then
    echo "  Trust anchor:      system trust store"
  else
    echo "  Trust anchor:      $CONTROL_PLANE_CA_CERT_PATH"
  fi
  if [ -n "$ENROLLMENT_PROFILE_TOKEN" ]; then
    echo "  Profile token:     installed in agent.env"
  else
    echo "  Profile token:     not set (agent will stay blocked until configured)"
  fi
  if [ "$ENABLE_SERVICE" = "1" ] && [ "$START_SERVICE" = "1" ]; then
    echo "Service started. Follow with: journalctl -u $SERVICE_NAME -f"
  elif [ "$ENABLE_SERVICE" = "1" ]; then
    echo "Next: sudo systemctl start $SERVICE_NAME"
  else
    echo "Service management skipped (ENABLE_SERVICE=$ENABLE_SERVICE)."
  fi
else
  echo "Legacy direct enrollment is still available via: ./scripts/agent-enroll.sh"
  if [ "$ENABLE_SERVICE" = "1" ] && [ "$START_SERVICE" = "1" ]; then
    echo "Service started. Follow with: journalctl -u $SERVICE_NAME -f"
  elif [ "$ENABLE_SERVICE" = "1" ]; then
    echo "Next: run scripts/agent-enroll.sh, then start $SERVICE_NAME."
  else
    echo "Service management skipped (ENABLE_SERVICE=$ENABLE_SERVICE)."
  fi
fi
