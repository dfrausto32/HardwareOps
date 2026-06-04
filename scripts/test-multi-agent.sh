#!/usr/bin/env bash
# test-multi-agent.sh — enroll N distinct devices and check each one in.
#
# Mirrors the proven enrollment path in artifact-e2e.sh: for each agent it
# generates a key + CSR, enrolls via the control-plane to obtain a device
# certificate, then runs the agent once so it checks in over mTLS. Each agent
# gets a unique HARDWARE_IDENTITY so they register as distinct devices.
#
# Reads from the environment: BASE_URL, AUTH_EMAIL, AUTH_PASSWORD,
# CA_CERT_PATH (or INSECURE=1), COUNT, WORK_DIR, AGENT_BIN.
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:?BASE_URL required}
AUTH_EMAIL=${AUTH_EMAIL:?AUTH_EMAIL required}
AUTH_PASSWORD=${AUTH_PASSWORD:?AUTH_PASSWORD required}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}
COUNT=${COUNT:-3}
WORK_DIR=${WORK_DIR:-$(mktemp -d /tmp/parcel-multi-agent.XXXXXX)}
AGENT_BIN=${AGENT_BIN:-$WORK_DIR/agent-bin}

mkdir -p "$WORK_DIR"

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

# Build the agent once; running a prebuilt binary in sequence avoids racing
# concurrent "go run" invocations on the shared build cache.
( cd "$BASE_DIR/agent" && go build -o "$AGENT_BIN" ./cmd/agent )

AUTH_TOKEN=$(curl -sf "${curl_opts[@]}" "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$AUTH_EMAIL\",\"password\":\"$AUTH_PASSWORD\"}" \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])')

ENROLL_TOKEN=$(curl -sf "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/enrollments" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"expiresInSec":3600,"maxUses":10}' \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])')

echo "Enrolling and checking in $COUNT agents ..."
for i in $(seq 1 "$COUNT"); do
  D="$WORK_DIR/agent-$i"
  mkdir -p "$D/data"
  openssl req -newkey rsa:2048 -nodes \
    -keyout "$D/device.key" -out "$D/device.csr" \
    -subj "/CN=parcel-device-$i" 2>/dev/null
  CSR=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' "$D/device.csr")
  ENROLL_JSON=$(curl -sf "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/devices/enroll" \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$ENROLL_TOKEN\",\"csr\":\"$CSR\"}")
  DEVICE_ID=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["deviceId"])' "$ENROLL_JSON")
  python3 -c 'import json,sys; open(sys.argv[2],"w").write(json.loads(sys.argv[1]).get("certPem",""))' "$ENROLL_JSON" "$D/device.crt"
  if [ ! -s "$D/device.crt" ]; then
    echo "agent $i: enrollment did not return a device certificate" >&2
    exit 1
  fi
  printf '{"deviceId":"%s","agentVersion":"0.1.0","currentVersion":"","currentConfigRev":""}\n' \
    "$DEVICE_ID" > "$D/state.json"

  ca_env=()
  [ -n "$CA_CERT_PATH" ] && ca_env=("CONTROL_PLANE_CA_CERT_PATH=$CA_CERT_PATH")
  echo "agent $i device=$DEVICE_ID ..."
  env CONTROL_PLANE_URL="$BASE_URL" \
    STATE_PATH="$D/state.json" \
    ARTIFACT_ROOT="$D/data" \
    DEVICE_CERT_PATH="$D/device.crt" \
    DEVICE_KEY_PATH="$D/device.key" \
    HARDWARE_IDENTITY="e2e-agent-$i-$(hostname)" \
    "${ca_env[@]}" \
    "$AGENT_BIN" -once
done

echo "All $COUNT agents enrolled and checked in"
