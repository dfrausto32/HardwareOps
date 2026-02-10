#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if [ -n "${CONTROL_PLANE_ENV_FILE:-}" ] && [ -f "$CONTROL_PLANE_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  source "$CONTROL_PLANE_ENV_FILE"
  set +a
fi

if [ -z "${ENABLE_TLS+x}" ]; then
  if [ "${USE_PROXY:-0}" = "1" ]; then
    ENABLE_TLS=0
  else
    ENABLE_TLS=1
  fi
fi
FORCE_DEV_CERTS=${FORCE_DEV_CERTS:-0}
SKIP_DEV_CERTS=${SKIP_DEV_CERTS:-0}
CERTS_DIR=${CERTS_DIR:-}
SKIP_CERTS_COPY=${SKIP_CERTS_COPY:-0}

# Resolve CA paths (prefer env overrides, but normalize to absolute)
# If certs exist in the current working directory, prefer them when no env overrides are set.
if [ -z "${CA_CERT_PATH+x}" ] && [ -z "${CA_KEY_PATH+x}" ]; then
  if [ -f "$PWD/dev-ca.crt" ] && [ -f "$PWD/dev-ca.key" ]; then
    CERTS_DIR=${CERTS_DIR:-$PWD}
    SKIP_CERTS_COPY=1
  fi
fi
if [ -n "$CERTS_DIR" ]; then
  CA_CERT_INPUT=${CA_CERT_PATH:-$CERTS_DIR/dev-ca.crt}
  CA_KEY_INPUT=${CA_KEY_PATH:-$CERTS_DIR/dev-ca.key}
  TLS_CERT_INPUT=${TLS_CERT_PATH:-$CERTS_DIR/dev-server.crt}
  TLS_KEY_INPUT=${TLS_KEY_PATH:-$CERTS_DIR/dev-server.key}
else
  CA_CERT_INPUT=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
  CA_KEY_INPUT=${CA_KEY_PATH:-$BASE_DIR/dev-ca.key}
  TLS_CERT_INPUT=${TLS_CERT_PATH:-$BASE_DIR/dev-server.crt}
  TLS_KEY_INPUT=${TLS_KEY_PATH:-$BASE_DIR/dev-server.key}
fi
CA_CERT_REPO="$BASE_DIR/dev-ca.crt"
CA_KEY_REPO="$BASE_DIR/dev-ca.key"
TLS_CERT_REPO="$BASE_DIR/dev-server.crt"
TLS_KEY_REPO="$BASE_DIR/dev-server.key"

if command -v realpath >/dev/null 2>&1; then
  CA_CERT=$(realpath -m "$CA_CERT_INPUT")
  CA_KEY=$(realpath -m "$CA_KEY_INPUT")
  TLS_CERT=$(realpath -m "$TLS_CERT_INPUT")
  TLS_KEY=$(realpath -m "$TLS_KEY_INPUT")
else
  CA_CERT="$CA_CERT_INPUT"
  CA_KEY="$CA_KEY_INPUT"
  TLS_CERT="$TLS_CERT_INPUT"
  TLS_KEY="$TLS_KEY_INPUT"
fi

CSR_DIR=${CSR_DIR:-/tmp/hardwareops}
mkdir -p "$CSR_DIR"

SKIP_CERTS_GEN=0
if [ "$SKIP_DEV_CERTS" = "1" ]; then
  SKIP_CERTS_GEN=1
  FORCE_DEV_CERTS=0
fi

# Force regeneration of dev certs when requested
if [ "$FORCE_DEV_CERTS" = "1" ]; then
  rm -f "$CA_CERT" "$CA_KEY" "$TLS_CERT" "$TLS_KEY"
  rm -f "$CSR_DIR/server.csr" "$CSR_DIR/server-ext.cnf"
  rm -f "${CA_CERT%.crt}.srl" "$CA_CERT.srl"
fi

# If repo-root dev CA already exists, prefer it and skip regeneration/copy.
if [ "$FORCE_DEV_CERTS" != "1" ]; then
  if [ -f "$CA_CERT_REPO" ] && [ -f "$CA_KEY_REPO" ]; then
    CA_CERT="$CA_CERT_REPO"
    CA_KEY="$CA_KEY_REPO"
  fi
  if ([ ! -f "$CA_CERT" ] || [ ! -f "$CA_KEY" ]) && [ -f "$CA_CERT_REPO" ] && [ -f "$CA_KEY_REPO" ]; then
    CA_CERT="$CA_CERT_REPO"
    CA_KEY="$CA_KEY_REPO"
  fi
  if ([ ! -f "$TLS_CERT" ] || [ ! -f "$TLS_KEY" ]) && [ -f "$TLS_CERT_REPO" ] && [ -f "$TLS_KEY_REPO" ]; then
    TLS_CERT="$TLS_CERT_REPO"
    TLS_KEY="$TLS_KEY_REPO"
  fi
fi

# Create dev CA only when missing (or forced). Do not reissue unless forced.
if [ "$SKIP_CERTS_GEN" = "0" ] && { [ "$FORCE_DEV_CERTS" = "1" ] || { [ ! -f "$CA_CERT" ] && [ ! -f "$CA_KEY" ]; }; }; then
  mkdir -p "$(dirname "$CA_CERT")" "$(dirname "$CA_KEY")"
  tmp_cfg=$(mktemp)
  cat > "$tmp_cfg" <<EOF
[req]
distinguished_name = dn
x509_extensions = v3_ca
prompt = no

[dn]
CN = HardwareOps Dev CA

