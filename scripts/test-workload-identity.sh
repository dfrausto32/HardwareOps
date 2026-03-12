#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-https://localhost:8080}
AUTH_EMAIL=${AUTH_EMAIL:-}
AUTH_PASSWORD=${AUTH_PASSWORD:-}
AUTH_TOKEN=${AUTH_TOKEN:-}
CA_CERT_PATH=${CA_CERT_PATH:-}
CURL_RESOLVE_HOSTS=${CURL_RESOLVE_HOSTS:-}
INSECURE=${INSECURE:-0}
EXPECT_PROVIDER=${EXPECT_PROVIDER:-}
CI_WORKLOAD_IDENTITY_PROVIDER=${CI_WORKLOAD_IDENTITY_PROVIDER:-}
CI_WORKLOAD_IDENTITY_TOKEN=${CI_WORKLOAD_IDENTITY_TOKEN:-}
REQUESTED_SCOPES=${REQUESTED_SCOPES:-artifact.publish}
TEST_PUBLISH_SCOPE=${TEST_PUBLISH_SCOPE:-1}

curl_args=()
if [ "$INSECURE" = "1" ]; then
  curl_args+=(-k)
elif [ -n "$CA_CERT_PATH" ]; then
  curl_args+=(--cacert "$CA_CERT_PATH")
fi

if [ -n "$CURL_RESOLVE_HOSTS" ]; then
  IFS=',' read -r -a resolve_entries <<<"$CURL_RESOLVE_HOSTS"
  for entry in "${resolve_entries[@]}"; do
    entry=$(printf '%s' "$entry" | xargs)
    [ -n "$entry" ] && curl_args+=(--resolve "$entry")
  done
fi

api_call() {
  curl -fsS "${curl_args[@]}" "$@"
}

login() {
  if [ -n "$AUTH_TOKEN" ]; then
    printf '%s' "$AUTH_TOKEN"
    return
  fi
  if [ -z "$AUTH_EMAIL" ] || [ -z "$AUTH_PASSWORD" ]; then
    echo "AUTH_TOKEN or AUTH_EMAIL/AUTH_PASSWORD is required." >&2
    exit 1
  fi
  local payload
  payload=$(python3 - <<'PY' "$AUTH_EMAIL" "$AUTH_PASSWORD"
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)
  api_call -H 'Content-Type: application/json' \
    -d "$payload" \
    "$BASE_URL/api/v1/auth/login" | python3 -c 'import json, sys; print(json.load(sys.stdin)["token"])'
}

token=$(login)

echo "Checking workload identity status..."
status_json=$(api_call -H "Authorization: Bearer $token" "$BASE_URL/api/v1/auth/workload-identity/status")
printf '%s\n' "$status_json" | python3 -c '
import json, sys
expected = sys.argv[1].strip()
data = json.load(sys.stdin)
providers = [p.get("name", "") for p in data.get("providers", [])]
print("enabled={} providers={}".format(data.get("enabled", False), providers))
if expected and expected not in providers:
    raise SystemExit(f"expected provider {expected!r}, got {providers!r}")
' "$EXPECT_PROVIDER"

if [ -z "$CI_WORKLOAD_IDENTITY_TOKEN" ] || [ -z "$CI_WORKLOAD_IDENTITY_PROVIDER" ]; then
  echo "Skipping exchange test; set CI_WORKLOAD_IDENTITY_PROVIDER and CI_WORKLOAD_IDENTITY_TOKEN to exercise token exchange."
  exit 0
fi

echo "Exchanging workload identity token..."
exchange_payload=$(python3 - <<'PY' "$CI_WORKLOAD_IDENTITY_PROVIDER" "$CI_WORKLOAD_IDENTITY_TOKEN" "$REQUESTED_SCOPES"
import json, sys
scopes=[s.strip() for s in sys.argv[3].split(",") if s.strip()]
print(json.dumps({
    "provider": sys.argv[1],
    "idToken": sys.argv[2],
    "scopes": scopes,
}))
PY
)
exchange_json=$(api_call -H 'Content-Type: application/json' \
  -d "$exchange_payload" \
  "$BASE_URL/api/v1/auth/workload-identity/exchange")
workload_token=$(printf '%s\n' "$exchange_json" | python3 -c 'import json, sys; data = json.load(sys.stdin); print(data["token"])')
printf '%s\n' "$exchange_json" | python3 -c '
import json, sys
data = json.load(sys.stdin)
print("provider={} subject={} expiresAt={}".format(data.get("provider"), data.get("subject"), data.get("expiresAt")))
'

if [ "$TEST_PUBLISH_SCOPE" != "1" ]; then
  exit 0
fi

echo "Verifying artifact.publish scope via presign-upload..."
presign_payload='{"filename":"workload-identity-smoke.tar.gz","contentType":"application/gzip","expiresSeconds":60}'
api_call -H "Authorization: Bearer $workload_token" \
  -H 'Content-Type: application/json' \
  -d "$presign_payload" \
  "$BASE_URL/api/v1/artifacts/presign-upload" | python3 -c '
import json, sys
data = json.load(sys.stdin)
print("artifactId={} objectKey={}".format(data.get("artifactId"), data.get("objectKey")))
'

echo "Workload identity smoke passed."
