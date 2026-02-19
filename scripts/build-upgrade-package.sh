#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION=${VERSION:-$(date +%Y%m%d%H%M%S)}
host_arch=$(uname -m)
case "$host_arch" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "Unsupported host arch: $host_arch" >&2
    exit 1
    ;;
esac
DIST_NAME=${DIST_NAME:-hardwareops-upgrade-${VERSION}-linux-${arch}}
DIST_DIR=${DIST_DIR:-$BASE_DIR/dist/upgrades/$DIST_NAME}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-https://hardwareops.internal}
MAINTENANCE_TOKEN=${MAINTENANCE_TOKEN:-change-me}
ENV_FILE=${ENV_FILE:-}
LICENSE_EMBED_PUBKEY_PATH=${LICENSE_EMBED_PUBKEY_PATH:-}
LICENSE_EMBED_PUBKEY_B64=${LICENSE_EMBED_PUBKEY_B64:-}

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Install Docker or run on a machine with Docker." >&2
  exit 1
fi

mkdir -p "$DIST_DIR/images" "$DIST_DIR/scripts"

cp_tag="hardwareops-control-plane:${VERSION}-${arch}"
gw_tag="hardwareops-gateway:${VERSION}-${arch}"

if [ -z "$LICENSE_EMBED_PUBKEY_B64" ] && [ -n "$LICENSE_EMBED_PUBKEY_PATH" ]; then
  if [ ! -f "$LICENSE_EMBED_PUBKEY_PATH" ]; then
    echo "LICENSE_EMBED_PUBKEY_PATH not found: $LICENSE_EMBED_PUBKEY_PATH" >&2
    exit 1
  fi
  LICENSE_EMBED_PUBKEY_B64=$(openssl pkey -pubin -in "$LICENSE_EMBED_PUBKEY_PATH" -pubout -outform DER | tail -c 32 | base64 -w 0)
fi
build_args=()
if [ -n "$LICENSE_EMBED_PUBKEY_B64" ]; then
  build_args+=(--build-arg "LICENSE_EMBED_PUBKEY_B64=$LICENSE_EMBED_PUBKEY_B64")
fi

docker build -t "$cp_tag" -f "$BASE_DIR/control-plane/Dockerfile" "${build_args[@]}" "$BASE_DIR"
docker build -t "$gw_tag" -f "$BASE_DIR/deploy/compose/nginx/Dockerfile" \
  --build-arg VITE_API_BASE_URL="$PUBLIC_BASE_URL" \
  --build-arg VITE_SIMULATE_PROD=1 \
  --build-arg VITE_MAINTENANCE_TOKEN="$MAINTENANCE_TOKEN" \
  "$BASE_DIR"

docker save -o "$DIST_DIR/images/control-plane.tar" "$cp_tag"
docker save -o "$DIST_DIR/images/gateway.tar" "$gw_tag"

cp -a "$BASE_DIR/scripts/apply-upgrade.sh" "$DIST_DIR/scripts/"
cp -a "$BASE_DIR/deploy/compose/.env.onprem.example" "$DIST_DIR/.env.onprem.example"
if [ -n "$ENV_FILE" ] && [ -f "$ENV_FILE" ]; then
  cp -a "$ENV_FILE" "$DIST_DIR/.env.onprem"
fi

cat > "$DIST_DIR/docker-compose.onprem.bundle.yml" <<EOF
version: "3.9"

