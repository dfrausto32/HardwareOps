#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
TMP_DIR=${TMP_DIR:-/tmp/parcel}
CA_CERT=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}
CLEANUP=${CLEANUP:-0}

mkdir -p "$TMP_DIR"

curl_opts=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT" ]; then
    curl_opts+=(--cacert "$CA_CERT")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

# 0) Health check
if ! curl -s "${curl_opts[@]}" "$BASE_URL/healthz" >/dev/null; then
  echo "Control-plane not reachable at $BASE_URL" >&2
  exit 1
fi

# 1) Create enrollment token
TOKEN_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/enrollments" \
  -H "Content-Type: application/json" \
  -d '{"expiresInSec":3600}')

echo "Enrollment response: $TOKEN_JSON"

TOKEN=$(python3 -c 'import json,sys
try:
    data=json.loads(sys.stdin.read())
    token=data.get("token", "")
    if not token:
        raise ValueError("missing token")
    print(token)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
' <<<"$TOKEN_JSON") || {
  echo "Enrollment failed; check control-plane logs and database." >&2
  exit 1
}

# 2) Generate CSR
openssl req -newkey rsa:2048 -nodes \
  -keyout "$TMP_DIR/device.key" -out "$TMP_DIR/device.csr" \
  -subj "/CN=parcel-device"

if [ ! -s "$TMP_DIR/device.csr" ]; then
  echo "CSR file missing or empty: $TMP_DIR/device.csr" >&2
  exit 1
fi

# 3) Enroll device (token + CSR)
PAYLOAD_FILE="$TMP_DIR/enroll-payload.json"
python3 - "$TOKEN" "$TMP_DIR/device.csr" <<'PY' > "$PAYLOAD_FILE"
import json, sys
from pathlib import Path

token = sys.argv[1]
csr = Path(sys.argv[2]).read_text()
print(json.dumps({"token": token, "csr": csr}))
PY

if [ ! -s "$PAYLOAD_FILE" ]; then
  echo "Enrollment payload missing or empty: $PAYLOAD_FILE" >&2
  exit 1
fi

ENROLL_JSON=$(curl -s "${curl_opts[@]}" -X POST "$BASE_URL/api/v1/devices/enroll" \
  -H "Content-Type: application/json" \
  -d @"$PAYLOAD_FILE")

echo "Enroll response: $ENROLL_JSON"

# Extract device ID + certs
python3 - "$ENROLL_JSON" "$TMP_DIR/device.crt" "$TMP_DIR/ca.crt" "$TMP_DIR/device-id" <<'PY'
import json, sys
data=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(data.get("certPem",""))
open(sys.argv[3],"w").write(data.get("caCertPem",""))
open(sys.argv[4],"w").write(data.get("deviceId",""))
PY

DEVICE_ID=$(cat "$TMP_DIR/device-id")

checkin_opts=("${curl_opts[@]}")
if [[ "$BASE_URL" == https:* ]]; then
  checkin_opts+=(--cert "$TMP_DIR/device.crt" --key "$TMP_DIR/device.key")
fi

CHECKIN_JSON=$(curl -s "${checkin_opts[@]}" -X POST "$BASE_URL/api/v1/devices/checkin" \
  -H "Content-Type: application/json" \
  -d "{\"deviceId\":\"$DEVICE_ID\",\"agentVersion\":\"0.1.0\",\"current\":{\"softwareVersion\":\"v1\",\"configRev\":\"c1\"}}")

echo "Check-in response: $CHECKIN_JSON"

if [ "$CLEANUP" = "1" ]; then
  curl -s "${curl_opts[@]}" -X DELETE "$BASE_URL/api/v1/devices/$DEVICE_ID" >/dev/null
  echo "Deleted device: $DEVICE_ID"
else
  echo "Device ID: $DEVICE_ID"
  echo "Delete: curl -X DELETE $BASE_URL/api/v1/devices/$DEVICE_ID"
fi
