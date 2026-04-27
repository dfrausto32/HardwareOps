#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RUN_ID=$(date +%s)

LAB_ROOT=${LAB_ROOT:-"${TMPDIR:-/tmp}/parcel-prod-docker"}
ENV_FILE=${ENV_FILE:-"$LAB_ROOT/.env.onprem"}
CERTS_DIR=${CERTS_DIR:-"$LAB_ROOT/certs"}
WORK_DIR=${WORK_DIR:-"$LAB_ROOT/first-contact"}
CA_CERT_PATH=${CA_CERT_PATH:-"$CERTS_DIR/ca.crt"}
CONTAINER_IMAGE=${CONTAINER_IMAGE:-ubuntu:24.04}
CONTAINER_NAME=${CONTAINER_NAME:-"hwops-prodtest-first-contact-agent-$RUN_ID"}
CONTAINER_HOSTNAME=${CONTAINER_HOSTNAME:-$CONTAINER_NAME}
KEEP_AGENT_CONTAINER=${KEEP_AGENT_CONTAINER:-0}

PROFILE_NAME=${PROFILE_NAME:-"prod-lab-first-contact-$RUN_ID"}
PROFILE_EXPIRES_IN_SEC=${PROFILE_EXPIRES_IN_SEC:-86400}
PROFILE_MAX_USES=${PROFILE_MAX_USES:-1}
PROFILE_DEFAULT_LABELS_JSON=${PROFILE_DEFAULT_LABELS_JSON:-'{"site":"prod-lab","flow":"first-contact"}'}
REQUEST_WAIT_SEC=${REQUEST_WAIT_SEC:-120}
DEVICE_WAIT_SEC=${DEVICE_WAIT_SEC:-180}
CHECKIN_WAIT_SEC=${CHECKIN_WAIT_SEC:-180}
AGENT_ARCH=${AGENT_ARCH:-}
AGENT_BUNDLE=${AGENT_BUNDLE:-}

if [ ! -f "$ENV_FILE" ]; then
  echo "Lab env file not found: $ENV_FILE" >&2
  echo "Run: ./scripts/testing/prod-lab-init.sh && ./scripts/testing/prod-lab-up.sh" >&2
  exit 1
fi

if [ ! -f "$CA_CERT_PATH" ]; then
  echo "CA cert not found: $CA_CERT_PATH" >&2
  exit 1
fi

read_env() {
  local key="$1"
  awk -F= -v k="$key" '$1==k{print substr($0, index($0,$2))}' "$ENV_FILE" | tail -n1
}

host_from_url() {
  local url="$1"
  echo "$url" | sed -e 's#^[a-zA-Z0-9+.-]*://##' -e 's#/.*$##' -e 's/:.*$//'
}

