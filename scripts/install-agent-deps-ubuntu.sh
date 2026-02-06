#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

apt-get update
apt-get install -y openssl curl python3

echo "Agent dependencies installed."
