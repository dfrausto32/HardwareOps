#!/usr/bin/env sh
set -eu

log() {
  printf '%s\n' "$*"
}

ensure_dir() {
  dir_path=$1
  if [ ! -d "$dir_path" ]; then
    mkdir -p "$dir_path"
  fi
}

parse_json_field() {
  payload=$1
  field=$2
  python3 - "$field" "$payload" <<'PY'
import json
import sys

field = sys.argv[1]
try:
    data = json.loads(sys.argv[2])
except Exception:
    print("")
    raise SystemExit(0)
val = data.get(field, "")
if val is None:
    val = ""
if not isinstance(val, str):
    val = str(val)
print(val)
PY
}

write_enroll_response() {
  payload=$1
  cert_path=$2
  device_id_path=$3
  python3 - "$payload" "$cert_path" "$device_id_path" <<'PY'
import json
import sys

resp = json.loads(sys.argv[1])
cert = resp.get("certPem", "")
device_id = resp.get("deviceId", "")
if not cert or not device_id:
    raise SystemExit("missing certPem or deviceId in enroll response")
with open(sys.argv[2], "w", encoding="utf-8") as fh:
    fh.write(cert)
with open(sys.argv[3], "w", encoding="utf-8") as fh:
    fh.write(device_id)
PY
}

curl_json() {
  method=$1
  url=$2
  payload=${3:-}
  auth_token=${4:-}

  set -- -sS --fail -X "$method" "$url"
  if [ -n "$payload" ]; then
    set -- "$@" -H "Content-Type: application/json" -d "$payload"
  fi
  if [ -n "$auth_token" ]; then
    set -- "$@" -H "Authorization: Bearer $auth_token"
  fi

  if [ -n "${CONTROL_PLANE_CA_CERT_PATH:-}" ] && [ -f "${CONTROL_PLANE_CA_CERT_PATH:-}" ]; then
    curl --cacert "$CONTROL_PLANE_CA_CERT_PATH" "$@"
    return
  fi

  if [ "${DEMO_TLS_INSECURE:-0}" = "1" ]; then
    curl -k "$@"
    return
  fi

  curl "$@"
}

DATA_ROOT=${DEMO_DATA_ROOT:-/data}
CERT_DIR=${DEMO_CERT_DIR:-"$DATA_ROOT/certs"}
DEVICE_ID_PATH=${DEMO_DEVICE_ID_PATH:-"$DATA_ROOT/device-id"}
DEVICE_CSR_PATH=${DEMO_DEVICE_CSR_PATH:-"$CERT_DIR/device.csr"}
DEVICE_NAME=${DEMO_DEVICE_NAME:-"parcel-demo-agent-${DEMO_AGENT_SLOT:-1}"}
ENROLLMENT_TTL=${DEMO_ENROLLMENT_EXPIRES_SEC:-3600}

if [ -z "${STATE_PATH:-}" ]; then
  export STATE_PATH="$DATA_ROOT/agent-state.json"
fi
if [ -z "${ARTIFACT_ROOT:-}" ]; then
  export ARTIFACT_ROOT="$DATA_ROOT/artifacts"
fi
if [ -z "${DEVICE_KEY_PATH:-}" ]; then
  export DEVICE_KEY_PATH="$CERT_DIR/device.key"
fi
if [ -z "${DEVICE_CERT_PATH:-}" ]; then
  export DEVICE_CERT_PATH="$CERT_DIR/device.crt"
fi
if [ -z "${CONTROL_PLANE_URL:-}" ] && [ -n "${DEMO_DEVICES_URL:-}" ]; then
  export CONTROL_PLANE_URL="${DEMO_DEVICES_URL%/}"
fi

if [ -z "${CONTROL_PLANE_URL:-}" ]; then
  log "CONTROL_PLANE_URL or DEMO_DEVICES_URL is required"
  exit 1
fi
if [ -z "${DEMO_APP_URL:-}" ]; then
  log "DEMO_APP_URL is required"
  exit 1
fi

APP_URL=${DEMO_APP_URL%/}
DEVICES_URL=${DEMO_DEVICES_URL:-$CONTROL_PLANE_URL}
DEVICES_URL=${DEVICES_URL%/}

