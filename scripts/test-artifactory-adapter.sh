#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

RUN_SETUP=${RUN_SETUP:-1}
SETUP_OUTPUT_DIR=${SETUP_OUTPUT_DIR:-/tmp/hardwareops-artifactory-demo}

BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-admin@example.com}
AUTH_PASSWORD=${AUTH_PASSWORD:-change-me}
CI_SERVICE_TOKEN=${CI_SERVICE_TOKEN:-}

log() {
  echo "[test-artifactory-adapter] $*"
}

fail() {
  echo "[test-artifactory-adapter] ERROR: $*" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

need_cmd curl
need_cmd python3

if [ "$RUN_SETUP" = "1" ]; then
  log "running setup-artifactory-demo.sh"
  SETUP_OUTPUT_DIR="$SETUP_OUTPUT_DIR" "$BASE_DIR/scripts/setup-artifactory-demo.sh"
fi

env_file="${SETUP_OUTPUT_DIR}/artifactory-demo.env"
[ -f "$env_file" ] || fail "missing setup env file: $env_file"
# shellcheck disable=SC1090
source "$env_file"

[ -f "$PULL_PAYLOAD_FILE" ] || fail "missing pull payload file: $PULL_PAYLOAD_FILE"
[ -f "$CREDENTIALS_FILE" ] || fail "missing credentials file: $CREDENTIALS_FILE"

curl_opts=(-sS)
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  else
    fail "BASE_URL is HTTPS but CA cert not found at $CA_CERT_PATH (or set INSECURE=1)"
  fi
fi

log "checking control-plane reachability"
curl "${curl_opts[@]}" --fail "$BASE_URL/healthz" >/dev/null || fail "cannot reach $BASE_URL/healthz"

auth_status_json=$(curl "${curl_opts[@]}" --fail "$BASE_URL/api/v1/auth/status")
auth_enabled=$(python3 - <<'PY' "$auth_status_json"
import json, sys
print("1" if json.loads(sys.argv[1]).get("enabled") else "0")
PY
)

admin_header=()
publish_token="$CI_SERVICE_TOKEN"
if [ "$auth_enabled" = "1" ]; then
  admin_jwt="$AUTH_TOKEN"
  if [ -z "$admin_jwt" ]; then
    login_payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
    login_json=$(curl "${curl_opts[@]}" --fail \
      -X POST "$BASE_URL/api/v1/auth/login" \
      -H "Content-Type: application/json" \
      -d "$login_payload") || fail "login failed (check AUTH_EMAIL/AUTH_PASSWORD)"
    admin_jwt=$(python3 - <<'PY' "$login_json"
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
  fi
  [ -n "$admin_jwt" ] || fail "unable to obtain admin JWT"
  admin_header=(-H "Authorization: Bearer $admin_jwt")

  if [ -z "$publish_token" ]; then
    service_payload=$(python3 - <<'PY'
import json
print(json.dumps({
    "name": "artifactory-adapter-test",
    "scopes": ["artifact.publish"],
    "ttlHours": 1
}))
PY
)
    service_json=$(curl "${curl_opts[@]}" --fail \
      -X POST "$BASE_URL/api/v1/auth/service-tokens" \
      "${admin_header[@]}" \
      -H "Content-Type: application/json" \
      -d "$service_payload")
    publish_token=$(python3 - <<'PY' "$service_json"
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
  fi
fi

publish_header=()
if [ -n "$publish_token" ]; then
  publish_header=(-H "Authorization: Bearer $publish_token")
fi

log "calling /api/v1/artifacts/pull with artifactory source"
pull_response_file=$(mktemp)
pull_status=$(curl "${curl_opts[@]}" -o "$pull_response_file" -w "%{http_code}" \
  -X POST "$BASE_URL/api/v1/artifacts/pull" \
  "${publish_header[@]}" \
  -H "Content-Type: application/json" \
  --data @"$PULL_PAYLOAD_FILE" || true)

if [ "$pull_status" != "200" ]; then
  body=$(cat "$pull_response_file" 2>/dev/null || true)
  rm -f "$pull_response_file"
  if echo "$body" | grep -qi "credentialRef"; then
    fail "pull failed (${pull_status}): $body
Control-plane likely missing credential resolver config.
Restart with:
  ARTIFACT_PULL_CREDENTIALS_FILE=$CREDENTIALS_FILE"
  fi
  fail "pull failed (${pull_status}): $body"
fi

pull_json=$(cat "$pull_response_file")
rm -f "$pull_response_file"

artifact_id=$(python3 - <<'PY' "$pull_json"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId", ""))
PY
)
[ -n "$artifact_id" ] || fail "pull response missing artifactId"

verify_header=("${publish_header[@]}")
if [ "$auth_enabled" = "1" ]; then
  verify_header=("${admin_header[@]}")
fi

artifacts_json=$(curl "${curl_opts[@]}" --fail \
  "${verify_header[@]}" \
  "$BASE_URL/api/v1/artifacts?limit=200")

python3 - <<'PY' "$artifacts_json" "$artifact_id"
import json, sys
data = json.loads(sys.argv[1])
aid = sys.argv[2]
if not any(item.get("artifactId") == aid for item in data.get("items", [])):
    raise SystemExit(f"artifact {aid} not found in list")
PY

audit_json=$(curl "${curl_opts[@]}" --fail \
  "${verify_header[@]}" \
  "$BASE_URL/api/v1/audit?limit=200")

python3 - <<'PY' "$audit_json" "$artifact_id" "$CREDENTIAL_REF"
import json, sys
events = json.loads(sys.argv[1]).get("items", [])
aid = sys.argv[2]
cref = sys.argv[3]
matches = [e for e in events if e.get("action") == "artifact.pull" and e.get("targetId") == aid]
if not matches:
    raise SystemExit("missing artifact.pull audit event")
evt = matches[0]
metadata = evt.get("after") or "{}"
if isinstance(metadata, str) and metadata:
    try:
        metadata = json.loads(metadata)
    except Exception:
        metadata = {}
if isinstance(metadata, dict):
    if metadata.get("sourceKind") != "artifactory":
        raise SystemExit("audit metadata sourceKind != artifactory")
    if metadata.get("credentialRef") != cref:
        raise SystemExit("audit metadata credentialRef mismatch")
PY

log "PASS"
log "artifactId=$artifact_id"
