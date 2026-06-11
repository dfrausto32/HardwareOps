#!/usr/bin/env bash
set -euo pipefail

LICENSE_KEY=${LICENSE_KEY:-}
OUT=${OUT:-./license.json}
ISSUED_TO=${ISSUED_TO:-}
MAX_DEVICES=${MAX_DEVICES:-}
NOT_BEFORE=${NOT_BEFORE:-}
EXPIRES_AT=${EXPIRES_AT:-}
KEY_ID=${KEY_ID:-}
VARIANT=${VARIANT:-}

if [ -z "$LICENSE_KEY" ] || [ ! -f "$LICENSE_KEY" ]; then
  echo "LICENSE_KEY must point to an Ed25519 private key (PEM)." >&2
  exit 1
fi
if [ -z "$ISSUED_TO" ]; then
  echo "ISSUED_TO is required." >&2
  exit 1
fi
if [ -z "$MAX_DEVICES" ]; then
  echo "MAX_DEVICES is required." >&2
  exit 1
fi

tmpdir=$(mktemp -d)
payload_json="$tmpdir/payload.json"
sig_bin="$tmpdir/sig.bin"

if [ -n "$VARIANT" ] && [ "$VARIANT" != "standard" ] && [ "$VARIANT" != "medical" ]; then
  echo "VARIANT must be 'standard' or 'medical' when set." >&2
  exit 1
fi

python3 - <<'PY' "$payload_json" "$ISSUED_TO" "$MAX_DEVICES" "$NOT_BEFORE" "$EXPIRES_AT" "$VARIANT"
import json, sys
out, issued_to, max_devices, not_before, expires_at, variant = sys.argv[1:7]
payload = {
    "issuedTo": issued_to,
    "maxDevices": int(max_devices),
}
if not_before:
    payload["notBefore"] = not_before
if expires_at:
    payload["expiresAt"] = expires_at
if variant:
    payload["variant"] = variant
with open(out, "w") as f:
    json.dump(payload, f, separators=(",", ":"), ensure_ascii=False)
PY

openssl pkeyutl -sign -inkey "$LICENSE_KEY" -rawin -in "$payload_json" -out "$sig_bin" >/dev/null 2>&1
sig_b64=$(base64 -w 0 "$sig_bin")

if [ -z "$KEY_ID" ]; then
  KEY_ID=$(openssl pkey -in "$LICENSE_KEY" -pubout -outform DER \
    | openssl dgst -sha256 -binary \
    | od -An -tx1 \
    | tr -d ' \n')
  KEY_ID="sha256:${KEY_ID}"
fi

cat > "$OUT" <<EOF
{"payload":$(cat "$payload_json"),"signature":"$sig_b64","keyId":"$KEY_ID"}
EOF

echo "License written to $OUT"
echo "Key ID: $KEY_ID"
