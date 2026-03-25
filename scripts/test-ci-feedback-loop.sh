#!/usr/bin/env bash
# test-ci-feedback-loop.sh — Integration smoke test for CI/CD feedback-loop features:
#   1. Service token scope enforcement
#   2. Outbound webhook delivery (with local HTTP receiver)
#   3. Deploy trigger endpoint (device + group)
#
# Usage:
#   AUTH_EMAIL=admin@example.com AUTH_PASSWORD=secret \
#   BASE_URL=https://localhost:8080 \
#   ./scripts/test-ci-feedback-loop.sh
#
# Optional env vars:
#   AUTH_TOKEN=<jwt>             Reuse existing JWT instead of logging in
#   CA_CERT_PATH=/path/ca.crt    Custom CA cert for private TLS
#   INSECURE=1                   Skip TLS verification
#   WEBHOOK_PORT=9876            Local port for the in-process webhook receiver
#   WEBHOOK_PUBLIC_URL=https://.../hook
#                                Publicly reachable callback URL for live webhook delivery
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}
AUTH_TOKEN=${AUTH_TOKEN:-}
WEBHOOK_PORT=${WEBHOOK_PORT:-9876}
WEBHOOK_PUBLIC_URL=${WEBHOOK_PUBLIC_URL:-}

PASS=0
FAIL=0
SKIP=0
TMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/hardwareops-ci-feedback-XXXXXX")
LISTENER_PID=""
WEBHOOK_MODE_FILE="$TMP_ROOT/webhook-mode"

cleanup() {
  [ -n "$LISTENER_PID" ] && kill "$LISTENER_PID" 2>/dev/null || true
  rm -rf "$TMP_ROOT"
  echo ""
  echo "Results: $PASS passed  $FAIL failed  $SKIP skipped"
  [ "$FAIL" -eq 0 ] || exit 1
}
trap cleanup EXIT

usage() {
  cat <<USAGE
Usage:
  AUTH_EMAIL=admin@example.com AUTH_PASSWORD=secret \\
  BASE_URL=https://app.example.com \\
  ./scripts/test-ci-feedback-loop.sh

Optional:
  AUTH_TOKEN=<jwt>             Reuse existing JWT instead of logging in
  CA_CERT_PATH=/path/ca.crt    Custom CA for private TLS
  INSECURE=1                   Skip TLS verification
  WEBHOOK_PORT=9876            Local port for webhook receiver (default: 9876)
  WEBHOOK_PUBLIC_URL=https://.../hook
                                Public webhook callback URL for live environments
USAGE
}

