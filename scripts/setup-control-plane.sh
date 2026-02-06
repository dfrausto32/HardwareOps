#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT_DIR=${OUT_DIR:-/opt/hardwareops/certs}
DOMAIN=${DOMAIN:-hardwareops.internal}
ENV_OUT=${ENV_OUT:-$BASE_DIR/control-plane.env}
ENABLE_TLS=${ENABLE_TLS:-0}
FORCE=${FORCE:-0}
FIX_PERMS=${FIX_PERMS:-1}
CHOWN_UID=${CHOWN_UID:-65532}
CHOWN_GID=${CHOWN_GID:-65532}

if [ ! -f "$BASE_DIR/deploy/control-plane.env.example" ] && [ ! -f "$BASE_DIR/control-plane.env.example" ]; then
  echo "control-plane.env.example not found. Run from repo root or installer bundle." >&2
  exit 1
fi

ENV_SRC="$BASE_DIR/deploy/control-plane.env.example"
if [ -f "$BASE_DIR/control-plane.env.example" ]; then
  ENV_SRC="$BASE_DIR/control-plane.env.example"
fi

if [ -f "$ENV_OUT" ] && [ "$FORCE" != "1" ]; then
  echo "Env file already exists at $ENV_OUT. Set FORCE=1 to overwrite." >&2
  echo "Continuing to update certs and permissions only." >&2
else
  cp "$ENV_SRC" "$ENV_OUT"
fi

if [ ! -x "$BASE_DIR/scripts/bootstrap-ca.sh" ] || [ ! -x "$BASE_DIR/scripts/issue-server-cert.sh" ]; then
  echo "Missing cert scripts in $BASE_DIR/scripts. Ensure bootstrap-ca.sh and issue-server-cert.sh are present." >&2
  exit 1
fi

CA_KEY="$OUT_DIR/ca.key"
CA_CERT="$OUT_DIR/ca.crt"
SERVER_KEY="$OUT_DIR/server.key"
SERVER_CERT="$OUT_DIR/server.crt"

if [ ! -f "$CA_KEY" ] || [ ! -f "$CA_CERT" ] || [ "$FORCE" = "1" ]; then
  "$BASE_DIR/scripts/bootstrap-ca.sh" OUT_DIR="$OUT_DIR" FORCE="$FORCE"
fi

if [ ! -f "$SERVER_KEY" ] || [ ! -f "$SERVER_CERT" ] || [ "$FORCE" = "1" ]; then
  "$BASE_DIR/scripts/issue-server-cert.sh" OUT_DIR="$OUT_DIR" DOMAIN="$DOMAIN"
fi

set_kv() {
  python3 - "$ENV_OUT" "$1" "$2" <<'PY'
import os, sys, re
path, key, val = sys.argv[1:4]
lines = []
if os.path.exists(path):
    with open(path, "r") as f:
        lines = f.read().splitlines()
new = []
found = False
for line in lines:
    if re.match(rf"^{re.escape(key)}=", line):
        new.append(f"{key}={val}")
        found = True
    else:
        new.append(line)
if not found:
    new.append(f"{key}={val}")
with open(path, "w") as f:
    f.write("\n".join(new) + "\n")
PY
}

set_kv "CA_CERT_PATH" "$OUT_DIR/ca.crt"
set_kv "CA_KEY_PATH" "$OUT_DIR/ca.key"

if [ "$ENABLE_TLS" = "1" ]; then
  set_kv "TLS_CERT_PATH" "$OUT_DIR/server.crt"
  set_kv "TLS_KEY_PATH" "$OUT_DIR/server.key"
  set_kv "TLS_CLIENT_CA_PATH" "$OUT_DIR/ca.crt"
fi

if [ "$FIX_PERMS" = "1" ]; then
  if [ -f "$CA_KEY" ]; then
    chmod 0640 "$CA_KEY"
  fi
  if [ -f "$CA_CERT" ]; then
    chmod 0644 "$CA_CERT"
  fi
  if [ -f "$SERVER_KEY" ]; then
    chmod 0640 "$SERVER_KEY"
  fi
  if [ -f "$SERVER_CERT" ]; then
    chmod 0644 "$SERVER_CERT"
  fi
  chown "$CHOWN_UID:$CHOWN_GID" "$OUT_DIR"/*.crt "$OUT_DIR"/*.key 2>/dev/null || true
fi

echo "Certs available in: $OUT_DIR"
echo "Env written to: $ENV_OUT"
if [ "$ENABLE_TLS" = "1" ]; then
  echo "TLS enabled in env file."
else
  echo "TLS not enabled in env file (ENABLE_TLS=0)."
fi
