#!/usr/bin/env bash
# Test duplicate artifact ingest policy: idempotent, conflict, and supersede paths.
# Requires a running control-plane with artifact signing enforced.
#
# Required env:
#   BASE_URL          control-plane base URL
#   AUTH_TOKEN        service token with artifact.publish scope
#   SIGNING_KEY       path to Ed25519 private key file
#   SIGNING_KEY_ID    key ID registered in the trusted-signing-keys registry
#
# Optional env:
#   ARTIFACT_NAME     default: dup-smoke-<run-id>
#   ARTIFACT_VERSION  default: 1.0.0
#   ARTIFACT_TYPE     default: app_bundle
#   WORK_DIR          default: /tmp/hwops-dup-smoke
#   INSECURE          set to 1 to skip TLS cert verification
#   CA_CERT_PATH      path to CA cert for HTTPS

set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

BASE_URL=${BASE_URL:-https://localhost:8080}
AUTH_TOKEN=${AUTH_TOKEN:-}
SIGNING_KEY=${SIGNING_KEY:-}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}
RUN_ID=${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}
ARTIFACT_NAME=${ARTIFACT_NAME:-dup-smoke-$RUN_ID}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-1.0.0}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
WORK_DIR=${WORK_DIR:-/tmp/hwops-dup-smoke-$RUN_ID}
INSECURE=${INSECURE:-0}
CA_CERT_PATH=${CA_CERT_PATH:-}

PASS_COUNT=0
FAIL_COUNT=0

log()  { echo "[dup-smoke] $*"; }
pass() { echo "[dup-smoke] PASS: $*"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[dup-smoke] FAIL: $*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }

[ -n "$AUTH_TOKEN" ]    || { echo "AUTH_TOKEN required" >&2; exit 1; }
[ -n "$SIGNING_KEY" ]   || { echo "SIGNING_KEY required" >&2; exit 1; }
[ -n "$SIGNING_KEY_ID" ] || { echo "SIGNING_KEY_ID required" >&2; exit 1; }

curl_opts=(-sS)
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    curl_opts+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    curl_opts+=(-k)
  fi
fi

auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")

mkdir -p "$WORK_DIR/input-v1" "$WORK_DIR/input-v2"
trap 'rm -rf "$WORK_DIR"' EXIT

# Write two distinct artifact content directories
cat > "$WORK_DIR/input-v1/README.txt" <<EOF
dup-smoke artifact
content: v1-canonical
name: $ARTIFACT_NAME
version: $ARTIFACT_VERSION
EOF

cat > "$WORK_DIR/input-v2/README.txt" <<EOF
dup-smoke artifact
content: v2-different
name: $ARTIFACT_NAME
version: $ARTIFACT_VERSION
EOF

# pack_artifact <input-dir> <out-tar>
# Outputs: JSON with artifactPath, sha256, signature, signatureKeyId
pack_artifact() {
  local input_dir="$1" out_tar="$2"
  python3 "$BASE_DIR/scripts/artifact-pack.py" \
    --name "$ARTIFACT_NAME" \
    --version "$ARTIFACT_VERSION" \
    --type "$ARTIFACT_TYPE" \
    --input-dir "$input_dir" \
    --out "$out_tar" \
    --signing-key "$SIGNING_KEY" \
    --signing-key-id "$SIGNING_KEY_ID"
}

# presign_and_upload <tar-file>
# Echoes JSON: {"artifactId":"...","objectKey":"...","sha256":"...","size":N,"sig":"...","sigKeyId":"..."}
presign_and_upload() {
  local tar_file="$1"
  local pack_json="$2"

  local sha sig sig_key_id size
  sha=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['sha256'])" "$pack_json")
  sig=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('signature',''))" "$pack_json")
  sig_key_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('signatureKeyId',''))" "$pack_json")
  size=$(wc -c < "$tar_file" | tr -d ' ')

  local presign_json
  presign_json=$(curl "${curl_opts[@]}" --fail "${auth_args[@]}" \
    -X POST "$BASE_URL/api/v1/artifacts/presign-upload" \
    -H "Content-Type: application/json" \
    -d "{\"filename\":\"artifact.tar.gz\",\"contentType\":\"application/gzip\",\"expiresSeconds\":300}")

  local artifact_id object_key upload_url
  artifact_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['artifactId'])" "$presign_json")
  object_key=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['objectKey'])" "$presign_json")
  upload_url=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['uploadUrl'])" "$presign_json")

  curl -sS --fail -X PUT -H "Content-Type: application/gzip" --upload-file "$tar_file" "$upload_url" >/dev/null

  python3 - <<PY
import json
print(json.dumps({
  "artifactId": "$artifact_id",
  "objectKey": "$object_key",
  "sha256": "$sha",
  "sizeBytes": int("$size"),
  "signature": "$sig",
  "signatureKeyId": "$sig_key_id",
}))
PY
}

