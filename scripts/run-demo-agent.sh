#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
AGENT_BASE_URL=${AGENT_BASE_URL:-}
CERT_DIR_BASE=${CERT_DIR:-/tmp/hardwareops-demo/certs}
DATA_DIR_BASE=${DATA_DIR:-/tmp/hardwareops-demo/data}
IMAGE_NAME=${IMAGE_NAME:-hardwareops-agent-demo}
CONTAINER_NAME=${CONTAINER_NAME:-hardwareops-demo-agent}
DEMO_HTTP_PORT=${DEMO_HTTP_PORT:-8081}
DEMO_AGENT_PORT_STEP=${DEMO_AGENT_PORT_STEP:-1000}
DEMO_COMPONENT_PORT_STEP=${DEMO_COMPONENT_PORT_STEP:-10}
DEMO_COMPONENTS=${DEMO_COMPONENTS:-agent_bundle}
NO_CACHE=${NO_CACHE:-0}
DEMO_COUNT=${DEMO_COUNT:-1}
DEMO_RESET=${DEMO_RESET:-0}
CERTS_RW=${CERTS_RW:-1}
ALLOW_UNSUPPORTED_APPLY=${ALLOW_UNSUPPORTED_APPLY:-1}
SIGN_ARTIFACTS=${SIGN_ARTIFACTS:-1}
REQUIRE_ARTIFACT_SIGNATURE=${REQUIRE_ARTIFACT_SIGNATURE:-$SIGN_ARTIFACTS}
UPLOAD_AGENT_BUNDLE=${UPLOAD_AGENT_BUNDLE:-1}
AGENT_ARTIFACT_NAME=${AGENT_ARTIFACT_NAME:-agent}
AGENT_ARTIFACT_VERSION=${AGENT_ARTIFACT_VERSION:-0.1.0}
ENROLLMENT_MODE=${ENROLLMENT_MODE:-legacy}
PENDING_ENROLL_PROFILE_NAME=${PENDING_ENROLL_PROFILE_NAME:-demo-agent}
PENDING_ENROLL_PROFILE_EXPIRES_IN_SEC=${PENDING_ENROLL_PROFILE_EXPIRES_IN_SEC:-86400}
PENDING_ENROLL_PROFILE_MAX_USES=${PENDING_ENROLL_PROFILE_MAX_USES:-1}
PENDING_ENROLL_ALLOW_UNSIGNED_HARDWARE_IDENTITY=${PENDING_ENROLL_ALLOW_UNSIGNED_HARDWARE_IDENTITY:-0}
PENDING_ENROLL_PROFILE_DEFAULT_LABELS_JSON=${PENDING_ENROLL_PROFILE_DEFAULT_LABELS_JSON:-}
PENDING_ENROLL_AUTO_APPROVE=${PENDING_ENROLL_AUTO_APPROVE:-0}
PENDING_ENROLL_CLAIM_POLL_INTERVAL_SEC=${PENDING_ENROLL_CLAIM_POLL_INTERVAL_SEC:-5}
PENDING_ENROLL_CLAIM_TIMEOUT_SEC=${PENDING_ENROLL_CLAIM_TIMEOUT_SEC:-900}
AUTH_TOKEN=${AUTH_TOKEN:-}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}

