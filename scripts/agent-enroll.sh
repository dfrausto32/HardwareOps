#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE_URL=${CONTROL_PLANE_URL:-https://hardwareops.internal}
CERT_DIR=${CERT_DIR:-/etc/hardwareops/agent/certs}
DEVICE_ID_PATH=${DEVICE_ID_PATH:-/var/lib/hardwareops/agent/device-id}
CA_CERT_PATH=${CA_CERT_PATH:-}
INSECURE=${INSECURE:-0}
AGENT_USER=${AGENT_USER:-hardwareops}
AGENT_GROUP=${AGENT_GROUP:-hardwareops}

mkdir -p "$CERT_DIR"

curl_opts=()
if [[ "$CONTROL_PLANE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  else
    echo "CA_CERT_PATH not set. Use CA_CERT_PATH=/path/to/ca.crt or INSECURE=1 for testing." >&2
    exit 1
  fi
fi

DEVICE_KEY="$CERT_DIR/device.key"
DEVICE_CSR="$CERT_DIR/device.csr"
DEVICE_CERT="$CERT_DIR/device.crt"
CA_CERT_OUT="$CERT_DIR/ca.crt"

openssl req -newkey rsa:2048 -nodes \
  -keyout "$DEVICE_KEY" -out "$DEVICE_CSR" \
  -subj "/CN=hardwareops-device"

TOKEN_JSON=$(curl -s "${curl_opts[@]}" -X POST "$CONTROL_PLANE_URL/api/v1/enrollments" \
  -H "Content-Type: application/json" \
  -d '{"expiresInSec":3600}')
TOKEN=$(python3 - <<'PY' "$TOKEN_JSON"
import json, sys
print(json.loads(sys.argv[1]).get("token",""))
PY
)
if [ -z "$TOKEN" ]; then
  echo "Failed to create enrollment token." >&2
  exit 1
fi

ENROLL_PAYLOAD=$(python3 - <<'PY' "$TOKEN" "$DEVICE_CSR"
import hashlib
import json
import os
import socket
import sys

with open(sys.argv[2], "r", encoding="utf-8") as fh:
    csr = fh.read()

raw = os.environ.get("HARDWARE_IDENTITY", "").strip()
source = "env"
if not raw:
    for path in ("/etc/machine-id", "/var/lib/dbus/machine-id"):
        try:
            with open(path, "r", encoding="utf-8") as fh:
                raw = fh.read().strip()
                if raw:
                    source = "machine-id"
                    break
        except Exception:
            pass
if not raw:
    raw = socket.gethostname().strip()
    source = "hostname"
salt = os.environ.get("HARDWARE_IDENTITY_SALT", "").strip()
payload = raw if not salt else f"{raw}|{salt}"
hardware = hashlib.sha256(payload.encode("utf-8")).hexdigest()

print(json.dumps({
    "token": sys.argv[1],
    "csr": csr,
    "capabilities": {"hw": {"identity": {"id": hardware, "source": source}}},
}))
PY
)

ENROLL_JSON=$(curl -s "${curl_opts[@]}" -X POST "$CONTROL_PLANE_URL/api/v1/devices/enroll" \
  -H "Content-Type: application/json" \
  -d "$ENROLL_PAYLOAD")

python3 - <<'PY' "$ENROLL_JSON" "$DEVICE_CERT" "$CA_CERT_OUT" "$DEVICE_ID_PATH"
import json, sys
resp=json.loads(sys.argv[1])
open(sys.argv[2],"w").write(resp.get("certPem",""))
open(sys.argv[3],"w").write(resp.get("caCertPem",""))
open(sys.argv[4],"w").write(resp.get("deviceId",""))
PY

chmod 0600 "$DEVICE_KEY"
chmod 0644 "$DEVICE_CERT" "$CA_CERT_OUT"

if id -u "$AGENT_USER" >/dev/null 2>&1; then
  chown -R "$AGENT_USER:$AGENT_GROUP" "$CERT_DIR"
  if [ -f "$DEVICE_ID_PATH" ]; then
    chown "$AGENT_USER:$AGENT_GROUP" "$DEVICE_ID_PATH"
  fi
else
  echo "Warning: user $AGENT_USER not found; certs remain owned by current user." >&2
fi

echo "Enrolled device: $(cat "$DEVICE_ID_PATH")"