services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: \${POSTGRES_USER:-hardwareops}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD:-hardwareops}
      POSTGRES_DB: \${POSTGRES_DB:-hardwareops}
    volumes:
      - pgdata:/var/lib/postgresql/data
    restart: unless-stopped

  minio:
    image: minio/minio:RELEASE.2024-12-18T13-15-44Z
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: \${MINIO_ROOT_USER:-minio}
      MINIO_ROOT_PASSWORD: \${MINIO_ROOT_PASSWORD:-minio123}
    volumes:
      - miniodata:/data
    restart: unless-stopped

  control-plane:
    image: ${cp_tag}
    environment:
      DATABASE_URL: \${DATABASE_URL:-postgres://hardwareops:hardwareops@postgres:5432/hardwareops?sslmode=disable}
      AUTO_MIGRATE: "1"
      MIGRATIONS_DIR: /app/migrations
      S3_ENDPOINT: \${S3_ENDPOINT:-minio:9000}
      S3_BUCKET: \${S3_BUCKET:-artifacts}
      S3_ACCESS_KEY: \${MINIO_ROOT_USER:-minio}
      S3_SECRET_KEY: \${MINIO_ROOT_PASSWORD:-minio123}
      S3_USE_SSL: "0"
      S3_REGION: \${S3_REGION:-us-east-1}
      CA_CERT_PATH: /certs/ca.crt
      CA_KEY_PATH: /certs/ca.key
      CA_BUNDLE_PATH: \${CA_BUNDLE_PATH:-/certs/ca-bundle.crt}
      ACTIVE_CA_CERT_PATH: \${ACTIVE_CA_CERT_PATH:-/certs/ca-active.crt}
      ACTIVE_CA_KEY_PATH: \${ACTIVE_CA_KEY_PATH:-/certs/ca-active.key}
      CERT_ROTATION_GRACE_PERIOD: \${CERT_ROTATION_GRACE_PERIOD:-168h}
      TRUST_PROXY: \${TRUST_PROXY:-1}
      TRUST_PROXY_CIDRS: \${TRUST_PROXY_CIDRS:-127.0.0.1/32,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,100.64.0.0/10,fc00::/7,fe80::/10}
      CLIENT_CERT_HEADER: X-Client-Cert
      CORS_ALLOWED_ORIGINS: \${CORS_ALLOWED_ORIGINS:-${PUBLIC_BASE_URL}}
      METRICS_ENABLED: \${METRICS_ENABLED:-1}
      METRICS_PATH: \${METRICS_PATH:-/metrics}
      METRICS_REFRESH_INTERVAL: \${METRICS_REFRESH_INTERVAL:-30s}
      AUDIT_RETENTION_DAYS: \${AUDIT_RETENTION_DAYS:-90}
      AUDIT_RETENTION_CLEANUP_INTERVAL: \${AUDIT_RETENTION_CLEANUP_INTERVAL:-1h}
      EVENT_RETENTION_DAYS: \${EVENT_RETENTION_DAYS:-30}
      EVENT_RETENTION_CLEANUP_INTERVAL: \${EVENT_RETENTION_CLEANUP_INTERVAL:-1h}
      ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID: \${ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID:-}
      ARTIFACT_PULL_CREDENTIALS_AWS_REGION: \${ARTIFACT_PULL_CREDENTIALS_AWS_REGION:-}
      AUTH_MODE: \${AUTH_MODE:-local}
      AUTH_JWT_SECRET: \${AUTH_JWT_SECRET:-change-me}
      AUTH_TOKEN_TTL: \${AUTH_TOKEN_TTL:-12h}
      AUTH_ISSUER: \${AUTH_ISSUER:-hardwareops}
      AUTH_BOOTSTRAP_EMAIL: \${AUTH_BOOTSTRAP_EMAIL:-admin@example.com}
      AUTH_BOOTSTRAP_PASSWORD: \${AUTH_BOOTSTRAP_PASSWORD:-change-me}
      AUTH_OIDC_ISSUER: \${AUTH_OIDC_ISSUER:-}
      AUTH_OIDC_CLIENT_ID: \${AUTH_OIDC_CLIENT_ID:-}
      BOOTSTRAP_TOKEN: \${BOOTSTRAP_TOKEN:-change-me-bootstrap}
      LICENSE_ENFORCE: \${LICENSE_ENFORCE:-0}
      LICENSE_PATH: \${LICENSE_PATH:-}
      LICENSE_PUBLIC_KEY: \${LICENSE_PUBLIC_KEY:-}
      LICENSE_PUBLIC_KEY_PATH: \${LICENSE_PUBLIC_KEY_PATH:-}
      LICENSE_KEY_MODE: \${LICENSE_KEY_MODE:-env}
      LICENSE_CACHE_TTL: \${LICENSE_CACHE_TTL:-30s}
      HARDENED_PROFILE: \${HARDENED_PROFILE:-0}
      DEVICE_IDENTITY_MODE: \${DEVICE_IDENTITY_MODE:-audit}
      DEVICE_IDENTITY_REQUIRE_ON_ENROLL: \${DEVICE_IDENTITY_REQUIRE_ON_ENROLL:-0}
      DEVICE_IDENTITY_REQUIRE_ON_CHECKIN: \${DEVICE_IDENTITY_REQUIRE_ON_CHECKIN:-0}
      LOG_DIR: /var/lib/hardwareops/logs
      DISABLE_HTTP2: "1"
      MAINTENANCE_MODE: \${MAINTENANCE_MODE:-0}
      MAINTENANCE_MESSAGE: \${MAINTENANCE_MESSAGE:-}
      MAINTENANCE_TOKEN: \${MAINTENANCE_TOKEN:-change-me}
      UPGRADE_APPLY_CMD: \${UPGRADE_APPLY_CMD:-/app/scripts/apply-upgrade.sh}
      UPGRADE_WORK_DIR: \${UPGRADE_WORK_DIR:-/stack}
      UPGRADE_LOG_DIR: \${UPGRADE_LOG_DIR:-/var/lib/hardwareops/logs}
      UPGRADE_UPDATES_DIR: \${UPGRADE_UPDATES_DIR:-/stack/updates}
      UPGRADE_RUNNER_MODE: \${UPGRADE_RUNNER_MODE:-docker}
      UPGRADE_RUNNER_IMAGE: \${UPGRADE_RUNNER_IMAGE:-${cp_tag}}
      BACKUP_CMD: \${BACKUP_CMD:-/app/scripts/backup-stack.sh}
      RESTORE_CMD: \${RESTORE_CMD:-/app/scripts/restore-stack.sh}
      BACKUP_DIR: \${BACKUP_DIR:-/stack/backups}
      BACKUP_WORK_DIR: \${BACKUP_WORK_DIR:-/stack}
      BACKUP_LOG_DIR: \${BACKUP_LOG_DIR:-/var/lib/hardwareops/logs}
      BACKUP_RUNNER_MODE: \${BACKUP_RUNNER_MODE:-docker}
      BACKUP_RUNNER_IMAGE: \${BACKUP_RUNNER_IMAGE:-}
      BACKUP_POSTGRES_CONTAINER: \${BACKUP_POSTGRES_CONTAINER:-hardwareops-postgres-1}
      BACKUP_MINIO_CONTAINER: \${BACKUP_MINIO_CONTAINER:-hardwareops-minio-1}
      BACKUP_POSTGRES_USER: \${BACKUP_POSTGRES_USER:-hardwareops}
      BACKUP_POSTGRES_DB: \${BACKUP_POSTGRES_DB:-hardwareops}
      STACK_DIR: \${STACK_DIR:-/stack}
    volumes:
      - \${CERTS_DIR:-/opt/hardwareops/certs}:/certs:ro
      - controlplane-logs:/var/lib/hardwareops/logs
      - /var/run/docker.sock:/var/run/docker.sock
      - \${STACK_DIR:-.}:/stack
    depends_on:
      - postgres
      - minio
    restart: unless-stopped

  gateway:
    image: ${gw_tag}
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - \${CERTS_DIR:-/opt/hardwareops/certs}:/certs:ro
    depends_on:
      - control-plane
    restart: unless-stopped

volumes:
  pgdata:
  miniodata:
  controlplane-logs:
EOF

cat > "$DIST_DIR/README.txt" <<'README'
HardwareOps Upgrade Package

Contents:
- images/control-plane.tar
- images/gateway.tar
- docker-compose.onprem.bundle.yml
- .env.onprem.example
- scripts/apply-upgrade.sh

Usage:
1) Copy this folder to the stack host (same box as the running stack).
2) Ensure /opt/hardwareops/certs exists on the host (from initial install).
3) Run:
   STACK_DIR=$PWD \
   ENV_FILE=.env.onprem.example \
   ./scripts/apply-upgrade.sh

The apply script loads images, updates compose, runs health checks,
and disables maintenance mode if MAINTENANCE_TOKEN is set.
If you pass ENV_FILE when building the package, .env.onprem is bundled.
README

out="$BASE_DIR/dist/hardwareops-upgrade-${VERSION}-linux-${arch}.tar.gz"
tar -C "$(dirname "$DIST_DIR")" -czf "$out" "$(basename "$DIST_DIR")"
echo "Upgrade package written to $out"
