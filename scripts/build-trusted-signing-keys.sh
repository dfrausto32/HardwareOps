#!/usr/bin/env bash
set -euo pipefail

OUT_FILE=${1:-}
PUB_PATH=${2:-${SIGNING_PUB_KEY_PATH:-}}
ALGORITHM=${ALGORITHM:-ed25519}
DISPLAY_NAME=${DISPLAY_NAME:-}
NOTES=${NOTES:-}
KEY_ID=${SIGNING_KEY_ID:-}

if [ -z "$OUT_FILE" ]; then
  echo "usage: $0 <out-file> [public-key-path]" >&2
  exit 1
fi

if [ -z "$PUB_PATH" ] || [ ! -s "$PUB_PATH" ]; then
  echo "public key not found: $PUB_PATH" >&2
  exit 1
fi

if [ -z "$KEY_ID" ]; then
  keyid_path="${PUB_PATH%.*}.keyid"
  if [ -f "$keyid_path" ]; then
    KEY_ID=$(tr -d '\n' <"$keyid_path")
  else
    KEY_ID=$(openssl pkey -pubin -in "$PUB_PATH" -outform DER \
      | openssl dgst -sha256 -binary \
      | od -An -tx1 \
      | tr -d ' \n')
    KEY_ID="sha256:${KEY_ID}"
  fi
fi

if [ -z "$DISPLAY_NAME" ]; then
  DISPLAY_NAME="$KEY_ID"
fi

mkdir -p "$(dirname "$OUT_FILE")"

python3 - "$OUT_FILE" "$PUB_PATH" "$KEY_ID" "$ALGORITHM" "$DISPLAY_NAME" "$NOTES" <<'PY'
import json
import pathlib
import sys

out_file, pub_path, key_id, algorithm, display_name, notes = sys.argv[1:]
public_key = pathlib.Path(pub_path).read_text(encoding="utf-8").strip()
payload = {
    "keys": [
        {
            "keyId": key_id.strip(),
            "displayName": display_name.strip(),
            "algorithm": algorithm.strip(),
            "publicKeyPem": public_key,
            "notes": notes.strip(),
        }
    ]
}
pathlib.Path(out_file).write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
PY

echo "Wrote trusted signing key set: $OUT_FILE"
