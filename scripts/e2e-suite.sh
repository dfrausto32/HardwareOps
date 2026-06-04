#!/usr/bin/env bash
# e2e-suite.sh — Full end-to-end test suite for Parcel.
#
# Starts a regional control-plane + global-plane + demo agents on a single
# machine, runs every E2E scenario, then tears everything down.
#
# Requirements:
#   - Docker (for Postgres + MinIO infra)
#   - Go 1.25+ in PATH
#   - Python 3 in PATH
#
# Usage:
#   ./scripts/e2e-suite.sh
#
# Override any default with env vars:
#   E2E_SKIP_BUILD=1   ./scripts/e2e-suite.sh   # skip binary rebuild
#   E2E_KEEP_INFRA=1   ./scripts/e2e-suite.sh   # leave docker compose running after
#   E2E_ONLY=artifact  ./scripts/e2e-suite.sh   # run only the named scenario

set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# ── Configuration ─────────────────────────────────────────────────────────────
E2E_CP_PORT=${E2E_CP_PORT:-8082}
E2E_GP_PORT=${E2E_GP_PORT:-8092}
E2E_PG_PORT=${E2E_PG_PORT:-5433}
E2E_MINIO_PORT=${E2E_MINIO_PORT:-9010}
E2E_ADMIN_EMAIL=${E2E_ADMIN_EMAIL:-admin@e2e.test}
E2E_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD:-e2e-test-password!}
E2E_SKIP_BUILD=${E2E_SKIP_BUILD:-0}
E2E_KEEP_INFRA=${E2E_KEEP_INFRA:-0}
E2E_ONLY=${E2E_ONLY:-}
E2E_TIMEOUT_SECS=${E2E_TIMEOUT_SECS:-300}  # per-scenario timeout

# Control-plane cert manager requires a CA cert + key at boot. Locally these
# are created by hand (see docs/local-dev-wsl.md); in CI they won't exist, so
# Step 2.5 below generates an ephemeral CA (and TLS server cert) when missing.
# The control-plane runs with TLS + optional mTLS so the device agent can
# authenticate its check-in via its client certificate.
CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
CA_KEY_PATH=${CA_KEY_PATH:-$BASE_DIR/dev-ca.key}
TLS_CERT_PATH=${TLS_CERT_PATH:-$BASE_DIR/dev-server.crt}
TLS_KEY_PATH=${TLS_KEY_PATH:-$BASE_DIR/dev-server.key}

BASE_URL="https://localhost:${E2E_CP_PORT}"
GLOBAL_URL="http://localhost:${E2E_GP_PORT}"
CP_DB="postgres://parcel:parcel@localhost:${E2E_PG_PORT}/parcel_e2e?sslmode=disable"
GP_DB="postgres://parcel:parcel@localhost:${E2E_PG_PORT}/parcel_global_e2e?sslmode=disable"
S3_ENDPOINT="localhost:${E2E_MINIO_PORT}"
S3_ACCESS="miniotest"
S3_SECRET="miniotest"
COMPOSE_FILE="$BASE_DIR/deploy/compose/docker-compose.e2e.yml"

LOG_DIR="${LOG_DIR:-/tmp/parcel-e2e-$$}"
BIN_DIR="${BIN_DIR:-/tmp/parcel-e2e-bin-$$}"
mkdir -p "$LOG_DIR" "$BIN_DIR"

CP_PID=""
GP_PID=""
PASS=0
FAIL=0
declare -a FAILURES=()

# ── Utilities ─────────────────────────────────────────────────────────────────
log() { echo "[e2e] $*"; }
ok()  { echo "  ✓ $*"; }
err() { echo "  ✗ $*" >&2; }

wait_for_url() {
  local url="$1" label="${2:-$1}" timeout="${3:-60}"
  local ca_opt=()
  [[ "$url" == https:* ]] && ca_opt=(--cacert "$CA_CERT_PATH")
  log "Waiting for $label ..."
  local i=0
  until curl -sf "${ca_opt[@]}" "$url" > /dev/null 2>&1; do
    sleep 1
    i=$((i+1))
    if [ "$i" -ge "$timeout" ]; then
      err "Timed out waiting for $label after ${timeout}s"
      return 1
    fi
  done
  ok "$label is up (${i}s)"
}

get_token() {
  local url="$1" email="$2" password="$3"
  curl -sf "$url/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$password\"}" \
    | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print(d.get("token",""))'
}

