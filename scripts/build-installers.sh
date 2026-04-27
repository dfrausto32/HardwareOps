#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
GO_BIN=${GO_BIN:-go}
VERSION=${VERSION:-$(date +%Y%m%d%H%M%S)}
DIST_DIR=${DIST_DIR:-$BASE_DIR/dist/installers/$VERSION}
LICENSE_EMBED_PUBKEY_PATH=${LICENSE_EMBED_PUBKEY_PATH:-}
LICENSE_EMBED_PUBKEY_B64=${LICENSE_EMBED_PUBKEY_B64:-}

AGENT_PLATFORMS_DEFAULT="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"
CONTROL_PLANE_PLATFORMS_DEFAULT="linux/amd64 linux/arm64"
BUILD_STACK=${BUILD_STACK:-1}

AGENT_PLATFORMS=${AGENT_PLATFORMS:-$AGENT_PLATFORMS_DEFAULT}
CONTROL_PLANE_PLATFORMS=${CONTROL_PLANE_PLATFORMS:-$CONTROL_PLANE_PLATFORMS_DEFAULT}

if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  echo "Go toolchain not found: $GO_BIN. Install Go 1.22+ or set GO_BIN=/path/to/go." >&2
  exit 1
fi

mkdir -p "$DIST_DIR"

copy_customer_docs() {
  local stage=$1
  "$BASE_DIR/scripts/assemble-customer-docs.sh" "$stage/customer-docs"
}

copy_common_agent_files() {
  local stage=$1
  mkdir -p "$stage/scripts" "$stage/deploy/systemd"
  cp -a "$BASE_DIR/scripts/agent-enroll.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/install-agent-deps-ubuntu.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/deploy/systemd/agent.env.example" "$stage/"
  cp -a "$BASE_DIR/deploy/systemd/agent.env.example" "$stage/deploy/systemd/"
  cp -a "$BASE_DIR/deploy/systemd/parcel-agent.service" "$stage/deploy/systemd/"
  cp -a "$BASE_DIR/scripts/agent-install.sh" "$stage/scripts/"
}

write_agent_readme() {
  local stage=$1
  local goos=$2
  local goarch=$3
  local filename="$stage/README.txt"

  if [ "$goos" = "linux" ]; then
    cat > "$filename" <<'AGENT_LINUX'
Parcel Agent (linux)

Install (systemd):
  sudo ./scripts/agent-install.sh \
       AGENT_SRC=./parcel-agent \
       CONTROL_PLANE_URL=https://agent.parcel.internal \
       CONTROL_PLANE_CA_CERT_SRC=/opt/parcel/certs/ca.crt \
       AGENT_ENROLL_MODE=approval \
       ENROLLMENT_PROFILE_TOKEN=<bootstrap-token> \
       START_SERVICE=1

For public CA server trust:
  add USE_SYSTEM_CA=1 and omit CONTROL_PLANE_CA_CERT_SRC

Config:
  /etc/parcel/agent/agent.env

Flow:
  1. Create an enrollment profile in the control-plane Security UI or API.
  2. Copy the bootstrap token to the device and copy the control-plane CA to
     /opt/parcel/certs/ca.crt.
  3. Run the install command above.
  4. Approve the pending request.
  5. Watch the agent move from approval bootstrap into normal mTLS check-in.

Legacy direct enrollment:
  sudo CONTROL_PLANE_URL=https://agent.parcel.internal \
       CA_CERT_PATH=/opt/parcel/certs/ca.crt \
       ./scripts/agent-enroll.sh

Docs:
  docs/agent-systemd.md
  docs/installer-flow.md
AGENT_LINUX
  else
    cat > "$filename" <<'AGENT_OTHER'
Parcel Agent

This bundle includes the agent binary and sample config.
Run manually for now:
  CONTROL_PLANE_URL=https://parcel.internal \
  CONTROL_PLANE_CA_CERT_PATH=/path/to/ca.crt \
  ./parcel-agent

macOS/Windows service install is not included in v1.
AGENT_OTHER
  fi

  # add arch info without templating the here-doc
  printf "\nPlatform: %s/%s\n" "$goos" "$goarch" >> "$filename"
}