[v3_ca]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true
keyUsage = critical, keyCertSign, cRLSign
EOF
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$CA_KEY" -out "$CA_CERT" \
    -days 365 -config "$tmp_cfg" -extensions v3_ca
  rm -f "$tmp_cfg"
else
  if [ ! -f "$CA_CERT" ] || [ ! -f "$CA_KEY" ]; then
    echo "Dev CA incomplete. Provide CA_CERT_PATH/CA_KEY_PATH or run with FORCE_DEV_CERTS=1." >&2
    exit 1
  fi
fi

# Ensure dev CA is available in repo root for easy reuse
if [ "$SKIP_CERTS_COPY" != "1" ] && [ "$CA_CERT" != "$CA_CERT_REPO" ]; then
  cp -f "$CA_CERT" "$CA_CERT_REPO"
  cp -f "$CA_KEY" "$CA_KEY_REPO"
  CA_CERT="$CA_CERT_REPO"
  CA_KEY="$CA_KEY_REPO"
fi

# Create a device CSR + key (for enroll testing)
if [ ! -f "$CSR_DIR/device.csr" ] || [ ! -f "$CSR_DIR/device.key" ]; then
  openssl req -newkey rsa:2048 -nodes \
    -keyout "$CSR_DIR/device.key" -out "$CSR_DIR/device.csr" \
    -subj "/CN=hardwareops-device"
fi

if [ "$ENABLE_TLS" = "1" ]; then
  if [ "$SKIP_CERTS_GEN" = "0" ] && { [ "$FORCE_DEV_CERTS" = "1" ] || { [ ! -f "$TLS_CERT" ] && [ ! -f "$TLS_KEY" ]; }; }; then
    mkdir -p "$(dirname "$TLS_CERT")" "$(dirname "$TLS_KEY")"
    openssl req -new -newkey rsa:2048 -nodes \
      -keyout "$TLS_KEY" -out "$CSR_DIR/server.csr" \
      -subj "/CN=localhost"
    EXT_FILE="$CSR_DIR/server-ext.cnf"
    cat > "$EXT_FILE" <<EOF
[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = host.docker.internal
IP.1 = 127.0.0.1
EOF
    openssl x509 -req -in "$CSR_DIR/server.csr" \
      -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
      -out "$TLS_CERT" -days 365 -extfile "$EXT_FILE" -extensions v3_req
  else
    if [ ! -f "$TLS_CERT" ] || [ ! -f "$TLS_KEY" ]; then
      echo "TLS cert/key incomplete. Provide TLS_CERT_PATH/TLS_KEY_PATH or run with FORCE_DEV_CERTS=1." >&2
      exit 1
    fi
  fi
  # Ensure dev server cert is available in repo root for easy reuse
  if [ "$SKIP_CERTS_COPY" != "1" ] && [ "$TLS_CERT" != "$TLS_CERT_REPO" ]; then
    cp -f "$TLS_CERT" "$TLS_CERT_REPO"
    cp -f "$TLS_KEY" "$TLS_KEY_REPO"
    TLS_CERT="$TLS_CERT_REPO"
    TLS_KEY="$TLS_KEY_REPO"
  fi
fi

export DATABASE_URL=${DATABASE_URL:-postgres://hardwareops:hardwareops@localhost:5432/hardwareops?sslmode=disable}
export CA_CERT_PATH="$CA_CERT"
export CA_KEY_PATH="$CA_KEY"
if [ "$ENABLE_TLS" = "1" ]; then
  export TLS_CERT_PATH="$TLS_CERT"
  export TLS_KEY_PATH="$TLS_KEY"
  export TLS_CLIENT_CA_PATH="$CA_CERT"
fi
if [ "${USE_PROXY:-0}" = "1" ]; then
  export TRUST_PROXY=1
fi
export AUTO_MIGRATE=${AUTO_MIGRATE:-1}
export S3_ENDPOINT=${S3_ENDPOINT:-localhost:9000}
export S3_BUCKET=${S3_BUCKET:-artifacts}
export S3_ACCESS_KEY=${S3_ACCESS_KEY:-minio}
export S3_SECRET_KEY=${S3_SECRET_KEY:-minio123}
export S3_USE_SSL=${S3_USE_SSL:-0}
export S3_PRESIGN_TTL=${S3_PRESIGN_TTL:-5m}
export LOG_INGEST_ADDR=${LOG_INGEST_ADDR:-tcp://0.0.0.0:5560}
export LOG_DIR=${LOG_DIR:-$BASE_DIR/logs}
export MIGRATIONS_DIR=${MIGRATIONS_DIR:-$BASE_DIR/control-plane/migrations}
export DISABLE_HTTP2=${DISABLE_HTTP2:-1}
export CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS:-http://localhost:5173,http://127.0.0.1:5173}
export MAINTENANCE_MODE=${MAINTENANCE_MODE:-1}
export MAINTENANCE_MESSAGE=${MAINTENANCE_MESSAGE:-"Maintenance mode enabled"}
export MAINTENANCE_TOKEN=${MAINTENANCE_TOKEN:-dev-token}
if [ -f "$BASE_DIR/scripts/apply-upgrade.sh" ]; then
  export UPGRADE_APPLY_CMD=${UPGRADE_APPLY_CMD:-"$BASE_DIR/scripts/apply-upgrade.sh"}
fi

# Quick connectivity check for Postgres
if ! docker compose -f "$BASE_DIR/deploy/compose/docker-compose.yml" ps >/dev/null 2>&1; then
  echo "Docker Compose not available or not running. Start with: make dev-up" >&2
fi

( cd "$BASE_DIR/control-plane" && go run ./cmd/control-plane )
