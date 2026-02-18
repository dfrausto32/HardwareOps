#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}

AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-admin@example.com}
AUTH_PASSWORD=${AUTH_PASSWORD:-change-me}
CI_SERVICE_TOKEN=${CI_SERVICE_TOKEN:-}

ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
WORK_DIR=${WORK_DIR:-/tmp/hardwareops-ingest-test}
HTTP_PORT=${HTTP_PORT:-18080}
RUN_NEGATIVE_SHA_TEST=${RUN_NEGATIVE_SHA_TEST:-1}
KEEP_WORK_DIR=${KEEP_WORK_DIR:-0}
PULL_SOURCE_MODE=${PULL_SOURCE_MODE:-source}

RUN_ID=${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
PUSH_ARTIFACT_NAME=${PUSH_ARTIFACT_NAME:-push-demo-$RUN_ID}
PULL_ARTIFACT_NAME=${PULL_ARTIFACT_NAME:-pull-demo-$RUN_ID}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-0.0.1}
TTL_HOURS=${TTL_HOURS:-1}

log() {
  echo "[test-artifact-ingest] $*"
}

fail() {
  echo "[test-artifact-ingest] ERROR: $*" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

need_cmd curl
need_cmd python3
need_cmd sha256sum

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

server_pid=""
cleanup() {
  if [ -n "$server_pid" ] && kill -0 "$server_pid" >/dev/null 2>&1; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
  if [ "$KEEP_WORK_DIR" != "1" ]; then
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

mkdir -p "$WORK_DIR/input"

log "checking control-plane auth status"
auth_status_json=$(curl "${curl_opts[@]}" --fail "$BASE_URL/api/v1/auth/status") || fail "cannot reach $BASE_URL/api/v1/auth/status"

auth_enabled=$(python3 - <<'PY' "$auth_status_json"
import json, sys
data = json.loads(sys.argv[1])
print("1" if data.get("enabled") else "0")
PY
)

admin_header=()
publish_token="$CI_SERVICE_TOKEN"

if [ "$auth_enabled" = "1" ]; then
  log "auth is enabled"
  admin_jwt="$AUTH_TOKEN"
  if [ -z "$admin_jwt" ]; then
    log "logging in as $AUTH_EMAIL"
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
    log "creating scoped service token (artifact.publish)"
    service_payload=$(python3 - <<'PY' "$RUN_ID" "$TTL_HOURS"
import json, sys
print(json.dumps({
    "name": f"ingest-test-{sys.argv[1]}",
    "scopes": ["artifact.publish"],
    "ttlHours": int(sys.argv[2]),
}))
PY
)
    service_json=$(curl "${curl_opts[@]}" --fail \
      -X POST "$BASE_URL/api/v1/auth/service-tokens" \
      "${admin_header[@]}" \
      -H "Content-Type: application/json" \
      -d "$service_payload") || fail "failed creating service token"
    publish_token=$(python3 - <<'PY' "$service_json"
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
  fi
else
  log "auth is disabled; using no-auth publish token placeholder"
  if [ -z "$publish_token" ]; then
    publish_token="no-auth-token"
  fi
fi

[ -n "$publish_token" ] || fail "publish token is empty"
publish_header=(-H "Authorization: Bearer $publish_token")

cat > "$WORK_DIR/input/readme.txt" <<EOF
artifact ingest push/pull smoke test
runId=$RUN_ID
EOF
date -u +"%Y-%m-%dT%H:%M:%SZ" > "$WORK_DIR/input/build.txt"

push_tar="$WORK_DIR/${PUSH_ARTIFACT_NAME}-${ARTIFACT_VERSION}.tar.gz"

log "running push ingest test via scripts/ci-upload-artifact.sh"
push_json=$(
  ARTIFACT_NAME="$PUSH_ARTIFACT_NAME" \
  ARTIFACT_VERSION="$ARTIFACT_VERSION" \
  ARTIFACT_TYPE="$ARTIFACT_TYPE" \
  INPUT_DIR="$WORK_DIR/input" \
  OUT_PATH="$push_tar" \
  BASE_URL="$BASE_URL" \
  CA_CERT_PATH="$CA_CERT_PATH" \
  INSECURE="$INSECURE" \
  CI_SERVICE_TOKEN="$publish_token" \
  "$BASE_DIR/scripts/ci-upload-artifact.sh"
) || fail "push ingest failed"

push_artifact_id=$(python3 - <<'PY' "$push_json"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId", ""))
PY
)
[ -n "$push_artifact_id" ] || fail "push ingest did not return artifactId"

push_sha=$(sha256sum "$push_tar" | awk '{print $1}')

log "starting local pull source server on 127.0.0.1:$HTTP_PORT"
(
  cd "$WORK_DIR"
  python3 -m http.server "$HTTP_PORT" --bind 127.0.0.1
) >"$WORK_DIR/http-server.log" 2>&1 &
server_pid=$!

for _ in $(seq 1 30); do
  if curl -s "http://127.0.0.1:$HTTP_PORT/" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
if ! curl -s "http://127.0.0.1:$HTTP_PORT/" >/dev/null 2>&1; then
  fail "local pull source server did not start"
fi

pull_payload=$(python3 - <<'PY' "$PULL_ARTIFACT_NAME" "$ARTIFACT_VERSION" "$ARTIFACT_TYPE" "$HTTP_PORT" "$(basename "$push_tar")" "$push_sha" "$PULL_SOURCE_MODE"
import json, sys
payload = {
    "name": sys.argv[1],
    "version": sys.argv[2],
    "type": sys.argv[3],
    "sha256": sys.argv[6],
}
source_url = f"http://localhost:{sys.argv[4]}/{sys.argv[5]}"
mode = sys.argv[7].strip().lower()
if mode == "legacy":
    payload["sourceUrl"] = source_url
else:
    payload["source"] = {"kind": "http", "uri": source_url}
print(json.dumps(payload))
PY
)

log "running pull ingest test via /api/v1/artifacts/pull"
pull_json=$(curl "${curl_opts[@]}" --fail \
  -X POST "$BASE_URL/api/v1/artifacts/pull" \
  "${publish_header[@]}" \
  -H "Content-Type: application/json" \
  -d "$pull_payload") || fail "pull ingest failed (check ARTIFACT_PULL_ALLOWED_HOSTS / source reachability)"

pull_artifact_id=$(python3 - <<'PY' "$pull_json"
import json, sys
print(json.loads(sys.argv[1]).get("artifactId", ""))
PY
)
[ -n "$pull_artifact_id" ] || fail "pull ingest did not return artifactId"

verify_header=()
if [ "$auth_enabled" = "1" ]; then
  verify_header=("${admin_header[@]}")
fi

log "verifying artifact list contains pushed + pulled artifacts"
artifacts_json=$(curl "${curl_opts[@]}" --fail \
  "${verify_header[@]}" \
  "$BASE_URL/api/v1/artifacts?limit=200")

python3 - <<'PY' "$artifacts_json" "$PUSH_ARTIFACT_NAME" "$PULL_ARTIFACT_NAME"
import json, sys
data = json.loads(sys.argv[1])
items = data.get("items", [])
names = {item.get("name") for item in items}
missing = [name for name in (sys.argv[2], sys.argv[3]) if name not in names]
if missing:
    raise SystemExit(f"missing artifacts in list response: {missing}")
PY

log "verifying audit contains pull + upload-complete events"
audit_json=$(curl "${curl_opts[@]}" --fail \
  "${verify_header[@]}" \
  "$BASE_URL/api/v1/audit?limit=200")

python3 - <<'PY' "$audit_json" "$push_artifact_id" "$pull_artifact_id"
import json, sys
data = json.loads(sys.argv[1])
items = data.get("items", [])
push_id = sys.argv[2]
pull_id = sys.argv[3]
has_push_complete = any(i.get("action") == "artifact.upload.complete" and i.get("targetId") == push_id for i in items)
has_pull = any(i.get("action") == "artifact.pull" and i.get("targetId") == pull_id for i in items)
if not has_push_complete or not has_pull:
    raise SystemExit("missing expected audit events for push/pull")
PY

if [ "$RUN_NEGATIVE_SHA_TEST" = "1" ]; then
  log "running negative test (sha mismatch)"
  bad_payload=$(python3 - <<'PY' "$RUN_ID" "$HTTP_PORT" "$(basename "$push_tar")" "$PULL_SOURCE_MODE"
import json, sys
payload = {
    "name": f"pull-bad-sha-{sys.argv[1]}",
    "version": "0.0.1",
    "type": "app_bundle",
    "sha256": "deadbeef",
}
source_url = f"http://localhost:{sys.argv[2]}/{sys.argv[3]}"
mode = sys.argv[4].strip().lower()
if mode == "legacy":
    payload["sourceUrl"] = source_url
else:
    payload["source"] = {"kind": "http", "uri": source_url}
print(json.dumps(payload))
PY
)
  bad_code=$(curl "${curl_opts[@]}" -o "$WORK_DIR/bad-sha-response.txt" -w "%{http_code}" \
    -X POST "$BASE_URL/api/v1/artifacts/pull" \
    "${publish_header[@]}" \
    -H "Content-Type: application/json" \
    -d "$bad_payload" || true)
  if [ "$bad_code" != "400" ]; then
    fail "expected 400 for bad sha pull test, got $bad_code"
  fi
fi

log "PASS"
log "push artifact: name=$PUSH_ARTIFACT_NAME id=$push_artifact_id"
log "pull artifact: name=$PULL_ARTIFACT_NAME id=$pull_artifact_id"
if [ "$KEEP_WORK_DIR" = "1" ]; then
  log "work dir retained at $WORK_DIR"
fi
