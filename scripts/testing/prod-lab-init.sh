#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/parcel-prod-docker"}
ENV_TEMPLATE=${ENV_TEMPLATE:-"$BASE_DIR/deploy/compose/.env.onprem.example"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
CERTS_DIR=${CERTS_DIR:-"$LAB_ROOT/certs"}
STACK_DIR=${STACK_DIR:-"$LAB_ROOT/stack"}

DOMAIN=${DOMAIN:-hwops.localhost}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-"https://$DOMAIN"}
AGENT_BASE_URL=${AGENT_BASE_URL:-"https://agent.$DOMAIN"}
ADMIN_EMAIL=${ADMIN_EMAIL:-admin@example.com}
LICENSE_ISSUED_TO=${LICENSE_ISSUED_TO:-prod-docker-lab}
MAX_DEVICES=${MAX_DEVICES:-200}
ARTIFACT_PULL_ALLOWED_HOSTS=${ARTIFACT_PULL_ALLOWED_HOSTS:-localhost,127.0.0.1,host.docker.internal}
COMPOSE_SUBNET=${COMPOSE_SUBNET:-172.29.240.0/24}
TRUST_PROXY_CIDRS=${TRUST_PROXY_CIDRS:-127.0.0.1/32,::1/128,$COMPOSE_SUBNET}
FORCE=${FORCE:-0}

if [ ! -f "$ENV_TEMPLATE" ]; then
  echo "Env template not found: $ENV_TEMPLATE" >&2
  exit 1
fi

if [ -f "$ENV_FILE" ] && [ "$FORCE" != "1" ]; then
  echo "Lab env already exists: $ENV_FILE (set FORCE=1 to overwrite)." >&2
  exit 1
fi

mkdir -p "$LAB_ROOT" "$CERTS_DIR" "$STACK_DIR"
mkdir -p "$STACK_DIR/signing" "$STACK_DIR/license-keys"

random_hex() {
  local bytes="$1"
  openssl rand -hex "$bytes"
}

AUTH_BOOTSTRAP_PASSWORD=${AUTH_BOOTSTRAP_PASSWORD:-"Lab-$(random_hex 12)"}
AUTH_JWT_SECRET=${AUTH_JWT_SECRET:-$(random_hex 32)}
BOOTSTRAP_TOKEN=${BOOTSTRAP_TOKEN:-$(random_hex 24)}
MAINTENANCE_TOKEN=${MAINTENANCE_TOKEN:-$(random_hex 24)}
UPGRADE_RUNNER_TOKEN=${UPGRADE_RUNNER_TOKEN:-$(random_hex 24)}
BACKUP_RUNNER_TOKEN=${BACKUP_RUNNER_TOKEN:-$(random_hex 24)}

cp "$ENV_TEMPLATE" "$ENV_FILE"

OUT_DIR="$CERTS_DIR" \
DOMAIN="$DOMAIN" \
AGENT_DOMAIN="agent.$DOMAIN" \
ENV_OUT="$LAB_ROOT/control-plane.env" \
FORCE=1 \
"$BASE_DIR/scripts/setup-control-plane.sh" >/dev/null 2>&1

cp -f "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca-bundle.crt"
cp -f "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca-active.crt"
cp -f "$CERTS_DIR/ca.key" "$CERTS_DIR/ca-active.key"

"$BASE_DIR/scripts/generate-signing-key.sh" "$STACK_DIR/signing" >/dev/null
ARTIFACT_SIGNATURE_KEY_ID=$(cat "$STACK_DIR/signing/ed25519.keyid")
"$BASE_DIR/scripts/build-trusted-signing-keys.sh" \
  "$STACK_DIR/signing/trusted-signing-keys.json" \
  "$STACK_DIR/signing/ed25519.pub" >/dev/null

"$BASE_DIR/scripts/generate-signing-key.sh" "$STACK_DIR/license-keys" >/dev/null
LICENSE_KEY="$STACK_DIR/license-keys/ed25519.key" \
OUT="$STACK_DIR/license.json" \
ISSUED_TO="$LICENSE_ISSUED_TO" \
MAX_DEVICES="$MAX_DEVICES" \
"$BASE_DIR/scripts/sign-license.sh" >/dev/null

python3 - "$ENV_FILE" \
  "$DOMAIN" "$PUBLIC_BASE_URL" "$AGENT_BASE_URL" \
  "$CERTS_DIR" "$STACK_DIR" \
  "$ADMIN_EMAIL" "$AUTH_BOOTSTRAP_PASSWORD" "$AUTH_JWT_SECRET" "$BOOTSTRAP_TOKEN" \
  "$MAINTENANCE_TOKEN" "$UPGRADE_RUNNER_TOKEN" "$BACKUP_RUNNER_TOKEN" \
  "$ARTIFACT_SIGNATURE_KEY_ID" "$ARTIFACT_PULL_ALLOWED_HOSTS" "$TRUST_PROXY_CIDRS" "$COMPOSE_SUBNET" <<'PY'