if [ -z "$AUTH_TOKEN" ] && { [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; }; then
  usage >&2
  exit 1
fi

CURL_OPTS=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    CURL_OPTS+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    CURL_OPTS+=(-k)
  fi
fi

json_field() {
  python3 - <<'PY' "$1" "$2"
import json, sys
try:
    obj = json.loads(sys.argv[1])
except Exception:
    print("")
    raise SystemExit(0)
if isinstance(obj, dict):
    print(obj.get(sys.argv[2], ""))
else:
    print("")
PY
}

# api_call <METHOD> <path> [extra curl args…]
# Populates API_STATUS and API_BODY.
api_call() {
  local method=$1
  local path=$2
  shift 2
  local body_file="$TMP_ROOT/last-body.json"
  API_STATUS=$(curl -sS "${CURL_OPTS[@]}" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -X "$method" \
    -o "$body_file" -w '%{http_code}' \
    "$BASE_URL$path" "$@" || true)
  API_BODY=$(cat "$body_file" 2>/dev/null || true)
}

pass() { echo "[${1}] PASS${2:+ — $2}"; PASS=$((PASS+1)); }
fail() { echo "[${1}] FAIL${2:+ — $2}" >&2; FAIL=$((FAIL+1)); }
skip() { echo "[${1}] SKIP${2:+ — $2}"; SKIP=$((SKIP+1)); }

login() {
  [ -n "$AUTH_TOKEN" ] && return
  local payload
  payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
  local body_file="$TMP_ROOT/login.json"
  local status
  status=$(curl -sS "${CURL_OPTS[@]}" \
    -H 'Content-Type: application/json' \
    -X POST "$BASE_URL/api/v1/auth/login" \
    -d "$payload" -o "$body_file" -w '%{http_code}' || true)
  local resp
  resp=$(cat "$body_file" 2>/dev/null || true)
  AUTH_TOKEN=$(json_field "$resp" token)
  if [ -z "$AUTH_TOKEN" ]; then
    echo "Login failed (http=$status): $resp" >&2
    exit 1
  fi
}

# ─── Setup ───────────────────────────────────────────────────────────────────
echo "=== HardwareOps CI feedback-loop integration test ==="
echo "Base URL: $BASE_URL"
if [ -n "$WEBHOOK_PUBLIC_URL" ]; then
  echo "Webhook callback: $WEBHOOK_PUBLIC_URL"
else
  echo "Webhook callback: http://127.0.0.1:$WEBHOOK_PORT/hook"
fi
login
echo "Authenticated."
echo ""

STAMP=$(date +%Y%m%d%H%M%S)
ADMIN_TOKEN=$AUTH_TOKEN   # preserve for switching back

# ─── Section 1: Service token scope enforcement ───────────────────────────────
echo "--- 1. Service token scope enforcement ---"

# 1a. Create a token with artifact.publish scope
payload=$(python3 - <<'PY' "$STAMP"
import json, sys
print(json.dumps({"name": f"ci-publish-{sys.argv[1]}", "scopes": ["artifact.publish"]}))
PY
)
api_call POST /api/v1/auth/service-tokens -H 'Content-Type: application/json' -d "$payload"
PUBLISH_TOKEN=$(json_field "$API_BODY" token)
if [ -n "$PUBLISH_TOKEN" ]; then
  pass "scope/create-publish-token" "http=$API_STATUS"
else
  fail "scope/create-publish-token" "no token in response http=$API_STATUS body=$API_BODY"
  PUBLISH_TOKEN=""
fi

# 1b. Create a token with device.read scope only
payload=$(python3 - <<'PY' "$STAMP"
import json, sys
print(json.dumps({"name": f"ci-read-{sys.argv[1]}", "scopes": ["device.read"]}))
PY
)
api_call POST /api/v1/auth/service-tokens -H 'Content-Type: application/json' -d "$payload"
READ_TOKEN=$(json_field "$API_BODY" token)
if [ -n "$READ_TOKEN" ]; then
  pass "scope/create-read-only-token" "http=$API_STATUS"
else
  fail "scope/create-read-only-token" "no token in response http=$API_STATUS body=$API_BODY"
  READ_TOKEN=""
fi

# 1c. artifact.publish token on presign-upload → auth passes (expects 400 bad request,
#     not 401/403 — the handler rejects the empty body, meaning the scope check passed).
if [ -n "$PUBLISH_TOKEN" ]; then
  AUTH_TOKEN=$PUBLISH_TOKEN
  api_call POST /api/v1/artifacts/presign-upload -H 'Content-Type: application/json' -d '{}'
  AUTH_TOKEN=$ADMIN_TOKEN
  if [[ "$API_STATUS" != "401" && "$API_STATUS" != "403" ]]; then
    pass "scope/publish-token-auth-accepted" "auth passed (handler returned http=$API_STATUS)"
  else
    fail "scope/publish-token-auth-accepted" "scope check should pass but got http=$API_STATUS"
  fi
fi

# 1d. device.read token on presign-upload → must be rejected with 403
if [ -n "$READ_TOKEN" ]; then
  AUTH_TOKEN=$READ_TOKEN
  api_call POST /api/v1/artifacts/presign-upload -H 'Content-Type: application/json' -d '{}'
  AUTH_TOKEN=$ADMIN_TOKEN
  if [ "$API_STATUS" = "403" ]; then
    pass "scope/read-token-blocked-on-presign" "http=403 as expected"
  else
    fail "scope/read-token-blocked-on-presign" "expected 403, got http=$API_STATUS body=$API_BODY"
  fi
fi

# 1e. Create a deployment.trigger-scoped token for use in section 3
payload=$(python3 - <<'PY' "$STAMP"
import json, sys
print(json.dumps({"name": f"ci-trigger-{sys.argv[1]}", "scopes": ["deployment.trigger"]}))
PY
)
api_call POST /api/v1/auth/service-tokens -H 'Content-Type: application/json' -d "$payload"
TRIGGER_TOKEN=$(json_field "$API_BODY" token)
if [ -n "$TRIGGER_TOKEN" ]; then
  pass "scope/create-trigger-token" "http=$API_STATUS"
else
  fail "scope/create-trigger-token" "no token in response http=$API_STATUS body=$API_BODY"
  TRIGGER_TOKEN=""
fi

# 1f. Create an artifact.read token
payload=$(python3 - <<'PY' "$STAMP"
import json, sys
print(json.dumps({"name": f"ci-artifact-read-{sys.argv[1]}", "scopes": ["artifact.read"]}))
PY
)
api_call POST /api/v1/auth/service-tokens -H 'Content-Type: application/json' -d "$payload"
ARTIFACT_READ_TOKEN=$(json_field "$API_BODY" token)
if [ -n "$ARTIFACT_READ_TOKEN" ]; then
  pass "scope/create-artifact-read-token" "http=$API_STATUS"
else
  fail "scope/create-artifact-read-token" "no token in response http=$API_STATUS body=$API_BODY"
  ARTIFACT_READ_TOKEN=""
fi

# 1g. Create a webhook.manage token
payload=$(python3 - <<'PY' "$STAMP"
import json, sys
print(json.dumps({"name": f"ci-webhook-manage-{sys.argv[1]}", "scopes": ["webhook.manage"]}))
PY
)
api_call POST /api/v1/auth/service-tokens -H 'Content-Type: application/json' -d "$payload"
WEBHOOK_TOKEN=$(json_field "$API_BODY" token)
if [ -n "$WEBHOOK_TOKEN" ]; then
  pass "scope/create-webhook-manage-token" "http=$API_STATUS"
else
  fail "scope/create-webhook-manage-token" "no token in response http=$API_STATUS body=$API_BODY"
  WEBHOOK_TOKEN=""
fi

if [ -n "$ARTIFACT_READ_TOKEN" ]; then
  AUTH_TOKEN=$ARTIFACT_READ_TOKEN
  api_call GET /api/v1/artifacts
  AUTH_TOKEN=$ADMIN_TOKEN
  if [ "$API_STATUS" = "200" ]; then
    pass "scope/artifact-read-token-allowed" "http=200"
  else
    fail "scope/artifact-read-token-allowed" "expected 200, got http=$API_STATUS body=$API_BODY"
  fi
fi

if [ -n "$WEBHOOK_TOKEN" ]; then
  AUTH_TOKEN=$WEBHOOK_TOKEN
  api_call GET /api/v1/webhooks
  AUTH_TOKEN=$ADMIN_TOKEN
  if [ "$API_STATUS" = "200" ]; then
    pass "scope/webhook-manage-token-allowed" "http=200"
  else
    fail "scope/webhook-manage-token-allowed" "expected 200, got http=$API_STATUS body=$API_BODY"
  fi
fi

echo ""

# ─── Section 2: Outbound webhook delivery ─────────────────────────────────────
echo "--- 2. Outbound webhook delivery ---"

# 2a. Start a local HTTP receiver (records the first POST to a JSON file)
WEBHOOK_RECEIVED="$TMP_ROOT/webhook-received.json"
WEBHOOK_TARGET_URL=${WEBHOOK_PUBLIC_URL:-http://127.0.0.1:$WEBHOOK_PORT/hook}
echo ok > "$WEBHOOK_MODE_FILE"
cat > "$TMP_ROOT/webhook-server.py" <<'PYEOF'
import sys, json, signal
from http.server import HTTPServer, BaseHTTPRequestHandler

received_file = sys.argv[1]
port = int(sys.argv[2])
mode_file = sys.argv[3]

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        with open(received_file, "w") as f:
            json.dump({
                "path": self.path,
                "signature": self.headers.get("X-HardwareOps-Signature", ""),
                "body": body.decode("utf-8", errors="replace"),
            }, f)
        mode = "ok"
        try:
            mode = open(mode_file).read().strip()
        except Exception:
            pass
        self.send_response(502 if mode == "fail" else 200)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, *args):
        pass

signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PYEOF

python3 "$TMP_ROOT/webhook-server.py" "$WEBHOOK_RECEIVED" "$WEBHOOK_PORT" "$WEBHOOK_MODE_FILE" &
LISTENER_PID=$!
sleep 0.4   # allow socket to bind

# 2b. Create webhook pointing at the callback URL, subscribing to ping + deployment.triggered
payload=$(python3 - <<'PY' "$STAMP" "$WEBHOOK_TARGET_URL"
import json, sys
print(json.dumps({
    "name": f"ci-webhook-{sys.argv[1]}",
    "url": sys.argv[2],
    "eventTypes": ["ping", "deployment.triggered"],
    "enabled": True,
}))
PY
)
api_call POST /api/v1/webhooks -H 'Content-Type: application/json' -d "$payload"
WEBHOOK_ID=$(json_field "$API_BODY" id)
WEBHOOK_SECRET=$(json_field "$API_BODY" secret)
if [ -n "$WEBHOOK_ID" ]; then
  pass "webhook/create" "id=$WEBHOOK_ID http=$API_STATUS"
else
  fail "webhook/create" "no id in response http=$API_STATUS body=$API_BODY"
  WEBHOOK_ID=""
fi

if [ -n "$WEBHOOK_ID" ]; then
  # 2c. Fire a test ping through the webhook
  api_call POST "/api/v1/webhooks/$WEBHOOK_ID/test"
  if [[ "$API_STATUS" =~ ^2 ]]; then
    pass "webhook/test-ping-queued" "http=$API_STATUS"
  else
    fail "webhook/test-ping-queued" "expected 2xx, got http=$API_STATUS body=$API_BODY"
  fi

  # 2d. Poll deliveries endpoint (dispatcher is async — allow up to 8s)
  DELIVERY_OK=0
  for _attempt in 1 2 3 4 5 6 7 8; do
    sleep 1
    api_call GET "/api/v1/webhooks/$WEBHOOK_ID/deliveries"
    if python3 - <<'PY' "$API_BODY"; then
import json, sys
try:
    data = json.loads(sys.argv[1])
    items = data if isinstance(data, list) else data.get("deliveries", data.get("items", []))
    for d in items:
        # "pending" means not yet attempted; anything else (success/failed) means dispatched
        if d.get("attempts", 0) > 0:
            sys.exit(0)
except Exception:
    pass
sys.exit(1)
PY
      DELIVERY_OK=1
      break
    fi
  done

  if [ "$DELIVERY_OK" = "1" ]; then
    pass "webhook/delivery-dispatched" "at least one delivery attempt recorded"
  else
    fail "webhook/delivery-dispatched" "no attempted delivery after 8s; response=$API_BODY"
  fi

  # 2e. Verify exact HMAC signature using the returned webhook secret
  if [ -f "$WEBHOOK_RECEIVED" ] && [ -n "$WEBHOOK_SECRET" ]; then
    if python3 - <<'PY' "$WEBHOOK_RECEIVED" "$WEBHOOK_SECRET"
import hashlib, hmac, json, sys
data = json.load(open(sys.argv[1]))
body = data.get("body", "").encode()
sig = data.get("signature", "")
want = "sha256=" + hmac.new(sys.argv[2].encode(), body, hashlib.sha256).hexdigest()
sys.exit(0 if sig == want else 1)
PY
    then
      pass "webhook/hmac-signature" "signature matched expected HMAC"
    else
      fail "webhook/hmac-signature" "signature did not match expected HMAC"
    fi
  else
    skip "webhook/hmac-signature" "receiver file or webhook secret unavailable"
  fi
fi

echo ""

# ─── Section 3: Deploy trigger ────────────────────────────────────────────────
echo "--- 3. Deploy trigger ---"

# 3a. List devices — find the first active one
api_call GET /api/v1/devices
DEVICE_ID=$(python3 - <<'PY' "$API_BODY"
import json, sys
try:
    data = json.loads(sys.argv[1])
    devices = data if isinstance(data, list) else data.get("devices", data.get("items", []))
    for d in devices:
        if d.get("status") not in ("decommissioned",):
            print(d.get("id", "") or d.get("deviceId", ""))
            break
except Exception:
    pass
PY
)

if [ -z "$DEVICE_ID" ]; then
  skip "trigger/device-apply" "no enrolled devices — run: ./scripts/run-demo-agent.sh"
  skip "trigger/scoped-token-accepted" "no enrolled devices"
  skip "trigger/wrong-scope-rejected" "no enrolled devices"
  echo ""
  echo "  To observe immediateRecheckin in action:"
  echo "    1. Start a demo agent:  ./scripts/run-demo-agent.sh"
  echo "    2. Re-run this script"
  echo "    3. The agent's next check-in response will include:"
  echo '       {"immediateRecheckin": true}'
else
  echo "[trigger] using device $DEVICE_ID"

  # 3b. Admin token fires a device trigger
  if [ -n "${WEBHOOK_ID:-}" ]; then
    echo fail > "$WEBHOOK_MODE_FILE"
  fi
  api_call POST "/api/v1/devices/$DEVICE_ID/trigger-apply" \
    -H 'Content-Type: application/json' \
    -d '{"reason":"ci-feedback-loop-test"}'
  if [[ "$API_STATUS" =~ ^2 ]]; then
    pass "trigger/device-apply" "http=$API_STATUS"
  else
    fail "trigger/device-apply" "expected 2xx, got http=$API_STATUS body=$API_BODY"
  fi

  # 3c. deployment.trigger-scoped token can also fire the same trigger (idempotent upsert)
  if [ -n "$TRIGGER_TOKEN" ]; then
    AUTH_TOKEN=$TRIGGER_TOKEN
    api_call POST "/api/v1/devices/$DEVICE_ID/trigger-apply" \
      -H 'Content-Type: application/json' \
      -d '{"reason":"ci-scoped-token-test"}'
    AUTH_TOKEN=$ADMIN_TOKEN
    if [[ "$API_STATUS" =~ ^2 ]]; then
      pass "trigger/scoped-token-accepted" "deployment.trigger token http=$API_STATUS"
    else
      fail "trigger/scoped-token-accepted" "expected 2xx, got http=$API_STATUS body=$API_BODY"
    fi
  fi

  # 3d. device.read-only token must be rejected
  if [ -n "$READ_TOKEN" ]; then
    AUTH_TOKEN=$READ_TOKEN
    api_call POST "/api/v1/devices/$DEVICE_ID/trigger-apply" \
      -H 'Content-Type: application/json' \
      -d '{"reason":"should-be-rejected"}'
    AUTH_TOKEN=$ADMIN_TOKEN
    if [ "$API_STATUS" = "403" ]; then
      pass "trigger/wrong-scope-rejected" "device.read token correctly blocked http=403"
    else
      fail "trigger/wrong-scope-rejected" "expected 403, got http=$API_STATUS body=$API_BODY"
    fi
  fi

  # 3e. List groups — attempt a group trigger if any exist
  api_call GET /api/v1/groups
  GROUP_ID=$(python3 - <<'PY' "$API_BODY"
import json, sys
try:
    data = json.loads(sys.argv[1])
    groups = data if isinstance(data, list) else data.get("groups", data.get("items", []))
    for g in groups:
        print(g.get("id", "") or g.get("groupId", ""))
        break
except Exception:
    pass
PY
)
  if [ -n "$GROUP_ID" ]; then
    api_call POST "/api/v1/groups/$GROUP_ID/trigger-apply" \
      -H 'Content-Type: application/json' \
      -d '{"reason":"ci-group-trigger-test"}'
    if [[ "$API_STATUS" =~ ^2 ]]; then
      pass "trigger/group-apply" "group=$GROUP_ID http=$API_STATUS"
    else
      fail "trigger/group-apply" "expected 2xx, got http=$API_STATUS body=$API_BODY"
    fi
  else
    skip "trigger/group-apply" "no groups defined"
  fi

  echo ""
  echo "[trigger] Trigger is pending. On next device check-in the response will contain:"
  echo '  {"immediateRecheckin": true}'
  echo "  Watching for it — if a demo agent is running you should see it connect:"
  echo "    journalctl -u hardwareops-agent -f   (on the device)"
fi

echo ""

# ─── Section 3b: Webhook redelivery ───────────────────────────────────────────
echo "--- 3b. Webhook redelivery ---"

if [ -n "${WEBHOOK_ID:-}" ] && [ -n "${DEVICE_ID:-}" ]; then
  FAILED_DELIVERY_ID=""
  for _attempt in 1 2 3 4 5 6 7 8; do
    sleep 1
    api_call GET "/api/v1/webhooks/$WEBHOOK_ID/deliveries"
    FAILED_DELIVERY_ID=$(python3 - <<'PY' "$API_BODY"
import json, sys
try:
    data = json.loads(sys.argv[1])
    items = data if isinstance(data, list) else data.get("deliveries", data.get("items", []))
    for d in items:
        if d.get("status") == "failed":
            print(d.get("id", ""))
            break
except Exception:
    pass
PY
)
    [ -n "$FAILED_DELIVERY_ID" ] && break
  done

  if [ -z "$FAILED_DELIVERY_ID" ]; then
    skip "webhook/redelivery-failed-delivery" "no failed delivery recorded"
    skip "webhook/redelivery-success" "no failed delivery recorded"
  else
    pass "webhook/redelivery-failed-delivery" "failed delivery id=$FAILED_DELIVERY_ID"
    echo ok > "$WEBHOOK_MODE_FILE"
    api_call POST "/api/v1/webhooks/$WEBHOOK_ID/deliveries/$FAILED_DELIVERY_ID/redeliver"
    if [ "$API_STATUS" = "200" ]; then
      REDELIVERY_ID=$(json_field "$API_BODY" deliveryId)
      if [ -n "$REDELIVERY_ID" ] && [ "$REDELIVERY_ID" != "$FAILED_DELIVERY_ID" ]; then
        pass "webhook/redelivery-created-new-record" "new delivery id=$REDELIVERY_ID"
      else
        fail "webhook/redelivery-created-new-record" "expected fresh delivery id body=$API_BODY"
      fi
      if python3 - <<'PY' "$API_BODY"
import json, sys
d = json.loads(sys.argv[1])
sys.exit(0 if d.get("success") is True else 1)
PY
      then
        pass "webhook/redelivery-success" "body=$API_BODY"
      else
        fail "webhook/redelivery-success" "expected success=true body=$API_BODY"
      fi
    else
      fail "webhook/redelivery-success" "expected 200, got http=$API_STATUS body=$API_BODY"
    fi
  fi
else
  skip "webhook/redelivery-failed-delivery" "webhook or device unavailable"
  skip "webhook/redelivery-success" "webhook or device unavailable"
fi

echo ""

# ─── Section 4: Deployment status polling ─────────────────────────────────────
echo "--- 4. Deployment status polling ---"

# 4a. Need both a group and an artifact to test the status endpoint.
api_call GET /api/v1/groups
STATUS_GROUP_ID=$(python3 - <<'PY' "$API_BODY"
import json, sys
try:
    data = json.loads(sys.argv[1])
    groups = data if isinstance(data, list) else data.get("groups", data.get("items", []))
    for g in groups:
        gid = g.get("id", "") or g.get("groupId", "")
        if gid:
            print(gid)
            break
except Exception:
    pass
PY
)

api_call GET /api/v1/artifacts
STATUS_ARTIFACT_ID=$(python3 - <<'PY' "$API_BODY"
import json, sys
try:
    data = json.loads(sys.argv[1])
    arts = data if isinstance(data, list) else data.get("artifacts", data.get("items", []))
    for a in arts:
        aid = a.get("id", "") or a.get("artifactId", "")
        if aid:
            print(aid)
            break
except Exception:
    pass
PY
)

if [ -z "$STATUS_GROUP_ID" ] || [ -z "$STATUS_ARTIFACT_ID" ]; then
  skip "deploy-status/group-rollout" "need at least one group and one artifact — upload an artifact and create a group first"
else
  # 4b. GET /groups/{id}/deployment-status?artifactId=...
  api_call GET "/api/v1/groups/$STATUS_GROUP_ID/deployment-status?artifactId=$STATUS_ARTIFACT_ID"
  if [ "$API_STATUS" = "200" ]; then
    TOTAL=$(python3 - <<'PY' "$API_BODY"
import json, sys
d = json.loads(sys.argv[1])
print(d.get("total", "?"))
PY
)
    PENDING=$(python3 - <<'PY' "$API_BODY"
import json, sys
d = json.loads(sys.argv[1])
print(d.get("pending", "?"))
PY
)
    APPLIED=$(python3 - <<'PY' "$API_BODY"
import json, sys
d = json.loads(sys.argv[1])
print(d.get("applied", "?"))
PY
)
    COMPLETE=$(python3 - <<'PY' "$API_BODY"
import json, sys
d = json.loads(sys.argv[1])
print(d.get("complete", False))
PY
)
    pass "deploy-status/group-rollout" "total=$TOTAL pending=$PENDING applied=$APPLIED complete=$COMPLETE"
  else
    fail "deploy-status/group-rollout" "expected 200, got http=$API_STATUS body=$API_BODY"
  fi

  # 4c. Missing artifactId → 400
  api_call GET "/api/v1/groups/$STATUS_GROUP_ID/deployment-status"
  if [ "$API_STATUS" = "400" ]; then
    pass "deploy-status/missing-artifact-param" "http=400 as expected"
  else
    fail "deploy-status/missing-artifact-param" "expected 400, got http=$API_STATUS"
  fi

  # 4d. Non-existent group → 404
  api_call GET "/api/v1/groups/00000000-0000-0000-0000-000000000000/deployment-status?artifactId=$STATUS_ARTIFACT_ID"
  if [ "$API_STATUS" = "404" ]; then
    pass "deploy-status/unknown-group" "http=404 as expected"
  else
    fail "deploy-status/unknown-group" "expected 404, got http=$API_STATUS"
  fi

  # 4e. device.read-scoped token can also read deployment status
  if [ -n "$READ_TOKEN" ]; then
    SAVED_TOKEN=$AUTH_TOKEN
    AUTH_TOKEN=$READ_TOKEN
    api_call GET "/api/v1/groups/$STATUS_GROUP_ID/deployment-status?artifactId=$STATUS_ARTIFACT_ID"
    AUTH_TOKEN=$SAVED_TOKEN
    if [ "$API_STATUS" = "200" ]; then
      pass "deploy-status/read-token-allowed" "device.read token http=200"
    else
      fail "deploy-status/read-token-allowed" "expected 200, got http=$API_STATUS"
    fi
  fi
fi

if [ -n "${WEBHOOK_ID:-}" ]; then
  api_call DELETE "/api/v1/webhooks/$WEBHOOK_ID"
  if [[ "$API_STATUS" =~ ^2 ]]; then
    pass "webhook/cleanup" "deleted webhook $WEBHOOK_ID"
  else
    fail "webhook/cleanup" "http=$API_STATUS body=$API_BODY"
  fi
fi