run_scenario() {
  local name="$1"; shift
  if [ -n "$E2E_ONLY" ] && [ "$name" != "$E2E_ONLY" ]; then
    return 0
  fi
  echo ""
  log "Scenario: $name"
  local out rc=0
  out=$(timeout "$E2E_TIMEOUT_SECS" bash -c "$*" 2>&1) || rc=$?
  if [ "$rc" -eq 0 ]; then
    ok "PASS: $name"
    PASS=$((PASS+1))
  else
    err "FAIL: $name (exit $rc)"
    FAIL=$((FAIL+1))
    FAILURES+=("$name")
    echo "$out" | tee "$LOG_DIR/scenario-${name//[^a-zA-Z0-9-]/_}.log" | tail -20 | sed 's/^/    /'
  fi
}

cleanup() {
  local exit_code=$?
  echo ""
  log "Cleaning up ..."
  [ -n "$CP_PID" ] && { kill "$CP_PID" 2>/dev/null || true; wait "$CP_PID" 2>/dev/null || true; }
  [ -n "$GP_PID" ] && { kill "$GP_PID" 2>/dev/null || true; wait "$GP_PID" 2>/dev/null || true; }

  if [ "${E2E_KEEP_INFRA:-0}" != "1" ]; then
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
  fi

  echo ""
  log "Results: ${PASS} passed, ${FAIL} failed"
  if [ "${#FAILURES[@]}" -gt 0 ]; then
    err "Failed scenarios: ${FAILURES[*]}"
    log "Logs in: $LOG_DIR"
  fi

  if [ "${FAIL}" -gt 0 ]; then
    exit 1
  fi
  exit "$exit_code"
}
trap cleanup EXIT

# ── Step 1: Infrastructure ────────────────────────────────────────────────────
log "Starting docker infrastructure (postgres + minio) ..."
docker compose -f "$COMPOSE_FILE" up -d 2>&1 | tail -5

log "Waiting for infrastructure health checks ..."
until docker compose -f "$COMPOSE_FILE" exec -T postgres \
    pg_isready -U parcel -d parcel_e2e > /dev/null 2>&1; do sleep 1; done
ok "postgres ready"
until curl -sf "http://localhost:${E2E_MINIO_PORT}/minio/health/live" > /dev/null 2>&1; do sleep 1; done
ok "minio ready"

# ── Step 2: Build binaries ────────────────────────────────────────────────────
if [ "${E2E_SKIP_BUILD:-0}" != "1" ]; then
  log "Building control-plane and global-plane binaries ..."
  (cd "$BASE_DIR/control-plane" && \
    go build -o "$BIN_DIR/control-plane" ./cmd/control-plane && \
    go build -o "$BIN_DIR/global-plane" ./cmd/global-plane)
  ok "binaries built → $BIN_DIR"
fi

# ── Step 2.5: Dev CA ──────────────────────────────────────────────────────────
# The control-plane cert manager loads CA_CERT_PATH/CA_KEY_PATH on startup and
# exits if they are missing. Generate an ephemeral CA when one isn't present so
# the suite is self-contained (e.g. on CI runners).
if [ ! -f "$CA_CERT_PATH" ] || [ ! -f "$CA_KEY_PATH" ]; then
  log "Generating ephemeral dev CA ..."
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$CA_KEY_PATH" -out "$CA_CERT_PATH" \
    -days 365 -subj "/CN=Parcel E2E CA" > /dev/null 2>&1
  ok "dev CA generated → $CA_CERT_PATH"
fi

# TLS server cert for the control-plane, signed by the dev CA, with SANs for
# localhost/127.0.0.1 so curl and the agent can verify it.
if [ ! -f "$TLS_CERT_PATH" ] || [ ! -f "$TLS_KEY_PATH" ]; then
  log "Generating control-plane TLS server cert ..."
  server_csr=$(mktemp)
  server_ext=$(mktemp)
  cat > "$server_ext" <<'EOF_EXT'
