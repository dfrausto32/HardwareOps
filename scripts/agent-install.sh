#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

AGENT_SRC=${AGENT_SRC:-}
AGENT_BIN=${AGENT_BIN:-/usr/local/bin/hardwareops-agent}
AGENT_USER=${AGENT_USER:-hardwareops}
AGENT_GROUP=${AGENT_GROUP:-hardwareops}
CONFIG_DIR=${CONFIG_DIR:-/etc/hardwareops/agent}
DATA_DIR=${DATA_DIR:-/var/lib/hardwareops/agent}
CERT_DIR=${CERT_DIR:-/etc/hardwareops/agent/certs}

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

if ! id -u "$AGENT_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$AGENT_USER"
fi

mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$CERT_DIR"
chown -R "$AGENT_USER:$AGENT_GROUP" "$DATA_DIR"
chown -R "$AGENT_USER:$AGENT_GROUP" "$CERT_DIR"
chmod 0700 "$CERT_DIR"

if [ -z "$AGENT_SRC" ]; then
  echo "AGENT_SRC is required (path to hardwareops-agent binary)." >&2
  exit 1
fi

install -m 0755 "$AGENT_SRC" "$AGENT_BIN"

if [ ! -f "$CONFIG_DIR/agent.env" ]; then
  cp "$BASE_DIR/deploy/systemd/agent.env.example" "$CONFIG_DIR/agent.env"
fi

install -m 0644 "$BASE_DIR/deploy/systemd/hardwareops-agent.service" /etc/systemd/system/hardwareops-agent.service

systemctl daemon-reload
systemctl enable hardwareops-agent

echo "Installed hardwareops-agent."
echo "Next: run scripts/agent-enroll.sh to enroll and create device certs."