# complete_artifact <upload_json> [supersede=true]
# Outputs HTTP status code on stdout, response body on stderr
complete_artifact() {
  local upload_json="$1"
  local supersede="${2:-}"

  local artifact_id object_key sha size sig sig_key_id
  artifact_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['artifactId'])" "$upload_json")
  object_key=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['objectKey'])" "$upload_json")
  sha=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['sha256'])" "$upload_json")
  size=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['sizeBytes'])" "$upload_json")
  sig=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('signature',''))" "$upload_json")
  sig_key_id=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('signatureKeyId',''))" "$upload_json")

  local complete_payload
  complete_payload=$(python3 - <<PY
import json
payload = {
  "artifactId": "$artifact_id",
  "name": "$ARTIFACT_NAME",
  "version": "$ARTIFACT_VERSION",
  "type": "$ARTIFACT_TYPE",
  "objectKey": "$object_key",
  "sha256": "$sha",
  "sizeBytes": int("$size"),
}
if "$sig":
  payload["signature"] = "$sig"
if "$sig_key_id":
  payload["signatureKeyId"] = "$sig_key_id"
print(json.dumps(payload))
PY
)

  local url="$BASE_URL/api/v1/artifacts/complete"
  [ -n "$supersede" ] && url="${url}?supersede=true"

  local resp_file="$WORK_DIR/complete-resp.json"
  local http_code
  http_code=$(curl "${curl_opts[@]}" "${auth_args[@]}" \
    -X POST "$url" \
    -H "Content-Type: application/json" \
    -d "$complete_payload" \
    -o "$resp_file" \
    -w "%{http_code}")

  echo "$http_code"
  cat "$resp_file" >&2
}

# ── Scenario 1: baseline upload ──────────────────────────────────────────────
log "Scenario 1: baseline upload of $ARTIFACT_NAME $ARTIFACT_VERSION"
PACK1=$(pack_artifact "$WORK_DIR/input-v1" "$WORK_DIR/v1.tar.gz")
UP1=$(presign_and_upload "$WORK_DIR/v1.tar.gz" "$PACK1")
ARTIFACT_ID_1=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['artifactId'])" "$UP1")

HTTP1=$(complete_artifact "$UP1" 2>"$WORK_DIR/resp1.json")
if [ "$HTTP1" = "200" ]; then
  pass "Scenario 1 — baseline upload returned 200 (id=${ARTIFACT_ID_1:0:8}...)"
else
  fail "Scenario 1 — baseline upload returned $HTTP1: $(cat "$WORK_DIR/resp1.json")"
fi

# ── Scenario 2: same content re-upload → idempotent ─────────────────────────
log "Scenario 2: re-upload same SHA256 → expect 200 + duplicate:true + same artifact ID"
UP2=$(presign_and_upload "$WORK_DIR/v1.tar.gz" "$PACK1")  # same tar, same sha

HTTP2=$(complete_artifact "$UP2" 2>"$WORK_DIR/resp2.json")
RESP2=$(cat "$WORK_DIR/resp2.json")
RETURNED_ID=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('artifactId',''))" "$RESP2" 2>/dev/null || echo "")
IS_DUP=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('duplicate',False))" "$RESP2" 2>/dev/null || echo "False")

if [ "$HTTP2" = "200" ] && [ "$IS_DUP" = "True" ] && [ "$RETURNED_ID" = "$ARTIFACT_ID_1" ]; then
  pass "Scenario 2 — idempotent: 200 duplicate:true, returned original id"
elif [ "$HTTP2" = "200" ] && [ "$IS_DUP" != "True" ]; then
  fail "Scenario 2 — got 200 but duplicate:true missing (new code not deployed?): $RESP2"
else
  fail "Scenario 2 — unexpected status=$HTTP2 duplicate=$IS_DUP returnedId=${RETURNED_ID:0:8}: $RESP2"
fi

# ── Scenario 3: different content same version → conflict ────────────────────
log "Scenario 3: different content same version → expect 409 artifact_version_conflict"
PACK2=$(pack_artifact "$WORK_DIR/input-v2" "$WORK_DIR/v2.tar.gz")
UP3=$(presign_and_upload "$WORK_DIR/v2.tar.gz" "$PACK2")

HTTP3=$(complete_artifact "$UP3" 2>"$WORK_DIR/resp3.json")
RESP3=$(cat "$WORK_DIR/resp3.json")

if [ "$HTTP3" = "409" ] && echo "$RESP3" | grep -q "artifact_version_conflict"; then
  pass "Scenario 3 — conflict: 409 artifact_version_conflict as expected"
elif [ "$HTTP3" = "200" ]; then
  fail "Scenario 3 — got 200 (no conflict enforced; new code not deployed?): $RESP3"
else
  fail "Scenario 3 — unexpected status=$HTTP3: $RESP3"
fi

# ── Scenario 4: ?supersede=true bypasses conflict check ─────────────────────
log "Scenario 4: different content + ?supersede=true → expect 200 new artifact ID"
UP4=$(presign_and_upload "$WORK_DIR/v2.tar.gz" "$PACK2")
ARTIFACT_ID_4=$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['artifactId'])" "$UP4")

HTTP4=$(complete_artifact "$UP4" supersede 2>"$WORK_DIR/resp4.json")
RESP4=$(cat "$WORK_DIR/resp4.json")
RETURNED_ID4=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('artifactId',''))" "$RESP4" 2>/dev/null || echo "")
IS_DUP4=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('duplicate',False))" "$RESP4" 2>/dev/null || echo "False")

if [ "$HTTP4" = "200" ] && [ "$RETURNED_ID4" = "$ARTIFACT_ID_4" ] && [ "$IS_DUP4" != "True" ]; then
  pass "Scenario 4 — supersede: 200 new artifact id=${ARTIFACT_ID_4:0:8}..."
elif [ "$HTTP4" = "409" ]; then
  fail "Scenario 4 — got 409 (supersede flag not respected; new code not deployed?): $RESP4"
else
  fail "Scenario 4 — unexpected status=$HTTP4 returnedId=${RETURNED_ID4:0:8} duplicate=$IS_DUP4: $RESP4"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
log "Results: $PASS_COUNT passed, $FAIL_COUNT failed"
log "Artifact name: $ARTIFACT_NAME  version: $ARTIFACT_VERSION"
log "Baseline ID (scenario 1): $ARTIFACT_ID_1"
log "Supersede ID (scenario 4): $ARTIFACT_ID_4"

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
