#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
BASE_URL=${BASE_URL:-https://localhost:8080}
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
INSECURE=${INSECURE:-0}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}
AUTH_TOKEN=${AUTH_TOKEN:-}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
TEST_PREFIX=${TEST_PREFIX:-trust-test}
SIGNING_KEY=${SIGNING_KEY:-}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}
SIGNATURE_TYPE=${SIGNATURE_TYPE:-ed25519}
EXPECT_UNSIGNED_RESULT=${EXPECT_UNSIGNED_RESULT:-reject}
EXPECT_SIGNED_RESULT=${EXPECT_SIGNED_RESULT:-accept}
RUN_WRONG_KEY_TEST=${RUN_WRONG_KEY_TEST:-0}
EXPECT_WRONG_KEY_RESULT=${EXPECT_WRONG_KEY_RESULT:-reject}

TMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/parcel-trust-test-XXXXXX")
trap 'rm -rf "$TMP_ROOT"' EXIT

usage() {
  cat <<USAGE
Usage:
  AUTH_EMAIL=admin@example.com AUTH_PASSWORD=secret \
  BASE_URL=https://app.example.com \
  ./scripts/test-artifact-trust.sh

Optional:
  AUTH_TOKEN=<jwt>                       Reuse existing auth token instead of logging in
  CA_CERT_PATH=/path/to/ca.crt           Custom CA for private TLS
  INSECURE=1                             Skip TLS verification
  CURL_RESOLVE_HOSTS='host:443:1.2.3.4'  Extra curl --resolve entries
  SIGNING_KEY=/path/to/ed25519.key       Override signing key for signed test
  SIGNING_KEY_ID=sha256:...              Override signing key ID
  EXPECT_UNSIGNED_RESULT=reject|accept   Default: reject
  EXPECT_SIGNED_RESULT=accept|reject     Default: accept
  RUN_WRONG_KEY_TEST=1                   Also test a signature from an untrusted key
USAGE
}

