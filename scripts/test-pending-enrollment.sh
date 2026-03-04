#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CONTROL_PLANE_CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
OUTPUT_DIR=${OUTPUT_DIR:-$(mktemp -d /tmp/hardwareops-pending-enroll.XXXXXX)}
PROFILE_NAME=${PROFILE_NAME:-local-pending-enroll}
PROFILE_EXPIRES_IN_SEC=${PROFILE_EXPIRES_IN_SEC:-86400}
PROFILE_MAX_USES=${PROFILE_MAX_USES:-1}
PROFILE_REQUIRE_APPROVAL=${PROFILE_REQUIRE_APPROVAL:-1}
ALLOW_UNSIGNED_HARDWARE_IDENTITY=${ALLOW_UNSIGNED_HARDWARE_IDENTITY:-0}
PROFILE_DEFAULT_LABELS_JSON=${PROFILE_DEFAULT_LABELS_JSON:-}
GROUP_NAME=${GROUP_NAME:-pending-enroll-e2e}
GROUP_SELECTOR_JSON=${GROUP_SELECTOR_JSON:-}
GROUP_DESIRED_VERSION=${GROUP_DESIRED_VERSION:-}
GROUP_CHECKIN_INTERVAL_SEC=${GROUP_CHECKIN_INTERVAL_SEC:-0}
AGENT_VERSION=${AGENT_VERSION:-pending-test}
HARDWARE_ID=${HARDWARE_ID:-pending-test-$(python3 - <<'PY'
import uuid
print(uuid.uuid4().hex)
PY
)}
CSR_SUBJECT=${CSR_SUBJECT:-/CN=hardwareops-pending-device}
POLL_FOR_CLAIM=${POLL_FOR_CLAIM:-1}
CLAIM_POLL_INTERVAL_SEC=${CLAIM_POLL_INTERVAL_SEC:-5}
AUTO_APPROVE_API=${AUTO_APPROVE_API:-0}
CHECKIN_AFTER_CLAIM=${CHECKIN_AFTER_CLAIM:-0}
CHECKIN_CURRENT_VERSION=${CHECKIN_CURRENT_VERSION:-bootstrap}
CHECKIN_CURRENT_CONFIG_REV=${CHECKIN_CURRENT_CONFIG_REV:-}
AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}

