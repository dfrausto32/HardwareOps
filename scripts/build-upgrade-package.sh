#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION=${VERSION:-$(date +%Y%m%d%H%M%S)}
DIST_DIR=${DIST_DIR:-$BASE_DIR/dist/upgrades/$VERSION}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-https://hardwareops.internal}
MAINTENANCE_TOKEN=${MAINTENANCE_TOKEN:-change-me}

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Install Docker or run on a machine with Docker." >&2
  exit 1
fi

host_arch=$(uname -m)
case "$host_arch" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "Unsupported host arch: $host_arch" >&2
    exit 1
    ;;
esac

mkdir -p "$DIST_DIR/images" "$DIST_DIR/scripts"

cp_tag="hardwareops-control-plane:${VERSION}-${arch}"
gw_tag="hardwareops-gateway:${VERSION}-${arch}"

docker build -t "$cp_tag" -f "$BASE_DIR/control-plane/Dockerfile" "$BASE_DIR"
docker build -t "$gw_tag" -f "$BASE_DIR/deploy/compose/nginx/Dockerfile" \
  --build-arg VITE_API_BASE_URL="$PUBLIC_BASE_URL" \
  --build-arg VITE_SIMULATE_PROD=1 \
  --build-arg VITE_MAINTENANCE_TOKEN="$MAINTENANCE_TOKEN" \
  "$BASE_DIR"

docker save -o "$DIST_DIR/images/control-plane.tar" "$cp_tag"
docker save -o "$DIST_DIR/images/gateway.tar" "$gw_tag"

cp -a "$BASE_DIR/scripts/apply-upgrade.sh" "$DIST_DIR/scripts/"
cp -a "$BASE_DIR/deploy/compose/.env.onprem.example" "$DIST_DIR/.env.onprem.example"

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
      TRUST_PROXY: "1"
      CLIENT_CERT_HEADER: X-Client-Cert
      CORS_ALLOWED_ORIGINS: \${CORS_ALLOWED_ORIGINS:-${PUBLIC_BASE_URL}}
      LOG_DIR: /var/lib/hardwareops/logs
      DISABLE_HTTP2: "1"
      MAINTENANCE_MODE: \${MAINTENANCE_MODE:-1}
      MAINTENANCE_MESSAGE: \${MAINTENANCE_MESSAGE:-Maintenance mode enabled}
      MAINTENANCE_TOKEN: \${MAINTENANCE_TOKEN:-change-me}
      UPGRADE_APPLY_CMD: \${UPGRADE_APPLY_CMD:-/app/scripts/apply-upgrade.sh}
      UPGRADE_WORK_DIR: \${UPGRADE_WORK_DIR:-/stack}
      UPGRADE_LOG_DIR: \${UPGRADE_LOG_DIR:-/var/lib/hardwareops/logs}
      UPGRADE_UPDATES_DIR: \${UPGRADE_UPDATES_DIR:-/stack/updates}
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
README

out="$DIST_DIR/hardwareops-upgrade-${VERSION}-linux-${arch}.tar.gz"
tar -C "$(dirname "$DIST_DIR")" -czf "$out" "$(basename "$DIST_DIR")"
echo "Upgrade package written to $out"