[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF_EXT
  openssl req -new -newkey rsa:2048 -nodes \
    -keyout "$TLS_KEY_PATH" -out "$server_csr" -subj "/CN=localhost" > /dev/null 2>&1
  openssl x509 -req -in "$server_csr" \
    -CA "$CA_CERT_PATH" -CAkey "$CA_KEY_PATH" -CAcreateserial \
    -out "$TLS_CERT_PATH" -days 365 -extfile "$server_ext" -extensions v3_req > /dev/null 2>&1
  rm -f "$server_csr" "$server_ext"
  ok "TLS server cert generated → $TLS_CERT_PATH"
fi

# ── Step 3: Start control-plane ───────────────────────────────────────────────
log "Starting control-plane on :${E2E_CP_PORT} ..."
env \
  DATABASE_URL="$CP_DB" \
  HTTP_ADDR=":${E2E_CP_PORT}" \
  ENABLE_TLS=1 \
  TLS_CERT_PATH="$TLS_CERT_PATH" \
  TLS_KEY_PATH="$TLS_KEY_PATH" \
  TLS_CLIENT_CA_PATH="$CA_CERT_PATH" \
  AUTO_MIGRATE=1 \
  MAINTENANCE_MODE=0 \
  DISABLE_HTTP2=1 \
  AUTH_BOOTSTRAP_EMAIL="$E2E_ADMIN_EMAIL" \
  AUTH_BOOTSTRAP_PASSWORD="$E2E_ADMIN_PASSWORD" \
  S3_ENDPOINT="$S3_ENDPOINT" \
  S3_BUCKET="parcel-e2e" \
  S3_ACCESS_KEY="$S3_ACCESS" \
  S3_SECRET_KEY="$S3_SECRET" \
  S3_USE_SSL=0 \
  S3_REGION="us-east-1" \
  MIGRATIONS_DIR="$BASE_DIR/control-plane/migrations" \
  CA_CERT_PATH="$CA_CERT_PATH" \
  CA_KEY_PATH="$CA_KEY_PATH" \
  DEVICE_IDENTITY_MODE=audit \
  AUTH_MODE=local \
  AUTH_JWT_SECRET="e2e-jwt-secret-change-in-prod!" \
  WEBHOOK_ENCRYPTION_KEY="$(python3 -c "import base64; print(base64.b64encode(b'e2e-webhook-key-fixed-32-bytes!!').decode())")" \
  "$BIN_DIR/control-plane" \
  > "$LOG_DIR/control-plane.log" 2>&1 &
CP_PID=$!

wait_for_url "$BASE_URL/healthz" "control-plane" 60

# ── Step 4: Start global-plane ────────────────────────────────────────────────
log "Starting global-plane on :${E2E_GP_PORT} ..."
# Generate a deterministic encryption key for E2E (base64 of 32 bytes)
GP_ENC_KEY=$(python3 -c "import base64; print(base64.b64encode(b'e2e-enc-key-fixed-32-bytes-xxxx!').decode())")

env \
  GLOBAL_DATABASE_URL="$GP_DB" \
  GLOBAL_HTTP_ADDR=":${E2E_GP_PORT}" \
  AUTH_JWT_SECRET="e2e-jwt-secret-change-in-prod!" \
  GLOBAL_TOKEN_ENCRYPTION_KEY="$GP_ENC_KEY" \
  GLOBAL_MIGRATIONS_DIR="$BASE_DIR/control-plane/migrations/global" \
  GLOBAL_SYNC_DEFAULT_INTERVAL=10 \
  GLOBAL_MINIO_ENDPOINT="$S3_ENDPOINT" \
  GLOBAL_MINIO_ACCESS_KEY="$S3_ACCESS" \
  GLOBAL_MINIO_SECRET_KEY="$S3_SECRET" \
  GLOBAL_MINIO_BUCKET="global-e2e-artifacts" \
  GLOBAL_MINIO_USE_TLS=0 \
  AUTH_BOOTSTRAP_EMAIL="$E2E_ADMIN_EMAIL" \
  AUTH_BOOTSTRAP_PASSWORD="$E2E_ADMIN_PASSWORD" \
  AUTH_ENABLED=1 \
  ENABLE_TLS=0 \
  "$BIN_DIR/global-plane" \
  > "$LOG_DIR/global-plane.log" 2>&1 &
GP_PID=$!

wait_for_url "$GLOBAL_URL/healthz" "global-plane" 60

echo ""
log "All services ready. Running scenarios ..."
echo ""

# ── Scenario 1: Artifact flow ─────────────────────────────────────────────────
run_scenario "artifact-flow" \
  "BASE_URL=$BASE_URL \
   AUTH_EMAIL=$E2E_ADMIN_EMAIL \
   AUTH_PASSWORD=$E2E_ADMIN_PASSWORD \
   INSECURE=0 \
   CA_CERT_PATH=$CA_CERT_PATH \
   SIGN_ARTIFACTS=0 \
   REQUIRE_ARTIFACT_SIGNATURE=0 \
   GENERATE_ARTIFACT=1 \
   ARTIFACT_NAME=e2e-app \
   ARTIFACT_VERSION=1.0.0 \
   ARTIFACT_TYPE=app_bundle \
   STATE_PATH=$LOG_DIR/agent-state-1.json \
   ARTIFACT_ROOT=$LOG_DIR/agent-data-1 \
   CLEANUP=0 \
   '$BASE_DIR/scripts/artifact-e2e.sh'"

# ── Scenario 2: Pending enrollment ───────────────────────────────────────────
run_scenario "pending-enrollment" \
  "BASE_URL=$BASE_URL \
   AUTH_EMAIL=$E2E_ADMIN_EMAIL \
   AUTH_PASSWORD=$E2E_ADMIN_PASSWORD \
   INSECURE=0 \
   CA_CERT_PATH=$CA_CERT_PATH \
   PROFILE_NAME=e2e-profile \
   PROFILE_REQUIRE_APPROVAL=1 \
   AUTO_APPROVE_API=1 \
   CHECKIN_AFTER_CLAIM=0 \
   '$BASE_DIR/scripts/test-pending-enrollment.sh'"

# ── Scenario 3: CI feedback loop ─────────────────────────────────────────────
run_scenario "ci-feedback-loop" \
  "BASE_URL=$BASE_URL \
   AUTH_EMAIL=$E2E_ADMIN_EMAIL \
   AUTH_PASSWORD=$E2E_ADMIN_PASSWORD \
   INSECURE=0 \
   CA_CERT_PATH=$CA_CERT_PATH \
   WEBHOOK_PORT=9877 \
   '$BASE_DIR/scripts/test-ci-feedback-loop.sh'"

# ── Scenario 4: Artifact trust ────────────────────────────────────────────────
run_scenario "artifact-trust" \
  "BASE_URL=$BASE_URL \
   AUTH_EMAIL=$E2E_ADMIN_EMAIL \
   AUTH_PASSWORD=$E2E_ADMIN_PASSWORD \
   INSECURE=0 \
   CA_CERT_PATH=$CA_CERT_PATH \
   EXPECT_UNSIGNED_RESULT=accept \
   EXPECT_SIGNED_RESULT=accept \
   RUN_WRONG_KEY_TEST=0 \
   '$BASE_DIR/scripts/test-artifact-trust.sh'"

# ── Scenario 5: Multi-agent enrollment ───────────────────────────────────────
run_scenario "multi-agent" '
  set -euo pipefail
  log() { echo "[multi-agent] $*"; }

  # Get auth token
  AUTH_TOKEN=$(curl -sf --cacert "'"$CA_CERT_PATH"'" "'"$BASE_URL"'/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"'"$E2E_ADMIN_EMAIL"'\",\"password\":\"'"$E2E_ADMIN_PASSWORD"'\"}" \
    | python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"token\"])")

  # Create enrollment token (legacy mode)
  TOKEN_RESP=$(curl -sf --cacert "'"$CA_CERT_PATH"'" -X POST "'"$BASE_URL"'/api/v1/enrollments" \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"expiresInSec\":3600,\"maxUses\":5}")
  TOKEN=$(python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"token\"])" <<<"$TOKEN_RESP")

  # Build the agent once. Three concurrent "go run" invocations race on the
  # shared Go build cache and can fail; a prebuilt binary is safe to run in
  # parallel.
  AGENT_BIN="'"$LOG_DIR"'/agent-e2e-bin"
  ( cd "'"$BASE_DIR"'/agent" && go build -o "$AGENT_BIN" ./cmd/agent )

  log "Enrolling and checking in 3 agents ..."
  # Each agent gets a unique HARDWARE_IDENTITY so they appear as distinct devices
  # to the control-plane's device identity conflict detection. Without this, all
  # three would derive the same hardware ID from /etc/machine-id, which would
  # trigger conflict warnings in audit mode and hard rejections in enforce mode.
  # LICENSE_ENFORCE is not set (defaults false) so the device cap is not checked.
  PIDS=()
  for i in 1 2 3; do
    STATE_PATH="'"$LOG_DIR"'/multi-agent-state-${i}.json"
    ART_ROOT="'"$LOG_DIR"'/multi-agent-data-${i}"
    mkdir -p "$ART_ROOT"
    (
      ENROLLMENT_TOKEN="$TOKEN" \
      CONTROL_PLANE_URL="'"$BASE_URL"'" \
      STATE_PATH="$STATE_PATH" \
      ARTIFACT_ROOT="$ART_ROOT" \
      CONTROL_PLANE_CA_CERT_PATH="'"$CA_CERT_PATH"'" \
      HARDWARE_IDENTITY="e2e-agent-${i}-$(hostname)" \
      "$AGENT_BIN" -once 2>&1
    ) >> "'"$LOG_DIR"'/multi-agent-${i}.log" 2>&1 &
    PIDS+=($!)
  done

  # Wait for all agents
  FAIL_COUNT=0
  for pid in "${PIDS[@]}"; do
    wait "$pid" || FAIL_COUNT=$((FAIL_COUNT+1))
  done
  [ "$FAIL_COUNT" -eq 0 ] || { echo "'"$FAIL_COUNT"' agent(s) failed"; exit 1; }
  log "All 3 agents enrolled and checked in"
'

# ── Scenario 6: Global plane sync ────────────────────────────────────────────
run_scenario "global-plane-sync" '
  set -euo pipefail
  log() { echo "[global-sync] $*"; }

  # Auth tokens for regional and global planes
  CP_TOKEN=$(curl -sf --cacert "'"$CA_CERT_PATH"'" "'"$BASE_URL"'/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"'"$E2E_ADMIN_EMAIL"'\",\"password\":\"'"$E2E_ADMIN_PASSWORD"'\"}" \
    | python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"token\"])")

  GP_TOKEN=$(curl -sf "'"$GLOBAL_URL"'/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"'"$E2E_ADMIN_EMAIL"'\",\"password\":\"'"$E2E_ADMIN_PASSWORD"'\"}" \
    | python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"token\"])")

  # Create a federation.push service token on the regional plane
  FED_TOKEN=$(curl -sf --cacert "'"$CA_CERT_PATH"'" -X POST "'"$BASE_URL"'/api/v1/auth/service-tokens" \
    -H "Authorization: Bearer $CP_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-federation\",\"scopes\":[\"federation.push\"],\"expiresInSeconds\":3600}" \
    | python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"token\"])")

  # The regional plane now serves TLS with a self-signed dev CA, so the global
  # plane must be told that CA (tlsCaPem) to trust it when polling.
  TLS_CA_PEM=$(python3 -c "import json; print(json.dumps(open(\"'"$CA_CERT_PATH"'\").read()))")

  log "Registering regional plane with global plane ..."
  curl -sf -X POST "'"$GLOBAL_URL"'/api/v1/planes" \
    -H "Authorization: Bearer $GP_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-regional\",\"baseUrl\":\"'"$BASE_URL"'\",\"serviceToken\":\"$FED_TOKEN\",\"syncIntervalSeconds\":5,\"tlsCaPem\":${TLS_CA_PEM}}" \
    > /dev/null

  # Wait for first sync cycle
  log "Waiting for global-plane to sync devices from regional plane ..."
  MAX_WAIT=30
  i=0
  while true; do
    DEVICE_COUNT=$(curl -sf "'"$GLOBAL_URL"'/api/v1/devices" \
      -H "Authorization: Bearer $GP_TOKEN" \
      | python3 -c "import json,sys; print(len(json.loads(sys.stdin.read())))")
    [ "$DEVICE_COUNT" -gt 0 ] && break
    i=$((i+1))
    [ "$i" -lt "$MAX_WAIT" ] || { echo "No devices synced to global plane after ${MAX_WAIT}s"; exit 1; }
    sleep 1
  done
  log "Devices visible in global plane: $DEVICE_COUNT"

  # Set a global desired state and verify it fans out
  log "Creating global group and setting desired state ..."
  GROUP_RESP=$(curl -sf -X POST "'"$GLOBAL_URL"'/api/v1/groups" \
    -H "Authorization: Bearer $GP_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-global-group\",\"selectorJson\":{}}")
  GROUP_ID=$(python3 -c "import json,sys; print(json.loads(sys.stdin.read())[\"groupId\"])" <<<"$GROUP_RESP")

  curl -sf -X PUT "'"$GLOBAL_URL"'/api/v1/groups/${GROUP_ID}/desired-state" \
    -H "Authorization: Bearer $GP_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"desiredVersion\":\"1.0.0\",\"checkinInterval\":60}" \
    > /dev/null

  log "Global desired state set for group $GROUP_ID"

  # Verify the policy appears on the regional plane
  sleep 8  # give the E4 reconciler time to push
  POLICY_COUNT=$(curl -sf --cacert "'"$CA_CERT_PATH"'" "'"$BASE_URL"'/api/v1/federation/policies" \
    -H "Authorization: Bearer $FED_TOKEN" \
    | python3 -c "import json,sys; data=json.loads(sys.stdin.read()); print(len(data) if isinstance(data,list) else len(data.get(\"items\",[])))")
  [ "$POLICY_COUNT" -gt 0 ] || { echo "Global policy did not fan out to regional plane"; exit 1; }
  log "Policy fan-out confirmed: $POLICY_COUNT policy/policies on regional plane"
'

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
log "Suite complete."