if [ -z "$AUTH_TOKEN" ] && { [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; }; then
  usage >&2
  exit 1
fi

CURL_OPTS=()
if [[ "$BASE_URL" == https:* ]]; then
  if [ -n "$CA_CERT_PATH" ] && [ -f "$CA_CERT_PATH" ]; then
    CURL_OPTS+=(--cacert "$CA_CERT_PATH")
  elif [ "$INSECURE" = "1" ]; then
    CURL_OPTS+=(-k)
  fi
fi
if [ -n "${CURL_RESOLVE_HOSTS:-}" ]; then
  IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
  for entry in "${resolve_entries[@]}"; do
    entry=$(echo "$entry" | xargs)
    if [ -n "$entry" ]; then
      CURL_OPTS+=(--resolve "$entry")
    fi
  done
fi

json_field() {
  python3 - <<'PY' "$1" "$2"
import json, sys
obj = json.loads(sys.argv[1])
print(obj.get(sys.argv[2], ""))
PY
}

login() {
  if [ -n "$AUTH_TOKEN" ]; then
    return
  fi
  local payload
  payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
  local resp
  resp=$(curl -sS --fail "${CURL_OPTS[@]}" \
    -H 'Content-Type: application/json' \
    -X POST "$BASE_URL/api/v1/auth/login" \
    -d "$payload")
  AUTH_TOKEN=$(json_field "$resp" token)
  if [ -z "$AUTH_TOKEN" ]; then
    echo "Failed to obtain auth token" >&2
    exit 1
  fi
}

get_policy() {
  local body_file status
  body_file="$TMP_ROOT/policy.json"
  status=$(curl -sS "${CURL_OPTS[@]}" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -o "$body_file" -w '%{http_code}' \
    "$BASE_URL/api/v1/artifact-trust/policy" || true)
  if [ "$status" = "200" ]; then
    echo "[policy] $(tr -d '\n' < "$body_file")"
  else
    echo "[policy] unavailable (HTTP $status)"
  fi
}

prepare_signing_key() {
  if [ -n "$SIGNING_KEY" ] && [ -n "$SIGNING_KEY_ID" ]; then
    return
  fi
  # shellcheck disable=SC1091
  source "$BASE_DIR/scripts/ensure-signing-key.sh"
  SIGNING_KEY=${SIGNING_KEY:-$SIGNING_KEY}
  SIGNING_KEY_ID=${SIGNING_KEY_ID:-$SIGNING_KEY_ID}
  if [ -z "$SIGNING_KEY" ] || [ -z "$SIGNING_KEY_ID" ]; then
    echo "Signing key bootstrap failed" >&2
    exit 1
  fi
}

pack_artifact() {
  local name=$1
  local version=$2
  local input_dir=$3
  local signing_key=${4:-}
  local signing_key_id=${5:-}
  local signature_out=""
  local out_path="$TMP_ROOT/${name}-${version}.tar.gz"
  if [ -n "$signing_key" ]; then
    signature_out="$TMP_ROOT/${name}-${version}.sig"
  fi

  local args=(
    --name "$name"
    --version "$version"
    --type "$ARTIFACT_TYPE"
    --input-dir "$input_dir"
    --out "$out_path"
  )
  if [ -n "$signing_key" ]; then
    args+=(--signing-key "$signing_key")
  fi
  if [ -n "$signature_out" ]; then
    args+=(--signature-out "$signature_out")
  fi
  if [ -n "$signing_key_id" ]; then
    args+=(--signing-key-id "$signing_key_id")
  fi

  python3 "$BASE_DIR/scripts/artifact-pack.py" "${args[@]}"
}

upload_artifact() {
  local name=$1
  local version=$2
  local packed_json=$3
  local label=$4
  local expected=$5

  local artifact_path signature signature_key_id signature_type
  artifact_path=$(json_field "$packed_json" artifactPath)
  signature=$(json_field "$packed_json" signature)
  signature_key_id=$(json_field "$packed_json" signatureKeyId)
  signature_type=$(json_field "$packed_json" signatureAlg)
  if [ -z "$signature_type" ]; then
    signature_type=$SIGNATURE_TYPE
  fi

  local body_file="$TMP_ROOT/${label}.response"
  local curl_args=(
    -sS
    "${CURL_OPTS[@]}"
    -H "Authorization: Bearer $AUTH_TOKEN"
    -o "$body_file"
    -w '%{http_code}'
    -X POST "$BASE_URL/api/v1/artifacts/upload"
    -F "name=$name"
    -F "version=$version"
    -F "type=$ARTIFACT_TYPE"
    -F "file=@$artifact_path"
  )
  if [ -n "$signature" ]; then
    curl_args+=(-F "signature=$signature")
    curl_args+=(-F "signatureKeyId=$signature_key_id")
    curl_args+=(-F "signatureType=$signature_type")
  fi

  local status
  status=$(curl "${curl_args[@]}" || true)
  local ok=0
  if [ "$expected" = "accept" ] && [[ "$status" =~ ^2 ]]; then
    ok=1
  fi
  if [ "$expected" = "reject" ] && ! [[ "$status" =~ ^2 ]]; then
    ok=1
  fi

  if [ "$ok" -eq 1 ]; then
    echo "[$label] PASS expected=$expected http=$status"
  else
    echo "[$label] FAIL expected=$expected http=$status" >&2
    echo "[$label] response: $(cat "$body_file")" >&2
    return 1
  fi

  if [ -s "$body_file" ]; then
    echo "[$label] response: $(tr -d '\n' < "$body_file")"
  fi
}

make_input_dir() {
  local dir=$1
  local text=$2
  mkdir -p "$dir"
  printf '%s\n' "$text" > "$dir/readme.txt"
}

login
get_policy

stamp=$(date +%Y%m%d%H%M%S)

unsigned_name="${TEST_PREFIX}-unsigned"
unsigned_version="0.0.$stamp"
unsigned_input="$TMP_ROOT/unsigned"
make_input_dir "$unsigned_input" "unsigned trust test $stamp"
unsigned_json=$(pack_artifact "$unsigned_name" "$unsigned_version" "$unsigned_input")
upload_artifact "$unsigned_name" "$unsigned_version" "$unsigned_json" unsigned "$EXPECT_UNSIGNED_RESULT"

prepare_signing_key
signed_name="${TEST_PREFIX}-signed"
signed_version="0.0.$stamp"
signed_input="$TMP_ROOT/signed"
make_input_dir "$signed_input" "signed trust test $stamp"
signed_json=$(pack_artifact "$signed_name" "$signed_version" "$signed_input" "$SIGNING_KEY" "$SIGNING_KEY_ID")
upload_artifact "$signed_name" "$signed_version" "$signed_json" signed "$EXPECT_SIGNED_RESULT"

if [ "$RUN_WRONG_KEY_TEST" = "1" ]; then
  wrong_dir="$TMP_ROOT/wrong-key"
  mkdir -p "$wrong_dir"
  bash "$BASE_DIR/scripts/generate-signing-key.sh" "$wrong_dir" >/dev/null
  wrong_name="${TEST_PREFIX}-wrong-key"
  wrong_version="0.0.$stamp"
  wrong_input="$TMP_ROOT/wrong-input"
  make_input_dir "$wrong_input" "wrong key trust test $stamp"
  wrong_json=$(pack_artifact "$wrong_name" "$wrong_version" "$wrong_input" "$wrong_dir/ed25519.key" "$(cat "$wrong_dir/ed25519.keyid")")
  upload_artifact "$wrong_name" "$wrong_version" "$wrong_json" wrong_key "$EXPECT_WRONG_KEY_RESULT"
fi

echo "Artifact trust test complete."
