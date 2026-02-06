#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

SIGNING_DIR=${SIGNING_DIR:-/tmp/hardwareops-demo/signing}
SIGNING_KEY=${SIGNING_KEY:-$SIGNING_DIR/ed25519.key}
SIGNING_PUB=${SIGNING_PUB_KEY_PATH:-$SIGNING_DIR/ed25519.pub}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}

if [ ! -s "$SIGNING_KEY" ] || [ ! -s "$SIGNING_PUB" ]; then
  bash "$BASE_DIR/scripts/generate-signing-key.sh" "$SIGNING_DIR" >/dev/null
fi

if [ -z "$SIGNING_KEY_ID" ] && [ -f "$SIGNING_DIR/ed25519.keyid" ]; then
  SIGNING_KEY_ID=$(cat "$SIGNING_DIR/ed25519.keyid")
fi

export SIGNING_DIR
export SIGNING_KEY
export SIGNING_PUB
export SIGNING_PUB_KEY_PATH="$SIGNING_PUB"
export SIGNING_KEY_ID