write_control_plane_readme() {
  local stage=$1
  local goos=$2
  local goarch=$3
  local filename="$stage/README.txt"

  cat > "$filename" <<'CONTROL_PLANE'
Parcel Control-Plane

1) Create env file from example:
   cp control-plane.env.example control-plane.env

2) Run:
   env $(cat control-plane.env | xargs) ./control-plane

Notes:
- If running behind a TLS gateway, do not set TLS_CERT_PATH/TLS_KEY_PATH.
- For TLS directly in the control-plane, set TLS_CERT_PATH/TLS_KEY_PATH.
- Migrations are in ./migrations.
- Customer-facing documentation is included under ./customer-docs.
CONTROL_PLANE

  printf "\nPlatform: %s/%s\n" "$goos" "$goarch" >> "$filename"
}

package_dir() {
  local stage=$1
  local out=$2
  local goos=$3
  local base
  base=$(basename "$stage")

  if [ "$goos" = "windows" ]; then
    python3 - <<'PY' "$stage" "$out"
import os, sys, zipfile
stage, out = sys.argv[1], sys.argv[2]
base = os.path.basename(stage)
with zipfile.ZipFile(out, 'w', compression=zipfile.ZIP_DEFLATED) as zf:
    for root, _, files in os.walk(stage):
        for name in files:
            path = os.path.join(root, name)
            rel = os.path.relpath(path, stage)
            zf.write(path, os.path.join(base, rel))
PY
  else
    tar -C "$DIST_DIR" -czf "$out" "$base"
  fi
}

build_agent() {
  local goos=$1
  local goarch=$2
  local ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  local stage="$DIST_DIR/agent-${VERSION}-${goos}-${goarch}"
  mkdir -p "$stage"

  (cd "$BASE_DIR/agent" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO_BIN" build -o "$stage/parcel-agent$ext" ./cmd/agent)

  if [ ! -f "$stage/parcel-agent$ext" ]; then
    echo "Agent build failed for $goos/$goarch (binary missing)." >&2
    exit 1
  fi

  copy_common_agent_files "$stage"
  write_agent_readme "$stage" "$goos" "$goarch"

  local out
  if [ "$goos" = "windows" ]; then
    out="$DIST_DIR/parcel-agent-${VERSION}-${goos}-${goarch}.zip"
  else
    out="$DIST_DIR/parcel-agent-${VERSION}-${goos}-${goarch}.tar.gz"
  fi
  package_dir "$stage" "$out" "$goos"
  rm -rf "$stage"
  echo "built $out"
}

build_control_plane() {
  local goos=$1
  local goarch=$2
  local ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  local ldflags=""
  if [ -z "$LICENSE_EMBED_PUBKEY_B64" ] && [ -n "$LICENSE_EMBED_PUBKEY_PATH" ]; then
    if [ ! -f "$LICENSE_EMBED_PUBKEY_PATH" ]; then
      echo "LICENSE_EMBED_PUBKEY_PATH not found: $LICENSE_EMBED_PUBKEY_PATH" >&2
      exit 1
    fi
    LICENSE_EMBED_PUBKEY_B64=$(openssl pkey -pubin -in "$LICENSE_EMBED_PUBKEY_PATH" -pubout -outform DER | tail -c 32 | base64 -w 0)
  fi
  if [ -n "$LICENSE_EMBED_PUBKEY_B64" ]; then
    ldflags="-ldflags=-X=github.com/parcel/control-plane/internal/license.EmbeddedPublicKey=$LICENSE_EMBED_PUBKEY_B64"
  fi

  local stage="$DIST_DIR/control-plane-${VERSION}-${goos}-${goarch}"
  mkdir -p "$stage"

  (cd "$BASE_DIR/control-plane" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO_BIN" build $ldflags -o "$stage/control-plane$ext" ./cmd/control-plane)

  if [ ! -f "$stage/control-plane$ext" ]; then
    echo "Control-plane build failed for $goos/$goarch (binary missing)." >&2
    exit 1
  fi

  cp -a "$BASE_DIR/control-plane/migrations" "$stage/"
  cp -a "$BASE_DIR/deploy/control-plane.env.example" "$stage/control-plane.env.example"
  mkdir -p "$stage/scripts"
  cp -a "$BASE_DIR/scripts/bootstrap-ca.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/issue-server-cert.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/setup-control-plane.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/reload-pull-credentials.sh" "$stage/scripts/"
  copy_customer_docs "$stage"
  write_control_plane_readme "$stage" "$goos" "$goarch"

  local out
  if [ "$goos" = "windows" ]; then
    out="$DIST_DIR/control-plane-${VERSION}-${goos}-${goarch}.zip"
  else
    out="$DIST_DIR/control-plane-${VERSION}-${goos}-${goarch}.tar.gz"
  fi
  package_dir "$stage" "$out" "$goos"
  rm -rf "$stage"
  echo "built $out"
}

write_stack_readme() {
  local stage=$1
  local arch=$2
  local filename="$stage/README.txt"

  cat > "$filename" <<'STACK'
Parcel Stack Bundle (Control‑Plane + UI)

1) Install Docker (Ubuntu):
   sudo ./scripts/install-docker-ubuntu.sh

2) Run the stack:
   sudo ./scripts/run-stack.sh

Double‑click installer (Linux desktop):
  desktop/parcel-installer.desktop

Defaults:
- DOMAIN=parcel.internal
- PUBLIC_BASE_URL=https://parcel.internal
- AGENT_BASE_URL=https://agent.parcel.internal
- CERTS_DIR=/opt/parcel/certs
- AUTH_MODE=local
- BOOTSTRAP_TOKEN=change-me-bootstrap

TLS split:
- SERVER_CERT_MODE=device-ca (dev default)
- For production browser trust, use SERVER_CERT_MODE=external and provide
  SERVER_CERT_INPUT + SERVER_KEY_INPUT before running installer.

Upgrade:
  Use scripts/apply-upgrade.sh (see docs/development/upgrade-strategy.md).

Notes:
- This bundle is architecture-specific.
- Images are loaded locally; no build on the target machine.
- Customer-facing documentation is included under ./customer-docs.
STACK

  printf "\nPlatform: linux/%s\n" "$arch" >> "$filename"
}

