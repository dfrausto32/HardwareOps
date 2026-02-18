#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}

AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}

APP_NAMES=${APP_NAMES:-customer,integration}
VERSIONS=${VERSIONS:-0.1.0,0.2.0}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
COMPONENT_PREFIX=${COMPONENT_PREFIX:-app:}
OUT_DIR=${OUT_DIR:-/tmp/hardwareops-aws-demo}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}

if [ -z "$BASE_URL" ]; then
  cat <<'USAGE' >&2
Usage:
  BASE_URL=https://app.example.com \
  AUTH_EMAIL=admin@example.com AUTH_PASSWORD='...' \
  ./scripts/aws-demo-seed.sh

Optional:
  AUTH_TOKEN=<jwt>                 # skips login
  CA_CERT_PATH=/path/to/ca.crt     # for private CA
  INSECURE=1                       # for quick test only
  APP_NAMES=customer,integration
  VERSIONS=0.1.0,0.2.0
USAGE
  exit 1
fi

if [ "$SIGN_ARTIFACTS" = "1" ]; then
  source "$BASE_DIR/scripts/ensure-signing-key.sh"
fi

BASE_URL="$BASE_URL" \
CA_CERT_PATH="$CA_CERT_PATH" \
INSECURE="$INSECURE" \
AUTH_TOKEN="$AUTH_TOKEN" \
AUTH_EMAIL="$AUTH_EMAIL" \
AUTH_PASSWORD="$AUTH_PASSWORD" \
APP_NAMES="$APP_NAMES" \
VERSIONS="$VERSIONS" \
ARTIFACT_TYPE="$ARTIFACT_TYPE" \
COMPONENT_PREFIX="$COMPONENT_PREFIX" \
OUT_DIR="$OUT_DIR" \
SIGN_ARTIFACTS="$SIGN_ARTIFACTS" \
"$BASE_DIR/scripts/multi-app-artifacts.sh"

cat <<EOF
Demo artifacts uploaded successfully.

Next:
  1) Open the UI Devices page.
  2) Edit a device's desired components.
  3) Set component keys like "${COMPONENT_PREFIX}customer" or "${COMPONENT_PREFIX}integration".
  4) Choose uploaded versions manually to switch artifacts.
EOF
