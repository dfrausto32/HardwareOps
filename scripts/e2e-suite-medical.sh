#!/usr/bin/env bash
# e2e-suite-medical.sh — Medical/IoMT E2E test suite for Parcel (Phase G / QM2).
#
# Boots a DEPLOYMENT_PROFILE=medical control-plane (full hardened profile:
# signed medical license, require_verified artifact trust, device identity
# enforce) against real Postgres + MinIO, then runs the QM2 scenarios:
#
#   0. license-variant     — medical profile + standard license must fail startup
#   1. change-record       — Class C artifact blocked (422) until change record approved
#   2. break-glass         — Class B bypass via ?bypass_change_approval=true is audited
#   3. vex-generation      — vuln scan → VEX document appears and is downloadable  (needs trivy)
#   4. hipaa-export        — device apply-result → PHI-touched HIPAA export rows
#   5. qms-package         — 422 prerequisite checklist; full 8-file ZIP            (ZIP needs trivy)
#
# Scenarios 3 and the ZIP half of 5 need `trivy` in PATH (SBOM + vuln scan);
# they are reported SKIP when it is missing.
#
# Requirements: Docker, Go 1.25+, Python 3, openssl, unzip.
#
# Usage:
#   ./scripts/e2e-suite-medical.sh
#   E2E_SKIP_BUILD=1 ./scripts/e2e-suite-medical.sh   # skip binary rebuild
#   E2E_KEEP_INFRA=1 ./scripts/e2e-suite-medical.sh   # leave docker compose running
#   E2E_ONLY=change-record ./scripts/e2e-suite-medical.sh

set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# ── Configuration ─────────────────────────────────────────────────────────────
MED_CP_PORT=${MED_CP_PORT:-8083}
E2E_PG_PORT=${E2E_PG_PORT:-5433}
E2E_MINIO_PORT=${E2E_MINIO_PORT:-9010}
E2E_ADMIN_EMAIL=${E2E_ADMIN_EMAIL:-admin@medical-e2e.test}
E2E_ADMIN_PASSWORD=${E2E_ADMIN_PASSWORD:-medical-e2e-Passw0rd!}
E2E_SKIP_BUILD=${E2E_SKIP_BUILD:-0}
E2E_KEEP_INFRA=${E2E_KEEP_INFRA:-0}
E2E_ONLY=${E2E_ONLY:-}
E2E_TIMEOUT_SECS=${E2E_TIMEOUT_SECS:-300}

CA_CERT_PATH=${CA_CERT_PATH:-$BASE_DIR/dev-ca.crt}
CA_KEY_PATH=${CA_KEY_PATH:-$BASE_DIR/dev-ca.key}
TLS_CERT_PATH=${TLS_CERT_PATH:-$BASE_DIR/dev-server.crt}
TLS_KEY_PATH=${TLS_KEY_PATH:-$BASE_DIR/dev-server.key}

BASE_URL="https://localhost:${MED_CP_PORT}"
CP_DB="postgres://parcel:parcel@localhost:${E2E_PG_PORT}/parcel_medical_e2e?sslmode=disable"
S3_ENDPOINT="localhost:${E2E_MINIO_PORT}"
S3_ACCESS="miniotest"
S3_SECRET="miniotest"
COMPOSE_FILE="$BASE_DIR/deploy/compose/docker-compose.e2e.yml"

LOG_DIR="${LOG_DIR:-/tmp/parcel-medical-e2e-$$}"
BIN_DIR="${BIN_DIR:-/tmp/parcel-medical-e2e-bin-$$}"
WORK_DIR="$LOG_DIR/work"
mkdir -p "$LOG_DIR" "$BIN_DIR" "$WORK_DIR"

JWT_SECRET="medical-e2e-jwt-secret-0123456789abcdef"

CP_PID=""
PASS=0
FAIL=0
SKIP=0
declare -a FAILURES=()

HAVE_TRIVY=0
command -v trivy >/dev/null 2>&1 && HAVE_TRIVY=1