build_stack_bundle() {
  local arch=$1
  local stage="$DIST_DIR/stack-${VERSION}-linux-${arch}"
  local cp_tag="parcel-control-plane:${VERSION}-${arch}"
  local gw_tag="parcel-gateway:${VERSION}-${arch}"
  local gp_tag="parcel-global-plane:${VERSION}-${arch}"

  if ! command -v docker >/dev/null 2>&1; then
    echo "Docker not found. Install Docker or set BUILD_STACK=0 to skip." >&2
    exit 1
  fi

  mkdir -p "$stage/images" "$stage/scripts"
  mkdir -p "$stage/desktop"

  local build_args=()
  if [ -n "$LICENSE_EMBED_PUBKEY_B64" ]; then
    build_args+=(--build-arg "LICENSE_EMBED_PUBKEY_B64=$LICENSE_EMBED_PUBKEY_B64")
  elif [ -n "$LICENSE_EMBED_PUBKEY_PATH" ]; then
    if [ ! -f "$LICENSE_EMBED_PUBKEY_PATH" ]; then
      echo "LICENSE_EMBED_PUBKEY_PATH not found: $LICENSE_EMBED_PUBKEY_PATH" >&2
      exit 1
    fi
    LICENSE_EMBED_PUBKEY_B64=$(openssl pkey -pubin -in "$LICENSE_EMBED_PUBKEY_PATH" -pubout -outform DER | tail -c 32 | base64 -w 0)
    build_args+=(--build-arg "LICENSE_EMBED_PUBKEY_B64=$LICENSE_EMBED_PUBKEY_B64")
  fi

  docker build -t "$cp_tag" -f "$BASE_DIR/control-plane/Dockerfile" "${build_args[@]}" "$BASE_DIR"
  docker build -t "$gw_tag" -f "$BASE_DIR/deploy/compose/nginx/Dockerfile" \
    --build-arg VITE_API_BASE_URL="https://parcel.internal" \
    --build-arg VITE_SIMULATE_PROD=1 \
    "$BASE_DIR"
  docker build -t "$gp_tag" -f "$BASE_DIR/control-plane/Dockerfile.global-plane" "$BASE_DIR"

  docker save -o "$stage/images/control-plane.tar" "$cp_tag"
  docker save -o "$stage/images/gateway.tar" "$gw_tag"
  docker save -o "$stage/images/global-plane.tar" "$gp_tag"

  cp -a "$BASE_DIR/deploy/compose/.env.onprem.example" "$stage/.env.onprem.example"
  cp -a "$BASE_DIR/deploy/control-plane.env.example" "$stage/control-plane.env.example"
  cp -a "$BASE_DIR/deploy/global-plane.env.example" "$stage/global-plane.env.example"
  cp -a "$BASE_DIR/scripts/setup-control-plane.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/bootstrap-ca.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/issue-server-cert.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/reload-pull-credentials.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/apply-upgrade.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/stack-installer.sh" "$stage/parcel-installer.sh"
  cp -a "$BASE_DIR/deploy/desktop/parcel-installer.desktop" "$stage/desktop/"
  cp -a "$BASE_DIR/scripts/run-stack.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/run-coredns.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/set-dns.sh" "$stage/scripts/"
  cp -a "$BASE_DIR/scripts/install-docker-ubuntu.sh" "$stage/scripts/"
  copy_customer_docs "$stage"

  cat > "$stage/docker-compose.onprem.bundle.yml" <<EOF
version: "3.9"

services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: \${POSTGRES_USER:-parcel}
      POSTGRES_PASSWORD: \${POSTGRES_PASSWORD:-parcel}
      POSTGRES_DB: \${POSTGRES_DB:-parcel}
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
      DATABASE_URL: \${DATABASE_URL:-postgres://parcel:parcel@postgres:5432/parcel?sslmode=disable}
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
      TRUST_PROXY_CIDRS: \${TRUST_PROXY_CIDRS:-127.0.0.1/32,::1/128}
      CLIENT_CERT_HEADER: X-Client-Cert
      CORS_ALLOWED_ORIGINS: \${CORS_ALLOWED_ORIGINS:-https://parcel.internal}
      METRICS_ENABLED: \${METRICS_ENABLED:-1}
      METRICS_PATH: \${METRICS_PATH:-/metrics}
      METRICS_REFRESH_INTERVAL: \${METRICS_REFRESH_INTERVAL:-30s}
      AUDIT_RETENTION_DAYS: \${AUDIT_RETENTION_DAYS:-90}
      AUDIT_RETENTION_CLEANUP_INTERVAL: \${AUDIT_RETENTION_CLEANUP_INTERVAL:-1h}
      EVENT_RETENTION_DAYS: \${EVENT_RETENTION_DAYS:-30}
      EVENT_RETENTION_CLEANUP_INTERVAL: \${EVENT_RETENTION_CLEANUP_INTERVAL:-1h}
      ARTIFACT_PULL_ALLOW_INSECURE_HTTP: \${ARTIFACT_PULL_ALLOW_INSECURE_HTTP:-0}
      ARTIFACT_TRUST_VERIFICATION_MODE: \${ARTIFACT_TRUST_VERIFICATION_MODE:-warn_unsigned}
      ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS: \${ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS:-}
      ARTIFACT_TRUST_ALLOWED_SIGNATURE_TYPES: \${ARTIFACT_TRUST_ALLOWED_SIGNATURE_TYPES:-}
      TRUSTED_SIGNING_KEYS_FILE: \${TRUSTED_SIGNING_KEYS_FILE:-}
      TRUSTED_SIGNING_KEYS_JSON: \${TRUSTED_SIGNING_KEYS_JSON:-}
      TRUSTED_SIGNING_KEYS_AWS_SECRET_ID: \${TRUSTED_SIGNING_KEYS_AWS_SECRET_ID:-}
      TRUSTED_SIGNING_KEYS_AWS_REGION: \${TRUSTED_SIGNING_KEYS_AWS_REGION:-}
      ARTIFACT_SIGNATURE_REQUIRE_DEFAULT: \${ARTIFACT_SIGNATURE_REQUIRE_DEFAULT:-0}
      ARTIFACT_SIGNATURE_ENFORCE_INGEST: \${ARTIFACT_SIGNATURE_ENFORCE_INGEST:-0}
      ARTIFACT_SIGNATURE_KEY_ID: \${ARTIFACT_SIGNATURE_KEY_ID:-}
      ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID: \${ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID:-}
      ARTIFACT_PULL_CREDENTIALS_AWS_REGION: \${ARTIFACT_PULL_CREDENTIALS_AWS_REGION:-}
      ARTIFACT_PULL_CREDENTIALS_VAULT_ADDR: \${ARTIFACT_PULL_CREDENTIALS_VAULT_ADDR:-}
      ARTIFACT_PULL_CREDENTIALS_VAULT_TOKEN: \${ARTIFACT_PULL_CREDENTIALS_VAULT_TOKEN:-}
      ARTIFACT_PULL_CREDENTIALS_VAULT_PATH: \${ARTIFACT_PULL_CREDENTIALS_VAULT_PATH:-}
      ARTIFACT_FULCIO_ROOT_CERT: \${ARTIFACT_FULCIO_ROOT_CERT:-}
      ARTIFACT_REKOR_URL: \${ARTIFACT_REKOR_URL:-}
      ARTIFACT_REQUIRE_REKOR_LOG: \${ARTIFACT_REQUIRE_REKOR_LOG:-0}
      RELEASE_AUTO_UPDATE_INTERVAL: \${RELEASE_AUTO_UPDATE_INTERVAL:-60s}
      AUTH_MODE: \${AUTH_MODE:-local}
      AUTH_JWT_SECRET: \${AUTH_JWT_SECRET:-change-me}
      AUTH_TOKEN_TTL: \${AUTH_TOKEN_TTL:-12h}
      AUTH_LOGIN_RPM: \${AUTH_LOGIN_RPM:-30}
      AUTH_LOGIN_BACKOFF_ENABLED: \${AUTH_LOGIN_BACKOFF_ENABLED:-1}
      AUTH_LOGIN_BACKOFF_THRESHOLD: \${AUTH_LOGIN_BACKOFF_THRESHOLD:-3}
      AUTH_LOGIN_BACKOFF_BASE: \${AUTH_LOGIN_BACKOFF_BASE:-2s}
      AUTH_LOGIN_BACKOFF_MAX: \${AUTH_LOGIN_BACKOFF_MAX:-5m}
      AUTH_LOGIN_BACKOFF_WINDOW: \${AUTH_LOGIN_BACKOFF_WINDOW:-15m}
      AUTH_ISSUER: \${AUTH_ISSUER:-parcel}
      AUTH_BOOTSTRAP_EMAIL: \${AUTH_BOOTSTRAP_EMAIL:-admin@example.com}
      AUTH_BOOTSTRAP_PASSWORD: \${AUTH_BOOTSTRAP_PASSWORD:-change-me}
      AUTH_OIDC_ISSUER: \${AUTH_OIDC_ISSUER:-}
      AUTH_OIDC_CLIENT_ID: \${AUTH_OIDC_CLIENT_ID:-}
      AUTH_LDAP_URL: \${AUTH_LDAP_URL:-}
      AUTH_LDAP_BASE_DN: \${AUTH_LDAP_BASE_DN:-}
      AUTH_LDAP_BIND_DN: \${AUTH_LDAP_BIND_DN:-}
      AUTH_LDAP_BIND_PASSWORD: \${AUTH_LDAP_BIND_PASSWORD:-}
      AUTH_LDAP_USER_FILTER: \${AUTH_LDAP_USER_FILTER:-}
      AUTH_LDAP_MAIL_ATTR: \${AUTH_LDAP_MAIL_ATTR:-}
      AUTH_LDAP_DISPLAY_ATTR: \${AUTH_LDAP_DISPLAY_ATTR:-}
      AUTH_LDAP_GROUP_ATTR: \${AUTH_LDAP_GROUP_ATTR:-}
      AUTH_LDAP_ROLE_MAP: \${AUTH_LDAP_ROLE_MAP:-{}}
      AUTH_LDAP_DEFAULT_ROLE: \${AUTH_LDAP_DEFAULT_ROLE:-viewer}
      CI_WORKLOAD_IDENTITY_PROVIDERS_FILE: \${CI_WORKLOAD_IDENTITY_PROVIDERS_FILE:-}
      CI_WORKLOAD_IDENTITY_PROVIDERS_JSON: \${CI_WORKLOAD_IDENTITY_PROVIDERS_JSON:-}
      CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_SECRET_ID: \${CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_SECRET_ID:-}
      CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_REGION: \${CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_REGION:-}
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
      VULN_ARTIFACT_SCANNER: \${VULN_ARTIFACT_SCANNER:-disabled}
      VULN_ARTIFACT_SCANNER_BIN: \${VULN_ARTIFACT_SCANNER_BIN:-}
      VULN_ARTIFACT_SKIP_TYPES: \${VULN_ARTIFACT_SKIP_TYPES:-}
      VULN_NESSUS_URL: \${VULN_NESSUS_URL:-}
      VULN_NESSUS_ACCESS_KEY: \${VULN_NESSUS_ACCESS_KEY:-}
      VULN_NESSUS_SECRET_KEY: \${VULN_NESSUS_SECRET_KEY:-}
      VULN_NESSUS_SYNC_INTERVAL: \${VULN_NESSUS_SYNC_INTERVAL:-1h}
      VULN_NESSUS_SCAN_IDS: \${VULN_NESSUS_SCAN_IDS:-}
      LOG_DIR: /var/lib/parcel/logs
      DISABLE_HTTP2: "1"
      MAINTENANCE_MODE: \${MAINTENANCE_MODE:-0}
      MAINTENANCE_MESSAGE: \${MAINTENANCE_MESSAGE:-}
      MAINTENANCE_TOKEN: \${MAINTENANCE_TOKEN:-change-me}
      UPGRADE_APPLY_CMD: \${UPGRADE_APPLY_CMD:-/app/scripts/apply-upgrade.sh}
      UPGRADE_WORK_DIR: \${UPGRADE_WORK_DIR:-/stack}
      UPGRADE_LOG_DIR: \${UPGRADE_LOG_DIR:-/var/lib/parcel/logs}
      UPGRADE_UPDATES_DIR: \${UPGRADE_UPDATES_DIR:-/stack/updates}
      UPGRADE_RUNNER_MODE: \${UPGRADE_RUNNER_MODE:-remote}
      UPGRADE_RUNNER_URL: \${UPGRADE_RUNNER_URL:-http://maintenance-runner:8090}
      UPGRADE_RUNNER_TOKEN: \${UPGRADE_RUNNER_TOKEN:-change-me-maintenance-runner}
      UPGRADE_RUNNER_IMAGE: \${UPGRADE_RUNNER_IMAGE:-${cp_tag}}
      BACKUP_CMD: \${BACKUP_CMD:-/app/scripts/backup-stack.sh}
      RESTORE_CMD: \${RESTORE_CMD:-/app/scripts/restore-stack.sh}
      BACKUP_DIR: \${BACKUP_DIR:-/stack/backups}
      BACKUP_WORK_DIR: \${BACKUP_WORK_DIR:-/stack}
      BACKUP_LOG_DIR: \${BACKUP_LOG_DIR:-/var/lib/parcel/logs}
      BACKUP_RUNNER_MODE: \${BACKUP_RUNNER_MODE:-remote}
      BACKUP_RUNNER_URL: \${BACKUP_RUNNER_URL:-http://maintenance-runner:8090}
      BACKUP_RUNNER_TOKEN: \${BACKUP_RUNNER_TOKEN:-change-me-maintenance-runner}
      BACKUP_RUNNER_IMAGE: \${BACKUP_RUNNER_IMAGE:-}
      BACKUP_POSTGRES_CONTAINER: \${BACKUP_POSTGRES_CONTAINER:-parcel-postgres-1}
      BACKUP_MINIO_CONTAINER: \${BACKUP_MINIO_CONTAINER:-parcel-minio-1}
      BACKUP_POSTGRES_USER: \${BACKUP_POSTGRES_USER:-parcel}
      BACKUP_POSTGRES_DB: \${BACKUP_POSTGRES_DB:-parcel}
      STACK_DIR: \${STACK_DIR:-/stack}
    volumes:
      - \${CERTS_DIR:-/opt/parcel/certs}:/certs:ro
      - controlplane-logs:/var/lib/parcel/logs
      - \${STACK_DIR:-.}:/stack
    depends_on:
      - postgres
      - minio
      - maintenance-runner
    restart: unless-stopped

  maintenance-runner:
    image: ${cp_tag}
    entrypoint: ["/app/maintenance-runner"]
    environment:
      CA_CERT_PATH: /certs/ca.crt
      MAINTENANCE_TOKEN: \${MAINTENANCE_TOKEN:-change-me}
      UPGRADE_APPLY_CMD: \${UPGRADE_APPLY_CMD:-/app/scripts/apply-upgrade.sh}
      UPGRADE_WORK_DIR: \${UPGRADE_WORK_DIR:-/stack}
      UPGRADE_LOG_DIR: \${UPGRADE_LOG_DIR:-/var/lib/parcel/logs}
      UPGRADE_UPDATES_DIR: \${UPGRADE_UPDATES_DIR:-/stack/updates}
      UPGRADE_RUNNER_MODE: docker
      UPGRADE_RUNNER_IMAGE: \${UPGRADE_RUNNER_IMAGE:-${cp_tag}}
      UPGRADE_RUNNER_TOKEN: \${UPGRADE_RUNNER_TOKEN:-change-me-maintenance-runner}
      BACKUP_CMD: \${BACKUP_CMD:-/app/scripts/backup-stack.sh}
      RESTORE_CMD: \${RESTORE_CMD:-/app/scripts/restore-stack.sh}
      BACKUP_DIR: \${BACKUP_DIR:-/stack/backups}
      BACKUP_WORK_DIR: \${BACKUP_WORK_DIR:-/stack}
      BACKUP_LOG_DIR: \${BACKUP_LOG_DIR:-/var/lib/parcel/logs}
      BACKUP_RUNNER_MODE: docker
      BACKUP_RUNNER_IMAGE: \${BACKUP_RUNNER_IMAGE:-}
      BACKUP_RUNNER_TOKEN: \${BACKUP_RUNNER_TOKEN:-change-me-maintenance-runner}
      BACKUP_POSTGRES_CONTAINER: \${BACKUP_POSTGRES_CONTAINER:-parcel-postgres-1}
      BACKUP_MINIO_CONTAINER: \${BACKUP_MINIO_CONTAINER:-parcel-minio-1}
      BACKUP_POSTGRES_USER: \${BACKUP_POSTGRES_USER:-parcel}
      BACKUP_POSTGRES_DB: \${BACKUP_POSTGRES_DB:-parcel}
      MAINTENANCE_RUNNER_ADDR: \${MAINTENANCE_RUNNER_ADDR:-:8090}
      STACK_DIR: \${STACK_DIR:-/stack}
    volumes:
      - \${CERTS_DIR:-/opt/parcel/certs}:/certs:ro
      - controlplane-logs:/var/lib/parcel/logs
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
      - \${CERTS_DIR:-/opt/parcel/certs}:/certs:ro
    depends_on:
      - control-plane
    restart: unless-stopped

  # ── Global plane (optional — uncomment if this node runs the global plane) ──────
  # global-plane:
  #   image: ${gp_tag}
  #   env_file: global-plane.env
  #   environment:
  #     GLOBAL_DATABASE_URL: \${GLOBAL_DATABASE_URL:-postgres://parcel:parcel@postgres:5432/parcel_global?sslmode=disable}
  #     GLOBAL_MIGRATIONS_DIR: /app/migrations/global
  #     GLOBAL_HTTP_ADDR: :8090
  #     GLOBAL_MINIO_ENDPOINT: \${GLOBAL_MINIO_ENDPOINT:-minio:9000}
  #     GLOBAL_MINIO_ACCESS_KEY: \${MINIO_ROOT_USER:-minio}
  #     GLOBAL_MINIO_SECRET_KEY: \${MINIO_ROOT_PASSWORD:-minio123}
  #     GLOBAL_MINIO_BUCKET: \${GLOBAL_MINIO_BUCKET:-global-artifacts}
  #     GLOBAL_MINIO_USE_TLS: "0"
  #     GLOBAL_PUBLIC_BASE_URL: \${GLOBAL_PUBLIC_BASE_URL:-https://global-plane.example.com}
  #     AUTH_JWT_SECRET: \${AUTH_JWT_SECRET:-change-me}
  #     GLOBAL_TOKEN_ENCRYPTION_KEY: \${GLOBAL_TOKEN_ENCRYPTION_KEY:-}
  #     CORS_ALLOWED_ORIGINS: \${CORS_ALLOWED_ORIGINS:-https://parcel.internal}
  #   ports:
  #     - "8090:8090"
  #   depends_on:
  #     - postgres
  #     - minio
  #   restart: unless-stopped

volumes:
  pgdata:
  miniodata:
  controlplane-logs:
EOF

  write_stack_readme "$stage" "$arch"

  local out="$DIST_DIR/parcel-stack-${VERSION}-linux-${arch}.tar.gz"
  package_dir "$stage" "$out" "linux"
  rm -rf "$stage"
  echo "built $out"
}

build_global_plane() {
  local goos=$1
  local goarch=$2

  local stage="$DIST_DIR/global-plane-${VERSION}-${goos}-${goarch}"
  mkdir -p "$stage"

  (cd "$BASE_DIR/control-plane" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO_BIN" build -o "$stage/global-plane" ./cmd/global-plane)

  if [ ! -f "$stage/global-plane" ]; then
    echo "Global-plane build failed for $goos/$goarch (binary missing)." >&2
    exit 1
  fi

  cp -r "$BASE_DIR/control-plane/migrations/global" "$stage/migrations/"
  cp -a "$BASE_DIR/deploy/global-plane.env.example" "$stage/global-plane.env.example"

  cat > "$stage/README.txt" <<GLOBAL_README
Parcel Global Plane

1) Create env file from example:
   cp global-plane.env.example global-plane.env

2) Edit global-plane.env — at minimum set:
   GLOBAL_DATABASE_URL, AUTH_JWT_SECRET, GLOBAL_TOKEN_ENCRYPTION_KEY

3) Run migrations and start:
   env \$(cat global-plane.env | xargs) ./global-plane

Notes:
- The global plane requires its own PostgreSQL database (separate from regional planes).
- Migrations are in ./migrations/global.
- Set GLOBAL_MIGRATIONS_DIR=migrations/global (default).
- The global plane communicates with regional control planes via their REST APIs.
  Each regional plane must have a service token with device.read, artifact.read,
  and federation.push scopes registered via POST /api/v1/planes on the global plane.
- For artifact federation (E2), configure GLOBAL_MINIO_* variables.
- See docs/global-desired-state.md and docs/artifact-federation.md.
GLOBAL_README

  printf "\nPlatform: %s/%s\n" "$goos" "$goarch" >> "$stage/README.txt"

  local out="$DIST_DIR/parcel-global-plane-${VERSION}-${goos}-${goarch}.tar.gz"
  package_dir "$stage" "$out" "$goos"
  rm -rf "$stage"
  echo "built $out"
}

for platform in $AGENT_PLATFORMS; do
  GOOS=${platform%/*}
  GOARCH=${platform#*/}
  build_agent "$GOOS" "$GOARCH"
  done

for platform in $CONTROL_PLANE_PLATFORMS; do
  GOOS=${platform%/*}
  GOARCH=${platform#*/}
  build_control_plane "$GOOS" "$GOARCH"
  done

GLOBAL_PLANE_PLATFORMS=${GLOBAL_PLANE_PLATFORMS:-$CONTROL_PLANE_PLATFORMS_DEFAULT}
for platform in $GLOBAL_PLANE_PLATFORMS; do
  GOOS=${platform%/*}
  GOARCH=${platform#*/}
  build_global_plane "$GOOS" "$GOARCH"
done

if [ "$BUILD_STACK" = "1" ]; then
  host_arch=$(uname -m)
  case "$host_arch" in
    x86_64) stack_arch=amd64 ;;
    aarch64|arm64) stack_arch=arm64 ;;
    *)
      echo "Unsupported host arch for stack bundle: $host_arch. Set BUILD_STACK=0 to skip." >&2
      stack_arch=""
      ;;
  esac
  if [ -n "$stack_arch" ]; then
    build_stack_bundle "$stack_arch"
  fi
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && sha256sum *.tar.gz *.zip > CHECKSUMS.txt 2>/dev/null || true)
fi

echo "Installers written to $DIST_DIR"