if [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA cert not found at $CA_CERT_PATH" >&2
  echo "Start the control-plane first: ENABLE_TLS=1 ./scripts/run-control-plane.sh" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

curl_opts=(--cacert "$CA_CERT_PATH")
if [ -n "${CURL_RESOLVE_HOSTS:-}" ]; then
  IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
  for entry in "${resolve_entries[@]}"; do
    entry=$(echo "$entry" | xargs)
    [ -n "$entry" ] && curl_opts+=(--resolve "$entry")
  done
fi

if ! curl -fsS "${curl_opts[@]}" "$BASE_URL/healthz" >/dev/null; then
  echo "Control-plane not reachable at $BASE_URL" >&2
  exit 1
fi

auth_status_json=$(curl -fsS "${curl_opts[@]}" "$BASE_URL/api/v1/auth/status" || true)
AUTH_ENABLED=$(python3 - <<'PY' "$auth_status_json"
import json, sys
raw = sys.argv[1] if len(sys.argv) > 1 else ""
try:
    payload = json.loads(raw) if raw else {}
except Exception:
    print("0")
    raise SystemExit(0)
print("1" if payload.get("enabled") else "0")
PY
)

if [ "$AUTH_ENABLED" = "1" ] && [ -z "$AUTH_TOKEN" ]; then
  if [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; then
    cat >&2 <<EOF
Auth is enabled on the control-plane.
Set either:
  AUTH_TOKEN=<jwt>
or:
  AUTH_EMAIL=<user email> AUTH_PASSWORD=<password>
EOF
    exit 1
  fi
  login_payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
  login_resp=$(curl -sS "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "$login_payload" \
    -w $'\n%{http_code}')
  login_status=${login_resp##*$'\n'}
  login_json=${login_resp%$'\n'*}
  if [ "$login_status" != "200" ]; then
    echo "Auth login failed (status=$login_status)." >&2
    [ -n "$login_json" ] && echo "$login_json" >&2
    exit 1
  fi
  AUTH_TOKEN=$(python3 - <<'PY' "$login_json"
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
  [ -n "$AUTH_TOKEN" ] || { echo "Auth login succeeded but token was empty." >&2; exit 1; }
fi

auth_args=()
if [ -n "$AUTH_TOKEN" ]; then
  auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

DEVICE_KEY_PATH="$OUTPUT_DIR/device.key"
DEVICE_CSR_PATH="$OUTPUT_DIR/device.csr"
DEVICE_CERT_PATH="$OUTPUT_DIR/device.crt"
DEVICE_CA_PATH="$OUTPUT_DIR/ca.crt"
DEVICE_ID_PATH="$OUTPUT_DIR/device-id"
REQUEST_JSON_PATH="$OUTPUT_DIR/request.json"
CLAIM_JSON_PATH="$OUTPUT_DIR/claim.json"
PROFILE_JSON_PATH="$OUTPUT_DIR/profile.json"

if [ ! -s "$DEVICE_KEY_PATH" ] || [ ! -s "$DEVICE_CSR_PATH" ]; then
  openssl req -new -newkey rsa:2048 -nodes \
    -keyout "$DEVICE_KEY_PATH" \
    -out "$DEVICE_CSR_PATH" \
    -subj "$CSR_SUBJECT" >/dev/null 2>&1
fi

profile_payload=$(python3 - <<'PY' \
  "$PROFILE_NAME" \
  "$PROFILE_EXPIRES_IN_SEC" \
  "$PROFILE_MAX_USES" \
  "$PROFILE_REQUIRE_APPROVAL" \
  "$ALLOW_UNSIGNED_HARDWARE_IDENTITY" \
  "$PROFILE_DEFAULT_LABELS_JSON"
import json, sys
payload = {
  "name": sys.argv[1],
  "expiresInSec": int(sys.argv[2]),
  "maxUses": int(sys.argv[3]),
  "requireApproval": sys.argv[4] == "1",
  "allowUnsignedHardwareIdentity": sys.argv[5] == "1",
}
labels_raw = sys.argv[6].strip()
if labels_raw:
  labels = json.loads(labels_raw)
  if not isinstance(labels, dict):
    raise SystemExit("PROFILE_DEFAULT_LABELS_JSON must be a JSON object")
  payload["defaultLabels"] = labels
print(json.dumps(payload))
PY
)

profile_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" \
  -X POST "$BASE_URL/api/v1/enrollment-profiles" \
  -H "Content-Type: application/json" \
  -d "$profile_payload" \
  -w $'\n%{http_code}')
profile_status=${profile_resp##*$'\n'}
profile_json=${profile_resp%$'\n'*}
if [ "$profile_status" != "201" ]; then
  echo "Create enrollment profile failed (status=$profile_status)." >&2
  [ -n "$profile_json" ] && echo "$profile_json" >&2
  exit 1
fi
printf '%s\n' "$profile_json" >"$PROFILE_JSON_PATH"

PROFILE_ID=$(python3 - <<'PY' "$profile_json"
import json, sys
print(json.loads(sys.argv[1]).get("profileId", ""))
PY
)
PROFILE_TOKEN=$(python3 - <<'PY' "$profile_json"
import json, sys
print(json.loads(sys.argv[1]).get("bootstrapToken", ""))
PY
)

if [ -z "$GROUP_SELECTOR_JSON" ] && [ -n "$PROFILE_DEFAULT_LABELS_JSON" ]; then
  GROUP_SELECTOR_JSON="$PROFILE_DEFAULT_LABELS_JSON"
fi

GROUP_ID=""
if [ -n "$GROUP_SELECTOR_JSON" ] || [ -n "$GROUP_DESIRED_VERSION" ] || [ "$GROUP_CHECKIN_INTERVAL_SEC" -gt 0 ]; then
  GROUP_ID=$(python3 - <<'PY'
import uuid
print(uuid.uuid4())
PY
)
  group_payload=$(python3 - <<'PY' "$GROUP_NAME" "$GROUP_SELECTOR_JSON"
import json, sys
selector_raw = sys.argv[2].strip() or "{}"
selector = json.loads(selector_raw)
if not isinstance(selector, dict):
  raise SystemExit("GROUP_SELECTOR_JSON must be a JSON object")
print(json.dumps({"name": sys.argv[1], "selector": selector}))
PY
)
  group_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" \
    -X PUT "$BASE_URL/api/v1/groups/$GROUP_ID" \
    -H "Content-Type: application/json" \
    -d "$group_payload" \
    -w $'\n%{http_code}')
  group_status=${group_resp##*$'\n'}
  group_json=${group_resp%$'\n'*}
  if [ "$group_status" != "200" ]; then
    echo "Create group failed (status=$group_status)." >&2
    [ -n "$group_json" ] && echo "$group_json" >&2
    exit 1
  fi

  if [ -n "$GROUP_DESIRED_VERSION" ] || [ "$GROUP_CHECKIN_INTERVAL_SEC" -gt 0 ]; then
    desired_payload=$(python3 - <<'PY' "$GROUP_DESIRED_VERSION" "$GROUP_CHECKIN_INTERVAL_SEC"
import json, sys
payload = {}
if sys.argv[1].strip():
  payload["desiredVersion"] = sys.argv[1].strip()
interval = int(sys.argv[2])
if interval > 0:
  payload["checkinIntervalSec"] = interval
print(json.dumps(payload))
PY
)
    desired_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" \
      -X PUT "$BASE_URL/api/v1/desired-state/groups/$GROUP_ID" \
      -H "Content-Type: application/json" \
      -d "$desired_payload" \
      -w $'\n%{http_code}')
    desired_status=${desired_resp##*$'\n'}
    desired_json=${desired_resp%$'\n'*}
    if [ "$desired_status" != "200" ]; then
      echo "Set group desired state failed (status=$desired_status)." >&2
      [ -n "$desired_json" ] && echo "$desired_json" >&2
      exit 1
    fi
  fi
  echo "Created test group: $GROUP_ID"
fi

request_payload=$(python3 - <<'PY' "$PROFILE_TOKEN" "$DEVICE_CSR_PATH" "$AGENT_VERSION" "$HARDWARE_ID"
import json, sys
print(json.dumps({
  "profileToken": sys.argv[1],
  "csr": open(sys.argv[2]).read(),
  "agentVersion": sys.argv[3],
  "capabilities": {
    "hw": {
      "identity": {
        "id": sys.argv[4],
        "source": "pending-test-script"
      }
    }
  },
  "metadata": {
    "hostname": "local-pending-test"
  }
}))
PY
)

request_resp=$(curl -sS "${curl_opts[@]}" \
  -X POST "$BASE_URL/api/v1/pending-enrollments/request" \
  -H "Content-Type: application/json" \
  -d "$request_payload" \
  -w $'\n%{http_code}')
request_status=${request_resp##*$'\n'}
request_json=${request_resp%$'\n'*}
if [ "$request_status" != "202" ]; then
  echo "Pending enrollment request failed (status=$request_status)." >&2
  [ -n "$request_json" ] && echo "$request_json" >&2
  exit 1
fi
printf '%s\n' "$request_json" >"$REQUEST_JSON_PATH"

REQUEST_ID=$(python3 - <<'PY' "$request_json"
import json, sys
print(json.loads(sys.argv[1]).get("requestId", ""))
PY
)
CLAIM_TOKEN=$(python3 - <<'PY' "$request_json"
import json, sys
print(json.loads(sys.argv[1]).get("claimToken", ""))
PY
)

echo "Enrollment profile created: $PROFILE_ID"
echo "Pending enrollment request: $REQUEST_ID"
echo "Artifacts written under: $OUTPUT_DIR"
echo

if [ "$AUTO_APPROVE_API" = "1" ]; then
  approve_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" \
    -X POST "$BASE_URL/api/v1/pending-enrollments/$REQUEST_ID/approve" \
    -H "Content-Type: application/json" \
    -d '{}' \
    -w $'\n%{http_code}')
  approve_status=${approve_resp##*$'\n'}
  approve_json=${approve_resp%$'\n'*}
  if [ "$approve_status" != "200" ]; then
    echo "Approve failed (status=$approve_status)." >&2
    [ -n "$approve_json" ] && echo "$approve_json" >&2
    exit 1
  fi
  echo "Approved via API."
else
  cat <<EOF
Approve this request in the UI:
  1. Start the UI: cd ui && VITE_API_BASE_URL=$BASE_URL VITE_SIMULATE_PROD=1 npm run dev
  2. Open http://localhost:5173
  3. Go to Security -> Pending enrollments
  4. Approve request: $REQUEST_ID

Or approve by API:
  curl --cacert $CA_CERT_PATH ${AUTH_TOKEN:+-H "Authorization: Bearer $AUTH_TOKEN"} \\
    -X POST $BASE_URL/api/v1/pending-enrollments/$REQUEST_ID/approve \\
    -H 'Content-Type: application/json' -d '{}'
EOF
fi

if [ "$POLL_FOR_CLAIM" != "1" ]; then
  exit 0
fi

while true; do
  claim_payload=$(python3 - <<'PY' "$REQUEST_ID" "$CLAIM_TOKEN"
import json, sys
print(json.dumps({"requestId": sys.argv[1], "claimToken": sys.argv[2]}))
PY
)
  claim_resp=$(curl -sS "${curl_opts[@]}" \
    -X POST "$BASE_URL/api/v1/pending-enrollments/claim" \
    -H "Content-Type: application/json" \
    -d "$claim_payload" \
    -w $'\n%{http_code}')
  claim_status=${claim_resp##*$'\n'}
  claim_json=${claim_resp%$'\n'*}
  printf '%s\n' "$claim_json" >"$CLAIM_JSON_PATH"

  case "$claim_status" in
    200)
      python3 - <<'PY' "$claim_json" "$DEVICE_CERT_PATH" "$DEVICE_CA_PATH" "$DEVICE_ID_PATH"
import json, sys
payload = json.loads(sys.argv[1])
open(sys.argv[2], "w").write(payload.get("certPem", ""))
open(sys.argv[3], "w").write(payload.get("caCertPem", ""))
open(sys.argv[4], "w").write(payload.get("deviceId", ""))
PY
      echo "Certificate issued."
      echo "Device ID: $(cat "$DEVICE_ID_PATH")"
      echo "Device cert: $DEVICE_CERT_PATH"
      echo "CA cert: $DEVICE_CA_PATH"
      if [ "$CHECKIN_AFTER_CLAIM" = "1" ]; then
        checkin_payload=$(python3 - <<'PY' "$DEVICE_ID_PATH" "$AGENT_VERSION" "$CHECKIN_CURRENT_VERSION" "$CHECKIN_CURRENT_CONFIG_REV"
import json, sys
payload = {
  "deviceId": open(sys.argv[1]).read().strip(),
  "agentVersion": sys.argv[2],
  "current": {
    "softwareVersion": sys.argv[3],
    "configRev": sys.argv[4],
  },
}
print(json.dumps(payload))
PY
)
        device_curl_opts=(--cacert "$DEVICE_CA_PATH" --cert "$DEVICE_CERT_PATH" --key "$DEVICE_KEY_PATH")
        if [ -n "${CURL_RESOLVE_HOSTS:-}" ]; then
          IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
          for entry in "${resolve_entries[@]}"; do
            entry=$(echo "$entry" | xargs)
            [ -n "$entry" ] && device_curl_opts+=(--resolve "$entry")
          done
        fi
        checkin_resp=$(curl -sS "${device_curl_opts[@]}" \
          -X POST "$BASE_URL/api/v1/devices/checkin" \
          -H "Content-Type: application/json" \
          -d "$checkin_payload" \
          -w $'\n%{http_code}')
        checkin_status=${checkin_resp##*$'\n'}
        checkin_json=${checkin_resp%$'\n'*}
        if [ "$checkin_status" != "200" ]; then
          echo "Post-claim checkin failed (status=$checkin_status)." >&2
          [ -n "$checkin_json" ] && echo "$checkin_json" >&2
          exit 1
        fi
        if [ -n "$GROUP_DESIRED_VERSION" ]; then
          python3 - <<'PY' "$checkin_json" "$GROUP_DESIRED_VERSION"
import json, sys
payload = json.loads(sys.argv[1])
desired = payload.get("desired") or {}
if desired.get("softwareVersion") != sys.argv[2]:
  raise SystemExit(f"expected desired.softwareVersion={sys.argv[2]!r}, got {desired.get('softwareVersion')!r}")
if desired.get("source") != "group":
  raise SystemExit(f"expected desired.source='group', got {desired.get('source')!r}")
PY
        fi
        echo "Post-claim checkin succeeded."
      fi
      exit 0
      ;;
    202)
      status=$(python3 - <<'PY' "$claim_json"
import json, sys
print(json.loads(sys.argv[1]).get("status", "pending"))
PY
)
      echo "Claim status: $status; waiting ${CLAIM_POLL_INTERVAL_SEC}s"
      sleep "$CLAIM_POLL_INTERVAL_SEC"
      ;;
    410)
      echo "Claim ended in terminal state." >&2
      echo "$claim_json" >&2
      exit 1
      ;;
    *)
      echo "Claim failed (status=$claim_status)." >&2
      [ -n "$claim_json" ] && echo "$claim_json" >&2
      exit 1
      ;;
  esac
done