host_resolves() {
  local host="$1"
  getent hosts "$host" >/dev/null 2>&1
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "Unsupported host architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

locate_agent_bundle() {
  local arch="$1"
  find "$BASE_DIR/dist/installers" -type f -name "parcel-agent-*-linux-${arch}.tar.gz" | sort | tail -n1
}

PUBLIC_BASE_URL=$(read_env PUBLIC_BASE_URL)
AGENT_BASE_URL=$(read_env AGENT_BASE_URL)
AUTH_EMAIL=$(read_env AUTH_BOOTSTRAP_EMAIL)
AUTH_PASSWORD=$(read_env AUTH_BOOTSTRAP_PASSWORD)

if [ -z "$PUBLIC_BASE_URL" ] || [ -z "$AGENT_BASE_URL" ]; then
  echo "PUBLIC_BASE_URL / AGENT_BASE_URL missing in $ENV_FILE" >&2
  exit 1
fi
if [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; then
  echo "AUTH_BOOTSTRAP_EMAIL / AUTH_BOOTSTRAP_PASSWORD missing in $ENV_FILE" >&2
  exit 1
fi

public_host=$(host_from_url "$PUBLIC_BASE_URL")
agent_host=$(host_from_url "$AGENT_BASE_URL")
curl_opts=(--cacert "$CA_CERT_PATH")
docker_host_args=(--add-host "$public_host:host-gateway")
if [ "$agent_host" != "$public_host" ]; then
  docker_host_args+=(--add-host "$agent_host:host-gateway")
fi
if ! host_resolves "$public_host"; then
  curl_opts+=(--resolve "$public_host:443:127.0.0.1")
fi
if ! host_resolves "$agent_host"; then
  curl_opts+=(--resolve "$agent_host:443:127.0.0.1")
fi

if ! curl --silent --show-error --fail "${curl_opts[@]}" "$PUBLIC_BASE_URL/healthz" >/dev/null; then
  echo "Control-plane not reachable at $PUBLIC_BASE_URL" >&2
  exit 1
fi

if [ -z "$AGENT_ARCH" ]; then
  AGENT_ARCH=$(detect_arch)
fi

if [ -z "$AGENT_BUNDLE" ]; then
  AGENT_BUNDLE=$(locate_agent_bundle "$AGENT_ARCH")
fi
if [ -z "$AGENT_BUNDLE" ] || [ ! -f "$AGENT_BUNDLE" ]; then
  echo "Agent bundle not found for linux/$AGENT_ARCH. Build one first with ./scripts/build-installers.sh." >&2
  exit 1
fi

mkdir -p "$WORK_DIR"
rm -rf "$WORK_DIR/bundle"
mkdir -p "$WORK_DIR/bundle"
tar -xzf "$AGENT_BUNDLE" -C "$WORK_DIR/bundle"

bundle_dir=$(find "$WORK_DIR/bundle" -mindepth 1 -maxdepth 1 -type d | head -n1)
if [ -z "$bundle_dir" ] || [ ! -f "$bundle_dir/parcel-agent" ]; then
  echo "Extracted bundle missing expected files: $AGENT_BUNDLE" >&2
  exit 1
fi

cleanup() {
  if [ "$KEEP_AGENT_CONTAINER" != "1" ]; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

login_payload=$(python3 - "$AUTH_EMAIL" "$AUTH_PASSWORD" <<'PY'
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
login_resp=$(curl --silent --show-error --fail "${curl_opts[@]}" \
  -H "Content-Type: application/json" \
  -d "$login_payload" \
  "$PUBLIC_BASE_URL/api/v1/auth/login")
AUTH_TOKEN=$(python3 - "$login_resp" <<'PY'
import json, sys
print(json.loads(sys.argv[1]).get("token", ""))
PY
)
if [ -z "$AUTH_TOKEN" ]; then
  echo "Failed to obtain auth token." >&2
  exit 1
fi

profile_payload=$(python3 - \
  "$PROFILE_NAME" \
  "$PROFILE_EXPIRES_IN_SEC" \
  "$PROFILE_MAX_USES" \
  "$PROFILE_DEFAULT_LABELS_JSON" <<'PY'
import json, sys
payload = {
  "name": sys.argv[1],
  "expiresInSec": int(sys.argv[2]),
  "maxUses": int(sys.argv[3]),
  "requireApproval": True,
}
labels_raw = sys.argv[4].strip()
if labels_raw:
  labels = json.loads(labels_raw)
  if not isinstance(labels, dict):
    raise SystemExit("PROFILE_DEFAULT_LABELS_JSON must be a JSON object")
  payload["defaultLabels"] = labels
print(json.dumps(payload))
PY
)

profile_resp=$(curl -sS "${curl_opts[@]}" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$profile_payload" \
  -w $'\n%{http_code}' \
  "$PUBLIC_BASE_URL/api/v1/enrollment-profiles")
profile_status=${profile_resp##*$'\n'}
profile_json=${profile_resp%$'\n'*}
if [ "$profile_status" != "201" ]; then
  echo "Failed to create enrollment profile (status=$profile_status)." >&2
  [ -n "$profile_json" ] && echo "$profile_json" >&2
  exit 1
fi

PROFILE_TOKEN=$(python3 - "$profile_json" <<'PY'
import json, sys
print(json.loads(sys.argv[1]).get("bootstrapToken", ""))
PY
)
if [ -z "$PROFILE_TOKEN" ]; then
  echo "Enrollment profile created without bootstrap token." >&2
  exit 1
fi

container_cmd=$(cat <<'EOF'
set -euo pipefail
cd /bundle
./scripts/agent-install.sh \
  AGENT_SRC=/bundle/parcel-agent \
  CONTROL_PLANE_URL="$CONTROL_PLANE_URL" \
  CONTROL_PLANE_CA_CERT_SRC=/input/ca.crt \
  AGENT_ENROLL_MODE=approval \
  ENROLLMENT_PROFILE_TOKEN="$ENROLLMENT_PROFILE_TOKEN" \
  ENABLE_SERVICE=0
exec su -s /bin/sh parcel -c 'set -a; . /etc/parcel/agent/agent.env; exec /usr/local/bin/parcel-agent'
EOF
)

docker run -d \
  --name "$CONTAINER_NAME" \
  --hostname "$CONTAINER_HOSTNAME" \
  "${docker_host_args[@]}" \
  -e CONTROL_PLANE_URL="$AGENT_BASE_URL" \
  -e ENROLLMENT_PROFILE_TOKEN="$PROFILE_TOKEN" \
  -v "$bundle_dir:/bundle:ro" \
  -v "$CA_CERT_PATH:/input/ca.crt:ro" \
  "$CONTAINER_IMAGE" \
  bash -lc "$container_cmd" >/dev/null

request_id=""
request_started=$(date +%s)
while true; do
  if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || echo false)" != "true" ]; then
    echo "Agent container exited before reaching the pending queue." >&2
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi

  list_json=$(curl --silent --show-error --fail "${curl_opts[@]}" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    "$PUBLIC_BASE_URL/api/v1/pending-enrollments?status=pending")
  request_id=$(python3 - "$list_json" "$CONTAINER_HOSTNAME" <<'PY'
import json, sys
payload = json.loads(sys.argv[1])
hostname = sys.argv[2]
for item in payload.get("items", []):
    meta = item.get("metadata") or {}
    if isinstance(meta, dict) and meta.get("hostname") == hostname:
        print(item.get("requestId", ""))
        break
PY
)
  if [ -n "$request_id" ]; then
    break
  fi
  now=$(date +%s)
  if [ $((now - request_started)) -ge "$REQUEST_WAIT_SEC" ]; then
    echo "Timed out waiting for pending enrollment request." >&2
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi
  sleep 2
done

curl --silent --show-error --fail "${curl_opts[@]}" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' \
  "$PUBLIC_BASE_URL/api/v1/pending-enrollments/$request_id/approve" >/dev/null

device_id=""
device_started=$(date +%s)
while true; do
  if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || echo false)" != "true" ]; then
    echo "Agent container exited before writing issued identity." >&2
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi

  device_id=$(docker exec "$CONTAINER_NAME" sh -lc 'if [ -s /var/lib/parcel/agent/device-id ]; then cat /var/lib/parcel/agent/device-id; fi' 2>/dev/null || true)
  if [ -n "$device_id" ]; then
    if docker exec "$CONTAINER_NAME" test -s /etc/parcel/agent/certs/device.crt; then
      if docker exec "$CONTAINER_NAME" test ! -e /var/lib/parcel/agent/bootstrap-state.json; then
        break
      fi
    fi
  fi
  now=$(date +%s)
  if [ $((now - device_started)) -ge "$DEVICE_WAIT_SEC" ]; then
    echo "Timed out waiting for issued device identity." >&2
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi
  sleep 2
done

checkin_started=$(date +%s)
while true; do
  devices_json=$(curl --silent --show-error --fail "${curl_opts[@]}" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    "$PUBLIC_BASE_URL/api/v1/devices?status=active&limit=200")
  device_status=$(python3 - "$devices_json" "$device_id" <<'PY'
import json, sys
payload = json.loads(sys.argv[1])
device_id = sys.argv[2]
for item in payload.get("items", []):
    if item.get("deviceId") == device_id:
        print(item.get("status", ""))
        break
PY
)
  if [ "$device_status" = "active" ]; then
    if docker logs "$CONTAINER_NAME" 2>&1 | grep -q "check-in ok"; then
      break
    fi
  fi
  now=$(date +%s)
  if [ $((now - checkin_started)) -ge "$CHECKIN_WAIT_SEC" ]; then
    echo "Timed out waiting for active mTLS check-in." >&2
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi
  sleep 2
done

cat <<EOF
First-contact approval onboarding passed.

Bundle:       $AGENT_BUNDLE
Container:    $CONTAINER_NAME
Request ID:   $request_id
Device ID:    $device_id
Public URL:   $PUBLIC_BASE_URL
Agent URL:    $AGENT_BASE_URL

Verified:
  - packaged agent installer wrote approval-mode config
  - agent stayed alive while waiting for operator approval
  - approval produced device.crt + device-id without reinstall
  - bootstrap state cleared after issuance
  - same agent process reached active mTLS check-in
EOF