# ── Utilities ─────────────────────────────────────────────────────────────────
log() { echo "[medical-e2e] $*"; }
ok()  { echo "  ✓ $*"; }
err() { echo "  ✗ $*" >&2; }

jget() { python3 -c 'import json,sys; d=json.loads(sys.stdin.read());
for k in sys.argv[1].split("."):
    d = d[int(k)] if isinstance(d, list) else d[k]
print(d)' "$1"; }

wait_for_url() {
  local url="$1" label="${2:-$1}" timeout="${3:-60}"
  log "Waiting for $label ..."
  local i=0
  until curl -sf --cacert "$CA_CERT_PATH" "$url" > /dev/null 2>&1; do
    sleep 1
    i=$((i+1))
    if [ "$i" -ge "$timeout" ]; then
      err "Timed out waiting for $label after ${timeout}s"
      return 1
    fi
  done
  ok "$label is up (${i}s)"
}

api() { # api METHOD PATH [curl args...] — authenticated JSON call, prints body
  local method="$1" path="$2"; shift 2
  curl -sS --fail --cacert "$CA_CERT_PATH" -X "$method" "$BASE_URL$path" \
    -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" "$@"
}

api_code() { # api_code METHOD PATH [curl args...] — prints HTTP status only
  local method="$1" path="$2"; shift 2
  curl -sS -o "$LOG_DIR/last-body.json" -w '%{http_code}' --cacert "$CA_CERT_PATH" \
    -X "$method" "$BASE_URL$path" \
    -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" "$@"
}

# upload_artifact NAME VERSION — packs, signs, uploads; prints artifactId
upload_artifact() {
  local name="$1" version="$2"
  local input_dir="$WORK_DIR/input-$name-$version"
  mkdir -p "$input_dir"
  echo "medical e2e payload $name $version" > "$input_dir/payload.txt"
  # A pinned old dependency gives trivy real components/CVEs for the VEX scenario.
  printf 'urllib3==1.26.4\nrequests==2.25.1\n' > "$input_dir/requirements.txt"
  BASE_URL="$BASE_URL" CA_CERT_PATH="$CA_CERT_PATH" AUTH_TOKEN="$ADMIN_TOKEN" \
    ARTIFACT_NAME="$name" ARTIFACT_VERSION="$version" ARTIFACT_TYPE=app_bundle \
    INPUT_DIR="$input_dir" \
    SIGNING_KEY="$SIGNING_KEY" SIGNING_KEY_ID="$SIGNING_KEY_ID" \
    "$BASE_DIR/scripts/pack-upload-artifact.sh" | jget artifactId
}

# create_change_record ARTIFACT_ID CLASS — creates a draft change record
create_change_record() {
  api POST "/api/v1/medical/artifacts/$1/change-record" \
    -d "{\"safetyClass\":\"$2\",\"impactSummary\":\"medical e2e impact assessment\",\"riskControls\":\"medical e2e risk controls\"}" \
    > /dev/null
}

audit_count() { # audit_count ACTION — number of audit events with that action
  api GET "/api/v1/audit?action=$1&limit=100" \
    | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print(len(d["items"] if isinstance(d,dict) else d))'
}

# Scenarios run as functions in a subshell of this script (so they inherit all
# helpers and variables); per-step waits inside each scenario are bounded.
run_scenario() {
  local name="$1" fn="$2"
  if [ -n "$E2E_ONLY" ] && [ "$name" != "$E2E_ONLY" ]; then
    return 0
  fi
  echo ""
  log "Scenario: $name"
  local out rc=0
  out=$( ( set -euo pipefail; "$fn" ) 2>&1 ) || rc=$?
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

skip_scenario() {
  local name="$1" reason="$2"
  if [ -n "$E2E_ONLY" ] && [ "$name" != "$E2E_ONLY" ]; then
    return 0
  fi
  echo ""
  log "Scenario: $name"
  echo "  - SKIP: $reason"
  SKIP=$((SKIP+1))
}

cleanup() {
  local exit_code=$?
  echo ""
  log "Cleaning up ..."
  [ -n "$CP_PID" ] && { kill "$CP_PID" 2>/dev/null || true; wait "$CP_PID" 2>/dev/null || true; }
  if [ "${E2E_KEEP_INFRA:-0}" != "1" ]; then
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true
  fi
  echo ""
  log "Results: ${PASS} passed, ${FAIL} failed, ${SKIP} skipped"
  if [ "${#FAILURES[@]}" -gt 0 ]; then
    err "Failed scenarios: ${FAILURES[*]}"
    log "Logs in: $LOG_DIR"
  fi
  [ "${FAIL}" -gt 0 ] && exit 1
  exit "$exit_code"
}
trap cleanup EXIT

# ── Step 1: Infrastructure ────────────────────────────────────────────────────
log "Starting docker infrastructure (postgres + minio) ..."
docker compose -f "$COMPOSE_FILE" up -d 2>&1 | tail -5

until docker compose -f "$COMPOSE_FILE" exec -T postgres \
    pg_isready -U parcel -d parcel_e2e > /dev/null 2>&1; do sleep 1; done
ok "postgres ready"
until curl -sf "http://localhost:${E2E_MINIO_PORT}/minio/health/live" > /dev/null 2>&1; do sleep 1; done
ok "minio ready"

# Own database, so the standard e2e suite and this one can share the compose stack.
# Retried: the postgres entrypoint restarts the server once after running its
# init scripts, so the first connection attempts can land in that window.
for i in $(seq 1 30); do
  if docker compose -f "$COMPOSE_FILE" exec -T postgres \
       psql -U parcel -d parcel_e2e -tc "SELECT 1 FROM pg_database WHERE datname='parcel_medical_e2e'" 2>/dev/null \
     | grep -q 1; then
    break
  fi
  if docker compose -f "$COMPOSE_FILE" exec -T postgres \
       createdb -U parcel parcel_medical_e2e 2>/dev/null; then
    break
  fi
  sleep 1
done
docker compose -f "$COMPOSE_FILE" exec -T postgres \
  psql -U parcel -d parcel_medical_e2e -tc "SELECT 1" > /dev/null
ok "database parcel_medical_e2e ready"

# ── Step 2: Build ─────────────────────────────────────────────────────────────
if [ "${E2E_SKIP_BUILD:-0}" != "1" ]; then
  log "Building control-plane binary ..."
  (cd "$BASE_DIR/control-plane" && go build -o "$BIN_DIR/control-plane" ./cmd/control-plane)
  ok "binary built → $BIN_DIR"
fi

# ── Step 3: PKI, signing keys, licenses ───────────────────────────────────────
if [ ! -f "$CA_CERT_PATH" ] || [ ! -f "$CA_KEY_PATH" ]; then
  log "Generating ephemeral dev CA ..."
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$CA_KEY_PATH" -out "$CA_CERT_PATH" \
    -days 365 -subj "/CN=Parcel Medical E2E CA" > /dev/null 2>&1
fi
if [ ! -f "$TLS_CERT_PATH" ] || [ ! -f "$TLS_KEY_PATH" ]; then
  log "Generating control-plane TLS server cert ..."
  server_csr=$(mktemp); server_ext=$(mktemp)
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
fi

log "Generating artifact signing key + trusted key registry ..."
SIGNING_DIR="$WORK_DIR/signing"
source "$BASE_DIR/scripts/ensure-signing-key.sh"
TRUSTED_KEYS_FILE="$WORK_DIR/trusted-signing-keys.json"
"$BASE_DIR/scripts/build-trusted-signing-keys.sh" "$TRUSTED_KEYS_FILE" "$SIGNING_PUB" > /dev/null
ok "signing key $SIGNING_KEY_ID"

log "Generating license keypair + licenses (medical and standard) ..."
LICENSE_KEY="$WORK_DIR/license-ed25519.key"
LICENSE_PUB="$WORK_DIR/license-ed25519.pub"
openssl genpkey -algorithm Ed25519 -out "$LICENSE_KEY" > /dev/null 2>&1
openssl pkey -in "$LICENSE_KEY" -pubout -out "$LICENSE_PUB" > /dev/null 2>&1
MEDICAL_LICENSE="$WORK_DIR/license-medical.json"
STANDARD_LICENSE="$WORK_DIR/license-standard.json"
LICENSE_KEY="$LICENSE_KEY" OUT="$MEDICAL_LICENSE" ISSUED_TO="medical-e2e" MAX_DEVICES=100 \
  VARIANT=medical "$BASE_DIR/scripts/sign-license.sh" > /dev/null
LICENSE_KEY="$LICENSE_KEY" OUT="$STANDARD_LICENSE" ISSUED_TO="medical-e2e" MAX_DEVICES=100 \
  VARIANT=standard "$BASE_DIR/scripts/sign-license.sh" > /dev/null
ok "licenses signed"

# ── Step 4: Control-plane env ─────────────────────────────────────────────────
# Full hardened-profile environment; LICENSE_PATH is parameterised so scenario 0
# can boot once against the standard license and assert the startup failure.
#
# NOTE: cp_env execs. Always call it from a subshell (command substitution or
# `( cp_env ... ) &`) — backgrounding it directly would otherwise make $! the
# subshell PID instead of the binary, leaving an orphaned server on cleanup.
cp_env() { # cp_env LICENSE_PATH CMD...
  exec env \
    DATABASE_URL="$CP_DB" \
    HTTP_ADDR=":${MED_CP_PORT}" \
    ENABLE_TLS=1 \
    TLS_CERT_PATH="$TLS_CERT_PATH" \
    TLS_KEY_PATH="$TLS_KEY_PATH" \
    TLS_CLIENT_CA_PATH="$CA_CERT_PATH" \
    AUTO_MIGRATE=1 \
    MAINTENANCE_MODE=0 \
    DISABLE_HTTP2=1 \
    MIGRATIONS_DIR="$BASE_DIR/control-plane/migrations" \
    MEDICAL_MIGRATIONS_DIR="$BASE_DIR/control-plane/migrations/medical" \
    CA_CERT_PATH="$CA_CERT_PATH" \
    CA_KEY_PATH="$CA_KEY_PATH" \
    AUTH_MODE=local \
    AUTH_JWT_SECRET="$JWT_SECRET" \
    AUTH_BOOTSTRAP_EMAIL="$E2E_ADMIN_EMAIL" \
    AUTH_BOOTSTRAP_PASSWORD="$E2E_ADMIN_PASSWORD" \
    S3_ENDPOINT="$S3_ENDPOINT" \
    S3_BUCKET="parcel-medical-e2e" \
    S3_ACCESS_KEY="$S3_ACCESS" \
    S3_SECRET_KEY="$S3_SECRET" \
    S3_USE_SSL=0 \
    S3_REGION="us-east-1" \
    HARDENED_PROFILE=1 \
    DEPLOYMENT_PROFILE=medical \
    LICENSE_ENFORCE=1 \
    LICENSE_PATH="$1" \
    LICENSE_PUBLIC_KEY_PATH="$LICENSE_PUB" \
    DEVICE_IDENTITY_MODE=enforce \
    DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1 \
    DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1 \
    ARTIFACT_PULL_ALLOWED_HOSTS=localhost \
    ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0 \
    ARTIFACT_TRUST_VERIFICATION_MODE=require_verified \
    ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS="$SIGNING_KEY_ID" \
    ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1 \
    ARTIFACT_SIGNATURE_ENFORCE_INGEST=1 \
    TRUSTED_SIGNING_KEYS_FILE="$TRUSTED_KEYS_FILE" \
    WEBHOOK_ENCRYPTION_KEY="$(python3 -c "import base64; print(base64.b64encode(b'medical-e2e-webhook-32-bytes-ok!').decode())")" \
    SBOM_ENABLED="$HAVE_TRIVY" \
    VULN_ARTIFACT_SCANNER="$( [ "$HAVE_TRIVY" = "1" ] && echo trivy || echo disabled )" \
    "${@:2}"
}

# ── Scenario 0: license variant enforcement ───────────────────────────────────
scenario_license_variant() {
  local out rc=0
  out=$( ( cp_env "$STANDARD_LICENSE" timeout 20 "$BIN_DIR/control-plane" ) 2>&1 ) || rc=$?
  if [ "$rc" -eq 0 ] || [ "$rc" -eq 124 ]; then
    echo "control-plane unexpectedly started with a standard-variant license (rc=$rc)"
    echo "$out" | tail -5
    return 1
  fi
  echo "$out" | grep -qi "variant=medical" || {
    echo "startup failure did not mention variant=medical:"; echo "$out" | tail -5; return 1; }
  echo "standard license rejected at startup as expected"
}

run_scenario "license-variant" scenario_license_variant

# ── Step 5: Start the medical control-plane ───────────────────────────────────
# A stale server from an earlier run answering on the port would make every
# scenario silently test old code — fail fast instead.
if curl -sk "$BASE_URL/healthz" > /dev/null 2>&1; then
  err "something is already listening on $BASE_URL — kill the stale control-plane first"
  exit 1
fi

log "Starting medical control-plane on :${MED_CP_PORT} ..."
( cp_env "$MEDICAL_LICENSE" "$BIN_DIR/control-plane" ) > "$LOG_DIR/control-plane.log" 2>&1 &
CP_PID=$!

# Wait for healthz, and fail fast if the server process dies (e.g. bind error).
log "Waiting for medical control-plane ..."
i=0
until curl -sf --cacert "$CA_CERT_PATH" "$BASE_URL/healthz" > /dev/null 2>&1; do
  if ! kill -0 "$CP_PID" 2>/dev/null; then
    err "control-plane exited during startup:"
    tail -10 "$LOG_DIR/control-plane.log" | sed 's/^/    /' >&2
    CP_PID=""
    exit 1
  fi
  sleep 1
  i=$((i+1))
  [ "$i" -lt 60 ] || { err "Timed out waiting for control-plane after 60s"; exit 1; }
done
ok "medical control-plane is up (${i}s)"

# Retried: /healthz can come up before the bootstrap admin row is seeded.
ADMIN_TOKEN=""
for i in $(seq 1 30); do
  ADMIN_TOKEN=$(curl -sf --cacert "$CA_CERT_PATH" "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$E2E_ADMIN_EMAIL\",\"password\":\"$E2E_ADMIN_PASSWORD\"}" \
    | jget token 2>/dev/null) && [ -n "$ADMIN_TOKEN" ] && break
  sleep 1
done
[ -n "$ADMIN_TOKEN" ] || { err "admin login failed after 30s"; exit 1; }
ok "admin logged in"

# Sanity: medical API surface is active.
api GET /api/v1/medical/status > /dev/null
ok "medical API surface active"

echo ""
log "Running scenarios ..."

# ── Scenario 1: change record workflow ────────────────────────────────────────
scenario_change_record() {
  AID=$(upload_artifact med-app-a 1.0.0)
  echo "uploaded artifact $AID"
  create_change_record "$AID" ClassC
  GID=$(python3 -c 'import uuid; print(uuid.uuid4())')

  CODE=$(api_code PUT "/api/v1/desired-state/groups/$GID" \
    -d "{\"artifactId\":\"$AID\",\"desiredVersion\":\"1.0.0\"}")
  [ "$CODE" = "422" ] || { echo "expected 422 before approval, got $CODE: $(cat "$LOG_DIR/last-body.json")"; return 1; }
  echo "desired-state blocked (422) while change record is draft"

  api POST "/api/v1/medical/artifacts/$AID/change-record/submit" > /dev/null
  STATUS=$(api POST "/api/v1/medical/artifacts/$AID/change-record/approve" | jget status)
  [ "$STATUS" = "approved" ] || { echo "expected approved, got $STATUS"; return 1; }

  CODE=$(api_code PUT "/api/v1/desired-state/groups/$GID" \
    -d "{\"artifactId\":\"$AID\",\"desiredVersion\":\"1.0.0\"}")
  [ "$CODE" = "200" ] || { echo "expected 200 after approval, got $CODE: $(cat "$LOG_DIR/last-body.json")"; return 1; }
  echo "desired-state accepted after approval"

  N=$(audit_count change_record.approved)
  [ "$N" -ge 1 ] || { echo "no change_record.approved audit event"; return 1; }
  echo "change_record.approved audit event present"
  echo "$AID" > "$WORK_DIR/artifact-a.id"
}

# ── Scenario 2: break-glass override ──────────────────────────────────────────
scenario_break_glass() {
  BID=$(upload_artifact med-app-b 1.0.0)
  create_change_record "$BID" ClassB
  GID=$(python3 -c 'import uuid; print(uuid.uuid4())')

  CODE=$(api_code PUT "/api/v1/desired-state/groups/$GID?bypass_change_approval=true" \
    -d "{\"artifactId\":\"$BID\",\"desiredVersion\":\"1.0.0\"}")
  [ "$CODE" = "200" ] || { echo "expected 200 with bypass, got $CODE: $(cat "$LOG_DIR/last-body.json")"; return 1; }
  echo "deployment proceeded with bypass"

  N=$(audit_count change_record.bypassed)
  [ "$N" -ge 1 ] || { echo "no change_record.bypassed audit event"; return 1; }
  echo "change_record.bypassed audit event present"
}

# ── Scenario 3: VEX generation ────────────────────────────────────────────────
scenario_vex() {
  CID=$(upload_artifact med-app-c 1.0.0)
  echo "uploaded artifact $CID; waiting for SBOM + vuln scan + VEX ..."
  for i in $(seq 1 120); do
    VEXKEY=$(api GET "/api/v1/artifacts/$CID" | python3 -c 'import json,sys; print(json.loads(sys.stdin.read()).get("vexObjectKey",""))')
    [ -n "$VEXKEY" ] && break
    sleep 2
  done
  [ -n "$VEXKEY" ] || { echo "VEX document never appeared (vexObjectKey empty after 240s)"; return 1; }
  echo "VEX generated: $VEXKEY"

  URL=$(api POST "/api/v1/medical/artifacts/$CID/sbom/vex/presign" | jget url)
  curl -sf "$URL" -o "$WORK_DIR/artifact-c.vex.json"
  python3 - "$WORK_DIR/artifact-c.vex.json" <<'PY'
import json, sys
doc = json.load(open(sys.argv[1]))
assert doc.get("bomFormat") == "CycloneDX", f"unexpected bomFormat: {doc.get('bomFormat')}"
assert doc.get("metadata", {}).get("component", {}).get("name"), "VEX missing artifact component metadata"
vulns = doc.get("vulnerabilities") or []
# The uploaded bundle pins old urllib3/requests, so the scan must find CVEs
# (the scan job extracts the tar.gz before invoking the scanner).
assert vulns, "VEX has no vulnerability entries despite known-vulnerable pinned deps"
for v in vulns:
    assert v.get("id"), "vulnerability entry missing id"
    assert v.get("analysis", {}).get("state"), f"vulnerability {v.get('id')} missing analysis state"
    assert v.get("affects"), f"vulnerability {v.get('id')} missing affects (SBOM component refs)"
print(f"VEX valid: {len(vulns)} CVE assertion(s) referencing SBOM components")
PY
}

# ── Scenario 4: HIPAA audit export ────────────────────────────────────────────
scenario_hipaa_export() {
  ENROLL_TOKEN=$(api POST /api/v1/enrollments -d '{"expiresInSec":3600}' | jget token)
  D="$WORK_DIR/device-1"
  mkdir -p "$D"
  openssl req -newkey rsa:2048 -nodes \
    -keyout "$D/device.key" -out "$D/device.csr" -subj "/CN=medical-e2e-device-1" 2>/dev/null
  CSR=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' "$D/device.csr")
  ENROLL_JSON=$(curl -sS --fail --cacert "$CA_CERT_PATH" -X POST "$BASE_URL/api/v1/devices/enroll" \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$ENROLL_TOKEN\",\"csr\":\"$CSR\",\"capabilities\":{\"hardwareId\":\"medical-e2e-hw-1\",\"hardwareSource\":\"machine-id\"}}")
  DEVICE_ID=$(echo "$ENROLL_JSON" | jget deviceId)
  echo "$ENROLL_JSON" | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["certPem"])' > "$D/device.crt"
  [ -s "$D/device.crt" ] || { echo "enrollment returned no device cert"; return 1; }
  echo "device enrolled: $DEVICE_ID"

  AID=$(cat "$WORK_DIR/artifact-a.id" 2>/dev/null || echo "")
  curl -sS --fail --cacert "$CA_CERT_PATH" \
    --cert "$D/device.crt" --key "$D/device.key" \
    -X POST "$BASE_URL/api/v1/devices/$DEVICE_ID/apply-result" \
    -H "Content-Type: application/json" \
    -d "{\"status\":\"success\",\"artifactId\":\"$AID\",\"appliedVersion\":\"1.0.0\",\"appliedConfigRev\":\"\"}" > /dev/null
  echo "device posted apply-result over mTLS"

  api GET "/api/v1/medical/audit/hipaa-export?limit=50" > "$WORK_DIR/hipaa-export.json"
  python3 - "$WORK_DIR/hipaa-export.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
items = d["items"]
assert items, "HIPAA export returned no PHI-touched events"
for ev in items:
    assert ev["phiTouched"] is True, f"non-PHI event in default export: {ev}"
ev = items[0]
for field in ("dateTime", "userId", "actionType", "dataAccessed", "status"):
    assert ev.get(field) not in (None, ""), f"HIPAA field {field} missing: {ev}"
print(f"HIPAA export OK: {len(items)} PHI-touched event(s), required fields populated")
PY
}

# ── Scenario 5: QMS package ───────────────────────────────────────────────────
scenario_qms_package() {
  # 5a: fresh artifact with no evidence → 422 with the structured checklist.
  DID=$(upload_artifact med-app-d 1.0.0)
  CODE=$(api_code POST "/api/v1/medical/artifacts/$DID/qms-package")
  [ "$CODE" = "422" ] || { echo "expected 422 for incomplete prerequisites, got $CODE"; return 1; }
  python3 - "$LOG_DIR/last-body.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
p = d["prerequisites"]
for k in ("sbom", "vex", "changeRecord", "vulnScan"):
    assert k in p, f"missing prerequisite key {k}"
    assert p[k]["ready"] is False and p[k]["reason"], f"prerequisite {k} should be not-ready with a reason"
print("422 prerequisite checklist OK:", ", ".join(p[k]["reason"] for k in p))
PY

  if [ "$HAVE_TRIVY" != "1" ]; then
    echo "trivy not installed — skipping full-package half of the scenario"
    return 0
  fi

  # 5b: artifact A has an approved change record; wait for SBOM/scan/VEX, then download.
  AID=$(cat "$WORK_DIR/artifact-a.id")
  CODE=""
  for i in $(seq 1 120); do
    CODE=$(api_code POST "/api/v1/medical/artifacts/$AID/qms-package")
    [ "$CODE" = "200" ] && break
    sleep 2
  done
  [ "$CODE" = "200" ] || { echo "QMS package never became ready: $(cat "$LOG_DIR/last-body.json")"; return 1; }
  URL=$(jget url < "$LOG_DIR/last-body.json")
  curl -sf "$URL" -o "$WORK_DIR/qms-package.zip"
  for f in manifest.json sbom.cdx.json sbom.vex.json attestations.json change-record.json \
           change-record-audit-trail.json vuln-scan-summary.json deployment-audit-trail.json; do
    unzip -l "$WORK_DIR/qms-package.zip" | grep -q "$f" || { echo "ZIP missing $f"; return 1; }
  done
  echo "QMS package ZIP contains all 8 evidence files"
}

run_scenario "change-record" scenario_change_record
run_scenario "break-glass" scenario_break_glass
if [ "$HAVE_TRIVY" = "1" ]; then
  run_scenario "vex-generation" scenario_vex
else
  skip_scenario "vex-generation" "trivy not installed (needed for SBOM + vuln scan)"
fi
run_scenario "hipaa-export" scenario_hipaa_export
run_scenario "qms-package" scenario_qms_package

echo ""
log "Suite complete."