ensure_dir "$DATA_ROOT"
ensure_dir "$CERT_DIR"
ensure_dir "$(dirname "$STATE_PATH")"
ensure_dir "$ARTIFACT_ROOT"

if [ -n "${CONTROL_PLANE_CA_CERT_PEM:-}" ]; then
  if [ -z "${CONTROL_PLANE_CA_CERT_PATH:-}" ]; then
    export CONTROL_PLANE_CA_CERT_PATH="$CERT_DIR/control-plane-ca.crt"
  fi
  ensure_dir "$(dirname "$CONTROL_PLANE_CA_CERT_PATH")"
  printf '%s\n' "$CONTROL_PLANE_CA_CERT_PEM" >"$CONTROL_PLANE_CA_CERT_PATH"
  chmod 0644 "$CONTROL_PLANE_CA_CERT_PATH"
fi

if [ ! -s "$DEVICE_CERT_PATH" ] || [ ! -s "$DEVICE_KEY_PATH" ]; then
  log "device cert/key missing, enrolling demo agent"

  if [ ! -s "$DEVICE_KEY_PATH" ]; then
    openssl genrsa -out "$DEVICE_KEY_PATH" 2048 >/dev/null 2>&1
    chmod 0600 "$DEVICE_KEY_PATH"
  fi
  openssl req -new -key "$DEVICE_KEY_PATH" -out "$DEVICE_CSR_PATH" -subj "/CN=${DEVICE_NAME}" >/dev/null 2>&1

  ENROLLMENT_TOKEN=${DEMO_ENROLLMENT_TOKEN:-}
  if [ -z "$ENROLLMENT_TOKEN" ]; then
    AUTH_TOKEN=""
    if [ -n "${DEMO_BOOTSTRAP_EMAIL:-}" ] && [ -n "${DEMO_BOOTSTRAP_PASSWORD:-}" ]; then
      login_payload=$(python3 - "${DEMO_BOOTSTRAP_EMAIL}" "${DEMO_BOOTSTRAP_PASSWORD}" <<'PY'
import json
import sys

print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
      login_json=$(curl_json "POST" "${APP_URL}/api/v1/auth/login" "$login_payload")
      AUTH_TOKEN=$(parse_json_field "$login_json" "token")
      if [ -z "$AUTH_TOKEN" ]; then
        log "failed to acquire auth token from /api/v1/auth/login"
        exit 1
      fi
    fi

    enrollments_payload=$(python3 - "$ENROLLMENT_TTL" <<'PY'
import json
import sys

print(json.dumps({"expiresInSec": int(sys.argv[1])}))
PY
)
    enrollments_json=$(curl_json "POST" "${APP_URL}/api/v1/enrollments" "$enrollments_payload" "$AUTH_TOKEN")
    ENROLLMENT_TOKEN=$(parse_json_field "$enrollments_json" "token")
    if [ -z "$ENROLLMENT_TOKEN" ]; then
      log "failed to create enrollment token from /api/v1/enrollments"
      exit 1
    fi
  fi

  enroll_payload=$(python3 - "$ENROLLMENT_TOKEN" "$DEVICE_CSR_PATH" "$DEVICE_NAME" <<'PY'
import hashlib
import json
import sys

with open(sys.argv[2], "r", encoding="utf-8") as fh:
    csr = fh.read()
hardware_id = hashlib.sha256(sys.argv[3].encode("utf-8")).hexdigest()
print(json.dumps({
    "token": sys.argv[1],
    "csr": csr,
    "capabilities": {
        "hw": {
            "identity": {
                "id": hardware_id,
                "source": "demo-device-name",
            }
        }
    }
}))
PY
)
  enroll_json=$(curl_json "POST" "${DEVICES_URL}/api/v1/devices/enroll" "$enroll_payload")
  write_enroll_response "$enroll_json" "$DEVICE_CERT_PATH" "$DEVICE_ID_PATH"
  chmod 0644 "$DEVICE_CERT_PATH"
  log "demo agent enrolled with device id: $(cat "$DEVICE_ID_PATH")"
fi

exec /usr/local/bin/agent
