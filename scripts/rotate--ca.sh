#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

CERTS_DIR=${CERTS_DIR:-}
if [ -z "$CERTS_DIR" ]; then
  if [ -d "/opt/hardwareops/certs" ]; then
    CERTS_DIR="/opt/hardwareops/certs"
  else
    CERTS_DIR="$BASE_DIR"
  fi
fi

if [ -n "${CA_PREFIX:-}" ]; then
  PREFIX="$CA_PREFIX"
elif [ -f "$CERTS_DIR/dev-ca.crt" ]; then
  PREFIX="dev-ca"
else
  PREFIX="ca"
fi

OLD_CA_CERT=${CA_CERT_PATH:-$CERTS_DIR/$PREFIX.crt}
OLD_CA_KEY=${CA_KEY_PATH:-$CERTS_DIR/$PREFIX.key}
ACTIVE_CA_CERT=${ACTIVE_CA_CERT_PATH:-$CERTS_DIR/${PREFIX}-active.crt}
ACTIVE_CA_KEY=${ACTIVE_CA_KEY_PATH:-$CERTS_DIR/${PREFIX}-active.key}
CA_BUNDLE_PATH=${CA_BUNDLE_PATH:-$CERTS_DIR/${PREFIX}-bundle.crt}
BASE_URL=${BASE_URL:-https://localhost:8080}
AUTH_TOKEN=${AUTH_TOKEN:-}

if [ ! -f "$OLD_CA_CERT" ] || [ ! -f "$OLD_CA_KEY" ]; then
  echo "Current CA not found. Expected $OLD_CA_CERT and $OLD_CA_KEY" >&2
  exit 1
fi

tmp_dir=$(mktemp -d)
cleanup() { rm -rf "$tmp_dir"; }
trap cleanup EXIT

OUT_DIR="$tmp_dir" "$BASE_DIR/scripts/bootstrap-ca.sh"

if [ -f "$ACTIVE_CA_CERT" ] || [ -f "$ACTIVE_CA_KEY" ]; then
  mv -f "$ACTIVE_CA_CERT" "${ACTIVE_CA_CERT}.bak" 2>/dev/null || true
  mv -f "$ACTIVE_CA_KEY" "${ACTIVE_CA_KEY}.bak" 2>/dev/null || true
fi

mv -f "$tmp_dir/ca.crt" "$ACTIVE_CA_CERT"
mv -f "$tmp_dir/ca.key" "$ACTIVE_CA_KEY"

cat "$OLD_CA_CERT" "$ACTIVE_CA_CERT" > "$CA_BUNDLE_PATH"

echo "Rotation files written:"
echo "  old CA:     $OLD_CA_CERT"
echo "  active CA:  $ACTIVE_CA_CERT"
echo "  bundle:     $CA_BUNDLE_PATH"

if command -v openssl >/dev/null 2>&1; then
  old_fp=$(openssl x509 -in "$OLD_CA_CERT" -noout -fingerprint -sha256 | cut -d= -f2)
  new_fp=$(openssl x509 -in "$ACTIVE_CA_CERT" -noout -fingerprint -sha256 | cut -d= -f2)
  echo "  old fp:     $old_fp"
  echo "  active fp:  $new_fp"
fi

curl_opts=(--cacert "$OLD_CA_CERT")
if [ -n "$AUTH_TOKEN" ]; then
  curl_opts+=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

if ! curl -sS "${curl_opts[@]}" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"rotate--ca.sh reload"}' \
  "$BASE_URL/api/v1/cert-rotation/reload" >/dev/null; then
  echo "Reload failed. Ensure the control-plane is configured with:" >&2
  echo "  CA_BUNDLE_PATH=$CA_BUNDLE_PATH" >&2
  echo "  ACTIVE_CA_CERT_PATH=$ACTIVE_CA_CERT" >&2
  echo "  ACTIVE_CA_KEY_PATH=$ACTIVE_CA_KEY" >&2
  exit 1
fi

echo "Reload requested: $BASE_URL/api/v1/cert-rotation/reload"