HOST_URL="$BASE_URL"
if [ -z "$AGENT_BASE_URL" ]; then
  if [[ "$BASE_URL" =~ ^https?://localhost(:|/|$) ]] || [[ "$BASE_URL" =~ ^https?://127\.0\.0\.1(:|/|$) ]]; then
    AGENT_BASE_URL="$BASE_URL"
  else
    scheme=${BASE_URL%%://*}
    rest=${BASE_URL#*://}
    hostport=${rest%%/*}
    pathpart=""
    if [[ "$rest" == */* ]]; then
      pathpart=/${rest#*/}
    fi
    AGENT_BASE_URL="${scheme}://agent.${hostport}${pathpart}"
  fi
fi
AGENT_HOST_URL="$AGENT_BASE_URL"

if [[ "$BASE_URL" == http:* ]]; then
  echo "Control-plane must be HTTPS for mTLS. Start with ENABLE_TLS=1 ./scripts/run-control-plane.sh" >&2
  exit 1
fi

CA_CERT_PATH=${CONTROL_PLANE_CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
if [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA cert not found at $CA_CERT_PATH. Run ./scripts/run-control-plane.sh first." >&2
  exit 1
fi

# Ensure signing keys are available for demo verification.
source "$BASE_DIR/scripts/ensure-signing-key.sh"

curl_opts=(--cacert "$CA_CERT_PATH")
if [ -n "${CURL_RESOLVE_HOSTS:-}" ]; then
  IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
  for entry in "${resolve_entries[@]}"; do
    entry=$(echo "$entry" | xargs)
    if [ -n "$entry" ]; then
      curl_opts+=(--resolve "$entry")
    fi
  done
fi
if ! curl -s "${curl_opts[@]}" "$HOST_URL/healthz" >/dev/null; then
  echo "Control-plane not reachable at $HOST_URL" >&2
  exit 1
fi

auth_status_json=$(curl -sS "${curl_opts[@]}" "$HOST_URL/api/v1/auth/status" || true)
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
  if [ -n "$AUTH_EMAIL" ] && [ -n "$AUTH_PASSWORD" ]; then
    LOGIN_PAYLOAD=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
    login_resp=$(curl -sS "${curl_opts[@]}" -X POST "$HOST_URL/api/v1/auth/login" \
      -H "Content-Type: application/json" \
      -d "$LOGIN_PAYLOAD" \
      -w $'\n%{http_code}')
    login_status=${login_resp##*$'\n'}
    login_json=${login_resp%$'\n'*}
    if [ "$login_status" != "200" ]; then
      echo "Auth login failed (status=$login_status)." >&2
      if [ -n "$login_json" ]; then
        echo "$login_json" >&2
      fi
      exit 1
    fi
    AUTH_TOKEN=$(python3 - <<'PY' "$login_json"
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
    if [ -z "$AUTH_TOKEN" ]; then
      echo "Auth login succeeded but token was empty." >&2
      exit 1
    fi
  else
    cat >&2 <<EOF
Auth is enabled on the control-plane.
Set either:
  AUTH_TOKEN=<jwt>
or:
  AUTH_EMAIL=<user email> AUTH_PASSWORD=<password>
EOF
    exit 1
  fi
fi

auth_args=()
if [ -n "$AUTH_TOKEN" ]; then
  auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

if [ "$UPLOAD_AGENT_BUNDLE" = "1" ]; then
  AGENT_BUNDLE_DIR=${AGENT_BUNDLE_DIR:-"${TMPDIR:-/tmp}/hardwareops-demo/agent-bundle"}
  mkdir -p "$AGENT_BUNDLE_DIR/files"
  cat > "$AGENT_BUNDLE_DIR/files/readme.txt" <<EOF
HardwareOps agent bundle demo
version=${AGENT_ARTIFACT_VERSION}
EOF
  cat > "$AGENT_BUNDLE_DIR/files/preapply.sh" <<'EOF'
#!/usr/bin/env sh
set -eu
mkdir -p files
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "agent preapply ok ${ts}" > files/preapply.txt
echo "${ts}" > files/last_applied.txt
echo "${ts} artifact=${HWOPS_ARTIFACT_ID:-} version=${HWOPS_ARTIFACT_VERSION:-}" >> files/apply.log
pid_file="${HWOPS_ARTIFACT_ROOT}/heartbeat.pid"
log_file="${HWOPS_ARTIFACT_DIR}/files/heartbeat.log"
if [ -f "$pid_file" ]; then
  old_pid=$(cat "$pid_file" 2>/dev/null || true)
  if [ -n "${old_pid:-}" ] && kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
fi
( while true; do
    beat_ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "$beat_ts" >> "$log_file"
    sleep 10
  done ) >/dev/null 2>&1 &
echo $! > "$pid_file"
EOF
  chmod 0755 "$AGENT_BUNDLE_DIR/files/preapply.sh"
  cat > "$AGENT_BUNDLE_DIR/files/plan.yaml" <<'EOF'
version: "v1"
steps:
  - id: preapply
    type: script.preApply
    onFail: abort
    params:
      command: files/preapply.sh
      timeoutSec: 60
EOF
  ARTIFACT_NAME="$AGENT_ARTIFACT_NAME" \
  ARTIFACT_VERSION="$AGENT_ARTIFACT_VERSION" \
  ARTIFACT_TYPE="agent_bundle" \
  INPUT_DIR="$AGENT_BUNDLE_DIR" \
  BASE_URL="$BASE_URL" \
  CA_CERT_PATH="$CA_CERT_PATH" \
  AUTH_TOKEN="$AUTH_TOKEN" \
  SIGNING_KEY="$SIGNING_KEY" \
  SIGNING_KEY_ID="$SIGNING_KEY_ID" \
  "$BASE_DIR/scripts/pack-upload-artifact.sh" >/dev/null
  echo "Uploaded agent bundle artifact ${AGENT_ARTIFACT_NAME}:${AGENT_ARTIFACT_VERSION}"
fi

AGENT_URL=${AGENT_URL:-$AGENT_HOST_URL}
agent_url_tail=${AGENT_URL#*://}
agent_url_hostport=${agent_url_tail%%/*}
agent_url_host=${agent_url_hostport%%:*}
if [ "$agent_url_host" = "localhost" ] || [ "$agent_url_host" = "127.0.0.1" ]; then
  AGENT_URL=${AGENT_URL/#*:\/\/$agent_url_host/https:\/\/host.docker.internal}
  agent_url_host="host.docker.internal"
fi
extra_agent_host_arg=()
if [[ "$agent_url_host" == *.localhost ]] || [[ "$agent_url_host" == *.local ]]; then
  extra_agent_host_arg=(--add-host "$agent_url_host:host-gateway")
fi

if [[ "$ENROLLMENT_MODE" != "legacy" && "$ENROLLMENT_MODE" != "pending" ]]; then
  echo "ENROLLMENT_MODE must be one of: legacy, pending" >&2
  exit 1
fi

build_args=()
if [ "$NO_CACHE" = "1" ]; then
  build_args+=(--no-cache)
fi
docker build "${build_args[@]}" -f "$BASE_DIR/agent/Dockerfile.demo" -t "$IMAGE_NAME" "$BASE_DIR"

if ! [[ "$DEMO_COUNT" =~ ^[0-9]+$ ]] || [ "$DEMO_COUNT" -lt 1 ]; then
  echo "DEMO_COUNT must be a positive integer." >&2
  exit 1
fi

if [ "$DEMO_RESET" = "1" ]; then
  # Remove any stale demo-agent containers from prior runs so DEMO_COUNT is authoritative.
  while IFS= read -r existing_name; do
    [ -z "$existing_name" ] && continue
    docker rm -f "$existing_name" >/dev/null || true
  done < <(docker ps -a --format '{{.Names}}' | grep -E "^${CONTAINER_NAME}(-[0-9]+)?$" || true)
fi

for i in $(seq 1 "$DEMO_COUNT"); do
  suffix=""
  if [ "$DEMO_COUNT" -gt 1 ] && [ "$i" -gt 1 ]; then
    suffix="-$i"
  fi

  CERT_DIR="$CERT_DIR_BASE$suffix"
  DATA_DIR="$DATA_DIR_BASE$suffix"
  DEMO_PORT=$((DEMO_HTTP_PORT + (i - 1) * DEMO_AGENT_PORT_STEP))
  NAME="$CONTAINER_NAME$suffix"

  if docker ps -a --format '{{.Names}}' | grep -q "^$NAME$"; then
    if [ "$DEMO_RESET" = "1" ]; then
      docker rm -f "$NAME" >/dev/null
    else
      echo "Container $NAME already exists. Remove it with: docker rm -f $NAME (or set DEMO_RESET=1)" >&2
      exit 1
    fi
  fi

  if [ "$DEMO_RESET" = "1" ]; then
    rm -rf "$CERT_DIR" "$DATA_DIR" 2>/dev/null || true
    # If prior runs wrote root-owned files, clear via a helper container.
    if [ -e "$CERT_DIR" ] || [ -e "$DATA_DIR" ]; then
      mkdir -p "$CERT_DIR" "$DATA_DIR"
      docker run --rm \
        -v "$CERT_DIR:/work/certs" \
        -v "$DATA_DIR:/work/data" \
        alpine:3.19 sh -c 'rm -rf /work/certs/* /work/data/* && chmod -R 0777 /work/certs /work/data' >/dev/null 2>&1 || true
      rm -rf "$CERT_DIR" "$DATA_DIR" 2>/dev/null || true
    fi
  fi

  mkdir -p "$CERT_DIR" "$DATA_DIR"
  # Ensure the container user (uid 10001) can write to /data when bind-mounted.
  chmod 0777 "$DATA_DIR"

  DEVICE_KEY_PATH="$CERT_DIR/device.key"
  DEVICE_CSR_PATH="$CERT_DIR/device.csr"
  DEVICE_CERT_PATH="$CERT_DIR/device.crt"
  DEVICE_CA_PATH="$CERT_DIR/issued-ca.crt"
  DEVICE_ID_PATH="$DATA_DIR/device-id"
  HARDWARE_ID_PATH="$DATA_DIR/hardware-id"
  if [ ! -s "$HARDWARE_ID_PATH" ]; then
    python3 - <<'PY' "$HARDWARE_ID_PATH"
import pathlib, sys, uuid
path = pathlib.Path(sys.argv[1])
path.parent.mkdir(parents=True, exist_ok=True)
path.write_text(uuid.uuid4().hex)
PY
  fi
  HARDWARE_ID=$(cat "$HARDWARE_ID_PATH")
  HARDWARE_ID_HASH=$(python3 - <<'PY' "$HARDWARE_ID" "${HARDWARE_IDENTITY_SALT:-}"
import hashlib, sys
raw = (sys.argv[1] or '').strip()
salt = (sys.argv[2] or '').strip()
payload = raw if not salt else f"{raw}|{salt}"
print(hashlib.sha256(payload.encode()).hexdigest())
PY
)

  if [ ! -s "$DEVICE_CERT_PATH" ] || [ ! -s "$DEVICE_KEY_PATH" ]; then
    if [ ! -s "$DEVICE_CSR_PATH" ] || [ ! -s "$DEVICE_KEY_PATH" ]; then
      openssl req -newkey rsa:2048 -nodes \
        -keyout "$DEVICE_KEY_PATH" -out "$DEVICE_CSR_PATH" \
        -subj "/CN=hardwareops-device"
    fi
    if [ "$ENROLLMENT_MODE" = "pending" ]; then
      PROFILE_JSON_PATH="$DATA_DIR/pending-profile.json"
      REQUEST_JSON_PATH="$DATA_DIR/pending-request.json"
      CLAIM_JSON_PATH="$DATA_DIR/pending-claim.json"
      profile_name="${PENDING_ENROLL_PROFILE_NAME}-${NAME}"

      profile_payload=$(python3 - \
        "$profile_name" \
        "$PENDING_ENROLL_PROFILE_EXPIRES_IN_SEC" \
        "$PENDING_ENROLL_PROFILE_MAX_USES" \
        "$PENDING_ENROLL_ALLOW_UNSIGNED_HARDWARE_IDENTITY" \
        "$PENDING_ENROLL_PROFILE_DEFAULT_LABELS_JSON" <<'PY'
import json, sys
payload = {
  "name": sys.argv[1],
  "expiresInSec": int(sys.argv[2]),
  "maxUses": int(sys.argv[3]),
  "requireApproval": True,
  "allowUnsignedHardwareIdentity": sys.argv[4] == "1",
}
labels_raw = sys.argv[5].strip()
if labels_raw:
  labels = json.loads(labels_raw)
  if not isinstance(labels, dict):
    raise SystemExit("PENDING_ENROLL_PROFILE_DEFAULT_LABELS_JSON must be a JSON object")
  payload["defaultLabels"] = labels
print(json.dumps(payload))
PY
)

      profile_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" -X POST "$HOST_URL/api/v1/enrollment-profiles" \
        -H "Content-Type: application/json" \
        -d "$profile_payload" \
        -w $'\n%{http_code}')
      profile_status=${profile_resp##*$'\n'}
      PROFILE_JSON=${profile_resp%$'\n'*}
      if [ -z "$PROFILE_JSON" ] || [ "$profile_status" != "201" ]; then
        echo "Failed to create enrollment profile (status=$profile_status)." >&2
        [ -n "$PROFILE_JSON" ] && echo "$PROFILE_JSON" >&2
        exit 1
      fi
      printf '%s\n' "$PROFILE_JSON" >"$PROFILE_JSON_PATH"

      PROFILE_TOKEN=$(python3 - <<'PY' "$PROFILE_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("bootstrapToken", ""))
PY
)
      if [ -z "$PROFILE_TOKEN" ]; then
        echo "Enrollment profile created without bootstrap token." >&2
        exit 1
      fi

      REQUEST_PAYLOAD=$(python3 - <<'PY' "$PROFILE_TOKEN" "$DEVICE_CSR_PATH" "$HARDWARE_ID_HASH"
import json, sys
hardware = sys.argv[3].strip()
print(json.dumps({
  "profileToken": sys.argv[1],
  "csr": open(sys.argv[2]).read(),
  "agentVersion": "demo-agent",
  "capabilities": {
    "hw": {"identity": {"id": hardware, "source": "demo-agent-name"}}
  },
  "metadata": {
    "containerName": None
  }
}))
PY
)

      REQUEST_PAYLOAD=$(python3 - <<'PY' "$REQUEST_PAYLOAD" "$NAME"
import json, sys
payload = json.loads(sys.argv[1])
payload["metadata"]["containerName"] = sys.argv[2]
print(json.dumps(payload))
PY
)

      request_resp=$(curl -sS "${curl_opts[@]}" -X POST "$AGENT_HOST_URL/api/v1/pending-enrollments/request" \
        -H "Content-Type: application/json" \
        -d "$REQUEST_PAYLOAD" \
        -w $'\n%{http_code}')
      request_status=${request_resp##*$'\n'}
      REQUEST_JSON=${request_resp%$'\n'*}
      if [ -z "$REQUEST_JSON" ] || [ "$request_status" != "202" ]; then
        echo "Pending enrollment request failed (status=$request_status)." >&2
        [ -n "$REQUEST_JSON" ] && echo "$REQUEST_JSON" >&2
        exit 1
      fi
      printf '%s\n' "$REQUEST_JSON" >"$REQUEST_JSON_PATH"

      REQUEST_ID=$(python3 - <<'PY' "$REQUEST_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("requestId", ""))
PY
)
      CLAIM_TOKEN=$(python3 - <<'PY' "$REQUEST_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("claimToken", ""))
PY
)
      if [ -z "$REQUEST_ID" ] || [ -z "$CLAIM_TOKEN" ]; then
        echo "Pending enrollment response missing requestId or claimToken." >&2
        exit 1
      fi

      echo "Pending enrollment created for $NAME."
      echo "  requestId: $REQUEST_ID"
      echo "  queue file: $REQUEST_JSON_PATH"

      if [ "$PENDING_ENROLL_AUTO_APPROVE" = "1" ]; then
        approve_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" -X POST "$HOST_URL/api/v1/pending-enrollments/$REQUEST_ID/approve" \
          -H "Content-Type: application/json" \
          -d '{}' \
          -w $'\n%{http_code}')
        approve_status=${approve_resp##*$'\n'}
        approve_json=${approve_resp%$'\n'*}
        if [ "$approve_status" != "200" ]; then
          echo "Failed to auto-approve pending enrollment for $NAME (status=$approve_status)." >&2
          [ -n "$approve_json" ] && echo "$approve_json" >&2
          exit 1
        fi
      else
        cat <<EOF
Approve this request in the UI before the agent container can start:
  http://localhost:5173 -> Security -> Pending enrollments
  requestId: $REQUEST_ID
EOF
      fi

      claim_started_at=$(date +%s)
      while true; do
        claim_payload=$(python3 - <<'PY' "$REQUEST_ID" "$CLAIM_TOKEN"
import json, sys
print(json.dumps({"requestId": sys.argv[1], "claimToken": sys.argv[2]}))
PY
)
        claim_resp=$(curl -sS "${curl_opts[@]}" -X POST "$AGENT_HOST_URL/api/v1/pending-enrollments/claim" \
          -H "Content-Type: application/json" \
          -d "$claim_payload" \
          -w $'\n%{http_code}')
        claim_status=${claim_resp##*$'\n'}
        CLAIM_JSON=${claim_resp%$'\n'*}
        printf '%s\n' "$CLAIM_JSON" >"$CLAIM_JSON_PATH"

        case "$claim_status" in
          200)
            python3 - <<'PY' "$CLAIM_JSON" "$DEVICE_CERT_PATH" "$DEVICE_CA_PATH" "$DEVICE_ID_PATH"
import json, sys
resp = json.loads(sys.argv[1])
open(sys.argv[2], "w").write(resp.get("certPem", ""))
open(sys.argv[3], "w").write(resp.get("caCertPem", ""))
open(sys.argv[4], "w").write(resp.get("deviceId", ""))
PY
            ;;
          202)
            now=$(date +%s)
            if [ "$PENDING_ENROLL_CLAIM_TIMEOUT_SEC" -gt 0 ] && [ $((now - claim_started_at)) -ge "$PENDING_ENROLL_CLAIM_TIMEOUT_SEC" ]; then
              echo "Timed out waiting for approval/claim for $NAME." >&2
              echo "Last claim response:" >&2
              echo "$CLAIM_JSON" >&2
              exit 1
            fi
            sleep "$PENDING_ENROLL_CLAIM_POLL_INTERVAL_SEC"
            continue
            ;;
          410)
            echo "Pending enrollment ended in terminal state for $NAME." >&2
            echo "$CLAIM_JSON" >&2
            exit 1
            ;;
          *)
            echo "Pending enrollment claim failed for $NAME (status=$claim_status)." >&2
            [ -n "$CLAIM_JSON" ] && echo "$CLAIM_JSON" >&2
            exit 1
            ;;
        esac

        if [ ! -s "$DEVICE_CERT_PATH" ]; then
          echo "Pending enrollment did not produce a device cert for $NAME." >&2
          exit 1
        fi
        break
      done
    else
      token_resp=$(curl -sS "${curl_opts[@]}" "${auth_args[@]}" -X POST "$HOST_URL/api/v1/enrollments" \
        -H "Content-Type: application/json" \
        -d '{"expiresInSec":3600}' \
        -w $'\n%{http_code}')
      token_status=${token_resp##*$'\n'}
      TOKEN_JSON=${token_resp%$'\n'*}
      if [ -z "$TOKEN_JSON" ] || [ "$token_status" != "200" ]; then
        echo "Failed to create enrollment token (status=$token_status)." >&2
        if [ "$token_status" = "401" ] || [ "$token_status" = "403" ]; then
          echo "Tip: auth is enabled; run with AUTH_TOKEN or AUTH_EMAIL/AUTH_PASSWORD." >&2
        fi
        if [ -n "$TOKEN_JSON" ]; then
          echo "$TOKEN_JSON" >&2
        fi
        exit 1
      fi

      TOKEN=$(python3 - <<'PY' "$TOKEN_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("token",""))
PY
)
      if [ -z "$TOKEN" ]; then
        echo "Failed to create enrollment token." >&2
        exit 1
      fi

      ENROLL_PAYLOAD=$(python3 - <<'PY' "$TOKEN" "$DEVICE_CSR_PATH" "$HARDWARE_ID_HASH"
import json, sys
hardware = sys.argv[3].strip()
print(json.dumps({
  "token": sys.argv[1],
  "csr": open(sys.argv[2]).read(),
  "capabilities": {
    "hw": {"identity": {"id": hardware, "source": "demo-agent-name"}}
  }
}))
PY
)

      enroll_resp=$(curl -sS "${curl_opts[@]}" -X POST "$AGENT_HOST_URL/api/v1/devices/enroll" \
        -H "Content-Type: application/json" \
        -d "$ENROLL_PAYLOAD" \
        -w $'\n%{http_code}')
      enroll_status=${enroll_resp##*$'\n'}
      ENROLL_JSON=${enroll_resp%$'\n'*}
      if [ -z "$ENROLL_JSON" ] || [ "$enroll_status" != "200" ]; then
        echo "Enrollment failed (status=$enroll_status)." >&2
        if [ "$enroll_status" = "400" ] && [[ "$ENROLL_JSON" == *"csr invalid"* ]]; then
          active_ca_crt="$BASE_DIR/dev-ca-active.crt"
          active_ca_key="$BASE_DIR/dev-ca-active.key"
          if [ -f "$active_ca_crt" ] && [ -f "$active_ca_key" ]; then
            crt_md5=$(openssl x509 -noout -modulus -in "$active_ca_crt" 2>/dev/null | openssl md5 2>/dev/null | awk '{print $2}' || true)
            key_md5=$(openssl rsa -noout -modulus -in "$active_ca_key" 2>/dev/null | openssl md5 2>/dev/null | awk '{print $2}' || true)
            if [ -n "$crt_md5" ] && [ -n "$key_md5" ] && [ "$crt_md5" != "$key_md5" ]; then
              cat >&2 <<EOF
Hint: active CA cert/key mismatch detected:
  $active_ca_crt
  $active_ca_key
Copy a matching cert/key pair and reload cert rotation:
  cp dev-ca.crt dev-ca-active.crt
  cp dev-ca.key dev-ca-active.key
  curl --cacert ./dev-ca.crt -H "Authorization: Bearer <token>" -H "Content-Type: application/json" -X POST $HOST_URL/api/v1/cert-rotation/reload -d '{"reason":"reload active CA after cert/key repair"}'
EOF
            fi
          fi
        fi
        if [ -n "$ENROLL_JSON" ]; then
          echo "$ENROLL_JSON" >&2
        fi
        exit 1
      fi

      python3 - <<'PY' "$ENROLL_JSON" "$DEVICE_CERT_PATH" "$DEVICE_ID_PATH"
import json, sys
resp=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(resp.get("certPem",""))
open(sys.argv[3],"w").write(resp.get("deviceId",""))
PY
    fi

    if [ ! -s "$DEVICE_CERT_PATH" ]; then
      echo "Enrollment failed; device cert not created." >&2
      exit 1
    fi
  fi

  RUNTIME_CA_SOURCE="$CA_CERT_PATH"
  if [ -s "$DEVICE_CA_PATH" ]; then
    RUNTIME_CA_SOURCE="$DEVICE_CA_PATH"
  fi
  cp -f "$RUNTIME_CA_SOURCE" "$CERT_DIR/dev-ca.crt"
  chmod 0644 "$DEVICE_CERT_PATH" "$DEVICE_KEY_PATH" || true
  cp -f "$SIGNING_PUB" "$CERT_DIR/signing.pub"

  cert_mount_mode="ro"
  if [ "$CERTS_RW" = "1" ]; then
    cert_mount_mode="rw"
  fi

  docker run -d \
    --name "$NAME" \
    --network host \
    --add-host host.docker.internal:host-gateway \
    "${extra_agent_host_arg[@]}" \
    --privileged \
    --user 0:0 \
    -e CONTROL_PLANE_URL="$AGENT_URL" \
    -e DEVICE_CERT_PATH=/certs/device.crt \
    -e DEVICE_KEY_PATH=/certs/device.key \
    -e CONTROL_PLANE_CA_CERT_PATH=/certs/dev-ca.crt \
    -e SIGNING_PUB_KEY_PATH=/certs/signing.pub \
    -e SIGNING_KEY_ID="$SIGNING_KEY_ID" \
    -e REQUIRE_ARTIFACT_SIGNATURE="$REQUIRE_ARTIFACT_SIGNATURE" \
    -e HARDWARE_IDENTITY="$HARDWARE_ID" \
    -e HARDWARE_IDENTITY_SALT="${HARDWARE_IDENTITY_SALT:-}" \
    -e LOG_EXPORT_ADDR="${LOG_EXPORT_ADDR:-}" \
    -e LOG_LEVEL="${LOG_LEVEL:-}" \
    -e DEMO_BASE_PORT="$DEMO_PORT" \
    -e DEMO_PORT_STEP="$DEMO_COMPONENT_PORT_STEP" \
    -e DEMO_COMPONENTS="$DEMO_COMPONENTS" \
    -e ALLOW_UNSUPPORTED_APPLY="$ALLOW_UNSUPPORTED_APPLY" \
    -v "$DATA_DIR:/data" \
    -v "$DEVICE_CERT_PATH:/certs/device.crt:${cert_mount_mode}" \
    -v "$DEVICE_KEY_PATH:/certs/device.key:ro" \
    -v "$CERT_DIR/dev-ca.crt:/certs/dev-ca.crt:ro" \
    -v "$CERT_DIR/signing.pub:/certs/signing.pub:ro" \
    "$IMAGE_NAME" >/dev/null

  echo "Demo agent running as $NAME."
  echo "Service URLs:"
  IFS=',' read -r -a component_list <<<"$DEMO_COMPONENTS"
  idx=0
  for comp in "${component_list[@]}"; do
    comp=$(echo "$comp" | xargs)
    [ -z "$comp" ] && continue
    port=$((DEMO_PORT + idx * DEMO_COMPONENT_PORT_STEP))
    echo "  ${comp}: http://localhost:${port}/index.html"
    idx=$((idx + 1))
  done
  if [ -s "$DEVICE_ID_PATH" ]; then
    echo "Device ID: $(cat "$DEVICE_ID_PATH")"
  fi
done
