#!/usr/bin/env bash
set -euo pipefail

OUT_DIR=${1:-./signing}
mkdir -p "$OUT_DIR"

KEY_PATH="$OUT_DIR/ed25519.key"
PUB_PATH="$OUT_DIR/ed25519.pub"
KEY_ID_PATH="$OUT_DIR/ed25519.keyid"

openssl genpkey -algorithm Ed25519 -out "$KEY_PATH" >/dev/null 2>&1
openssl pkey -in "$KEY_PATH" -pubout -out "$PUB_PATH" >/dev/null 2>&1

KEY_ID=$(openssl pkey -in "$KEY_PATH" -pubout -outform DER \
  | openssl dgst -sha256 -binary \
  | od -An -tx1 \
  | tr -d ' \n')

echo "sha256:${KEY_ID}" > "$KEY_ID_PATH"

echo "Generated:"
echo "  Private key: $KEY_PATH"
echo "  Public key:  $PUB_PATH"
echo "  Key ID:      $(cat "$KEY_ID_PATH")"