import re
import sys

(
    env_file,
    domain,
    public_base_url,
    agent_base_url,
    certs_dir,
    stack_dir,
    admin_email,
    admin_password,
    jwt_secret,
    bootstrap_token,
    maintenance_token,
    upgrade_runner_token,
    backup_runner_token,
    artifact_key_id,
    pull_allowed_hosts,
    trust_proxy_cidrs,
    compose_subnet,
) = sys.argv[1:]

updates = {
    "DOMAIN": domain,
    "PUBLIC_BASE_URL": public_base_url,
    "AGENT_BASE_URL": agent_base_url,
    "CERTS_DIR": certs_dir,
    "STACK_DIR": stack_dir,
    "CORS_ALLOWED_ORIGINS": public_base_url,
    "AUTH_MODE": "local",
    "AUTH_BOOTSTRAP_EMAIL": admin_email,
    "AUTH_BOOTSTRAP_PASSWORD": admin_password,
    "AUTH_JWT_SECRET": jwt_secret,
    "BOOTSTRAP_TOKEN": bootstrap_token,
    "AUTH_LOGIN_BACKOFF_ENABLED": "1",
    "MAINTENANCE_MODE": "0",
    "MAINTENANCE_TOKEN": maintenance_token,
    "UPGRADE_RUNNER_TOKEN": upgrade_runner_token,
    "BACKUP_RUNNER_TOKEN": backup_runner_token,
    "HARDENED_PROFILE": "1",
    "LICENSE_ENFORCE": "1",
    "LICENSE_PATH": "/stack/license.json",
    "LICENSE_KEY_MODE": "file",
    "LICENSE_PUBLIC_KEY_PATH": "/stack/license-keys/ed25519.pub",
    "DEVICE_IDENTITY_MODE": "enforce",
    "DEVICE_IDENTITY_REQUIRE_ON_ENROLL": "1",
    "DEVICE_IDENTITY_REQUIRE_ON_CHECKIN": "1",
    "ARTIFACT_PULL_ALLOWED_HOSTS": pull_allowed_hosts,
    "ARTIFACT_PULL_ALLOW_INSECURE_HTTP": "0",
    "ARTIFACT_TRUST_VERIFICATION_MODE": "require_verified",
    "ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS": artifact_key_id,
    "ARTIFACT_TRUST_ALLOWED_SIGNATURE_TYPES": "ed25519",
    "TRUSTED_SIGNING_KEYS_FILE": "/stack/signing/trusted-signing-keys.json",
    "ARTIFACT_SIGNATURE_REQUIRE_DEFAULT": "1",
    "ARTIFACT_SIGNATURE_ENFORCE_INGEST": "1",
    "ARTIFACT_SIGNATURE_KEY_ID": artifact_key_id,
    "UPGRADE_RUNNER_MODE": "remote",
    "BACKUP_RUNNER_MODE": "remote",
    "TRUST_PROXY": "1",
    "TRUST_PROXY_CIDRS": trust_proxy_cidrs,
    "COMPOSE_SUBNET": compose_subnet,
}

with open(env_file, "r", encoding="utf-8") as f:
    lines = f.read().splitlines()

out = []
seen = set()
for line in lines:
    replaced = False
    for key, value in updates.items():
        if re.match(rf"^{re.escape(key)}=", line):
            out.append(f"{key}={value}")
            seen.add(key)
            replaced = True
            break
    if not replaced:
        out.append(line)

for key, value in updates.items():
    if key not in seen:
        out.append(f"{key}={value}")

with open(env_file, "w", encoding="utf-8") as f:
    f.write("\n".join(out) + "\n")
PY

cat <<EOF
Prod lab initialized.

Lab root:            $LAB_ROOT
Env file:            $ENV_FILE
Certs dir:           $CERTS_DIR
Stack dir:           $STACK_DIR
Domain:              $DOMAIN
Public URL:          $PUBLIC_BASE_URL
Agent URL:           $AGENT_BASE_URL
Bootstrap admin:     $ADMIN_EMAIL
Bootstrap password:  $AUTH_BOOTSTRAP_PASSWORD
Artifact key id:     $ARTIFACT_SIGNATURE_KEY_ID
Trusted key file:    $STACK_DIR/signing/trusted-signing-keys.json
License file:        $STACK_DIR/license.json
Trusted proxies:     $TRUST_PROXY_CIDRS
Compose subnet:      $COMPOSE_SUBNET

Next steps:
  1) scripts/testing/prod-lab-up.sh
  2) scripts/testing/prod-lab-smoke.sh
EOF
