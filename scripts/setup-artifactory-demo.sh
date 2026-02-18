#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

ARTIFACTORY_CONTAINER_NAME=${ARTIFACTORY_CONTAINER_NAME:-hardwareops-artifactory-demo}
ARTIFACTORY_IMAGE=${ARTIFACTORY_IMAGE:-releases-docker.jfrog.io/jfrog/artifactory-oss:7.77.8}
ARTIFACTORY_PORT=${ARTIFACTORY_PORT:-8082}
ARTIFACTORY_INTERNAL_PORT=${ARTIFACTORY_INTERNAL_PORT:-8081}
ARTIFACTORY_BASE_URL=${ARTIFACTORY_BASE_URL:-http://localhost:${ARTIFACTORY_PORT}}
ARTIFACTORY_DATA_DIR=${ARTIFACTORY_DATA_DIR:-/tmp/hardwareops-artifactory-demo/data}
ARTIFACTORY_START_CONTAINER=${ARTIFACTORY_START_CONTAINER:-1}
ARTIFACTORY_WAIT_TIMEOUT_SECONDS=${ARTIFACTORY_WAIT_TIMEOUT_SECONDS:-900}
ARTIFACTORY_RECREATE_ON_PORT_MISMATCH=${ARTIFACTORY_RECREATE_ON_PORT_MISMATCH:-1}
ARTIFACTORY_RECREATE_ON_IMAGE_MISMATCH=${ARTIFACTORY_RECREATE_ON_IMAGE_MISMATCH:-1}
ARTIFACTORY_RESET_DATA_ON_IMAGE_MISMATCH=${ARTIFACTORY_RESET_DATA_ON_IMAGE_MISMATCH:-1}
ARTIFACTORY_RECOVER_UNHEALTHY_CONTAINER=${ARTIFACTORY_RECOVER_UNHEALTHY_CONTAINER:-1}
ARTIFACTORY_RECOVER_AFTER_SECONDS=${ARTIFACTORY_RECOVER_AFTER_SECONDS:-90}

ARTIFACTORY_ADMIN_USER=${ARTIFACTORY_ADMIN_USER:-admin}
ARTIFACTORY_ADMIN_PASSWORD=${ARTIFACTORY_ADMIN_PASSWORD:-}
ARTIFACTORY_REPO_KEY=${ARTIFACTORY_REPO_KEY:-example-repo-local}
ARTIFACTORY_PATH_PREFIX=${ARTIFACTORY_PATH_PREFIX:-hardwareops}

OUTPUT_DIR=${OUTPUT_DIR:-/tmp/hardwareops-artifactory-demo}
ARTIFACT_NAME=${ARTIFACT_NAME:-artifactory-demo}
ARTIFACT_VERSION=${ARTIFACT_VERSION:-0.1.0}
ARTIFACT_TYPE=${ARTIFACT_TYPE:-app_bundle}
CREDENTIAL_REF=${CREDENTIAL_REF:-artifactory-demo}
ARTIFACTORY_DATA_IMAGE_MARKER_PATH=${ARTIFACTORY_DATA_IMAGE_MARKER_PATH:-${OUTPUT_DIR}/.artifactory-image}
ARTIFACT_SIGN=${ARTIFACT_SIGN:-1}
SIGNING_DIR=${SIGNING_DIR:-/tmp/hardwareops-demo/signing}
SIGNING_KEY=${SIGNING_KEY:-}
SIGNING_PUB=${SIGNING_PUB:-$SIGNING_DIR/ed25519.pub}
SIGNING_KEY_ID=${SIGNING_KEY_ID:-}
SIGNATURE_OUT=${SIGNATURE_OUT:-$OUTPUT_DIR/${ARTIFACT_NAME}-${ARTIFACT_VERSION}.sig}

log() {
  echo "[setup-artifactory-demo] $*"
}

fail() {
  echo "[setup-artifactory-demo] ERROR: $*" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

need_cmd docker
need_cmd curl
need_cmd python3
need_cmd sha256sum

mkdir -p "$OUTPUT_DIR" "$ARTIFACTORY_DATA_DIR" "$OUTPUT_DIR/input"

clear_data_dir() {
  mkdir -p "$ARTIFACTORY_DATA_DIR"
  docker run --rm -v "${ARTIFACTORY_DATA_DIR}:/data" alpine:3.19 sh -lc \
    "rm -rf /data/* /data/.[!.]* /data/..?* 2>/dev/null || true" >/dev/null
  mkdir -p "$ARTIFACTORY_DATA_DIR"
}

ensure_data_dir_permissions() {
  # Artifactory container runs as uid/gid 1030 by default.
  # Use a one-shot root container to normalize ownership/permissions on host volume.
  docker run --rm -v "${ARTIFACTORY_DATA_DIR}:/data" alpine:3.19 sh -lc \
    "chown -R 1030:1030 /data && chmod -R u+rwX,g+rwX,o+rX /data" >/dev/null
}

prepare_data_dir_for_image() {
  if docker ps -a --format '{{.Names}}' | grep -q "^${ARTIFACTORY_CONTAINER_NAME}$"; then
    return 0
  fi

  local marker_file="${ARTIFACTORY_DATA_IMAGE_MARKER_PATH}"
  local existing_image=""
  if [ -f "$marker_file" ]; then
    existing_image=$(cat "$marker_file" || true)
  fi

  if [ -n "$existing_image" ] && [ "$existing_image" != "$ARTIFACTORY_IMAGE" ] && [ "$ARTIFACTORY_RESET_DATA_ON_IMAGE_MISMATCH" = "1" ]; then
    log "clearing ${ARTIFACTORY_DATA_DIR} due to data/image mismatch (${existing_image} vs ${ARTIFACTORY_IMAGE})"
    clear_data_dir
  fi

  if [ -z "$existing_image" ] && [ "$ARTIFACTORY_RESET_DATA_ON_IMAGE_MISMATCH" = "1" ] && \
    find "$ARTIFACTORY_DATA_DIR" -mindepth 1 -maxdepth 1 | grep -q .; then
    log "clearing ${ARTIFACTORY_DATA_DIR} due to untracked prior Artifactory data"
    clear_data_dir
  fi

  mkdir -p "$(dirname "$marker_file")"
  printf "%s\n" "$ARTIFACTORY_IMAGE" >"$marker_file"
}

ensure_container_mapping() {
  if ! docker ps -a --format '{{.Names}}' | grep -q "^${ARTIFACTORY_CONTAINER_NAME}$"; then
    return 0
  fi
  local image_name
  image_name=$(docker inspect -f '{{.Config.Image}}' "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null || true)
  if [ "$image_name" != "$ARTIFACTORY_IMAGE" ]; then
    if [ "$ARTIFACTORY_RECREATE_ON_IMAGE_MISMATCH" = "1" ]; then
      log "recreating container due to image mismatch (have ${image_name}, want ${ARTIFACTORY_IMAGE})"
      docker rm -f "$ARTIFACTORY_CONTAINER_NAME" >/dev/null
      if [ "$ARTIFACTORY_RESET_DATA_ON_IMAGE_MISMATCH" = "1" ]; then
        log "clearing ${ARTIFACTORY_DATA_DIR} due to image mismatch"
        clear_data_dir
      fi
      return 0
    fi
    fail "existing container uses image ${image_name}. Set ARTIFACTORY_RECREATE_ON_IMAGE_MISMATCH=1 or remove ${ARTIFACTORY_CONTAINER_NAME}."
  fi
  # Example output: "0.0.0.0:8082->8081/tcp, :::8082->8081/tcp"
  local ports
  ports=$(docker port "$ARTIFACTORY_CONTAINER_NAME" "${ARTIFACTORY_INTERNAL_PORT}/tcp" 2>/dev/null || true)
  if echo "$ports" | grep -q ":${ARTIFACTORY_PORT}$"; then
    return 0
  fi
  if [ "$ARTIFACTORY_RECREATE_ON_PORT_MISMATCH" = "1" ]; then
    log "recreating container due to port mapping mismatch (want host:${ARTIFACTORY_PORT} -> container:${ARTIFACTORY_INTERNAL_PORT})"
    docker rm -f "$ARTIFACTORY_CONTAINER_NAME" >/dev/null
    return 0
  fi
  fail "existing container has unexpected port mapping for ${ARTIFACTORY_INTERNAL_PORT}/tcp. Set ARTIFACTORY_RECREATE_ON_PORT_MISMATCH=1 or remove ${ARTIFACTORY_CONTAINER_NAME}."
}

if [ "$ARTIFACTORY_START_CONTAINER" = "1" ]; then
  ensure_container_mapping
  prepare_data_dir_for_image
  log "preparing data directory permissions at ${ARTIFACTORY_DATA_DIR}"
  ensure_data_dir_permissions
  preexisting_running=0
  if docker ps -a --format '{{.Names}}' | grep -q "^${ARTIFACTORY_CONTAINER_NAME}$"; then
    if ! docker ps --format '{{.Names}}' | grep -q "^${ARTIFACTORY_CONTAINER_NAME}$"; then
      log "starting existing container $ARTIFACTORY_CONTAINER_NAME"
      docker start "$ARTIFACTORY_CONTAINER_NAME" >/dev/null
    else
      log "container already running: $ARTIFACTORY_CONTAINER_NAME"
      preexisting_running=1
    fi
  else
    log "launching Artifactory container $ARTIFACTORY_CONTAINER_NAME"
    docker run -d \
      --name "$ARTIFACTORY_CONTAINER_NAME" \
      --restart unless-stopped \
      -p "${ARTIFACTORY_PORT}:${ARTIFACTORY_INTERNAL_PORT}" \
      -v "${ARTIFACTORY_DATA_DIR}:/var/opt/jfrog/artifactory" \
      "$ARTIFACTORY_IMAGE" >/dev/null
  fi

  # If an existing container was already running but is clearly unhealthy, reset once.
  if [ "$preexisting_running" = "1" ] && [ "$ARTIFACTORY_RECOVER_UNHEALTHY_CONTAINER" = "1" ]; then
    if ! curl -fsS "${ARTIFACTORY_BASE_URL}/artifactory/api/system/ping" 2>/dev/null | grep -qi "OK"; then
      log "existing Artifactory container is not healthy; recreating with clean data"
      docker rm -f "$ARTIFACTORY_CONTAINER_NAME" >/dev/null
      clear_data_dir
      ensure_data_dir_permissions
      docker run -d \
        --name "$ARTIFACTORY_CONTAINER_NAME" \
        --restart unless-stopped \
        -p "${ARTIFACTORY_PORT}:${ARTIFACTORY_INTERNAL_PORT}" \
        -v "${ARTIFACTORY_DATA_DIR}:/var/opt/jfrog/artifactory" \
        "$ARTIFACTORY_IMAGE" >/dev/null
    fi
  fi
fi

log "waiting for Artifactory readiness"
start_epoch=$(date +%s)
last_wait_log_epoch=0
recover_attempted=0
until ping_body=$(curl -fsS "${ARTIFACTORY_BASE_URL}/artifactory/api/system/ping" 2>/dev/null || true) && echo "$ping_body" | grep -qi "OK"; do
  status=$(docker inspect -f '{{.State.Status}}' "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null || true)
  if [ "$status" = "restarting" ] && [ "$ARTIFACTORY_START_CONTAINER" = "1" ]; then
    docker logs --tail 120 "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null || true
    fail "artifactory container is restarting repeatedly (check volume permissions at ${ARTIFACTORY_DATA_DIR})"
  fi
  if docker logs --tail 200 "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null | grep -q "DB Type derby is not allowed"; then
    fail "this Artifactory image requires external PostgreSQL. Use ARTIFACTORY_IMAGE=releases-docker.jfrog.io/jfrog/artifactory-oss:7.77.8 for local demo."
  fi
  if [ "$ARTIFACTORY_START_CONTAINER" = "1" ] && ! docker ps --format '{{.Names}}' | grep -q "^${ARTIFACTORY_CONTAINER_NAME}$"; then
    docker logs --tail 120 "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null || true
    fail "artifactory container is not running"
  fi
  now_epoch=$(date +%s)
  elapsed=$((now_epoch - start_epoch))
  if [ "$ARTIFACTORY_RECOVER_UNHEALTHY_CONTAINER" = "1" ] && [ "$recover_attempted" = "0" ] && [ "$elapsed" -ge "$ARTIFACTORY_RECOVER_AFTER_SECONDS" ]; then
    log "startup exceeded ${ARTIFACTORY_RECOVER_AFTER_SECONDS}s without readiness; resetting container data and retrying once"
    docker rm -f "$ARTIFACTORY_CONTAINER_NAME" >/dev/null || true
    clear_data_dir
    ensure_data_dir_permissions
    docker run -d \
      --name "$ARTIFACTORY_CONTAINER_NAME" \
      --restart unless-stopped \
      -p "${ARTIFACTORY_PORT}:${ARTIFACTORY_INTERNAL_PORT}" \
      -v "${ARTIFACTORY_DATA_DIR}:/var/opt/jfrog/artifactory" \
      "$ARTIFACTORY_IMAGE" >/dev/null
    recover_attempted=1
    start_epoch=$(date +%s)
    last_wait_log_epoch=0
    continue
  fi
  if [ $((now_epoch - last_wait_log_epoch)) -ge 15 ]; then
    log "still waiting for readiness (${elapsed}s elapsed)"
    last_wait_log_epoch=$now_epoch
  fi
  if [ $((now_epoch - start_epoch)) -ge "$ARTIFACTORY_WAIT_TIMEOUT_SECONDS" ]; then
    if [ "$ARTIFACTORY_START_CONTAINER" = "1" ]; then
      docker logs --tail 120 "$ARTIFACTORY_CONTAINER_NAME" 2>/dev/null || true
    fi
    fail "timed out waiting for Artifactory at ${ARTIFACTORY_BASE_URL}"
  fi
  sleep 5
done

if [ -z "$ARTIFACTORY_ADMIN_PASSWORD" ] && [ "$ARTIFACTORY_START_CONTAINER" = "1" ]; then
  bootstrap=$(docker exec "$ARTIFACTORY_CONTAINER_NAME" sh -lc '
for p in \
  /opt/jfrog/artifactory/var/etc/security/bootstrap.creds \
  /var/opt/jfrog/artifactory/etc/security/bootstrap.creds \
  /opt/jfrog/artifactory/var/etc/access/bootstrap.creds
do
  if [ -f "$p" ]; then
    cat "$p"
    exit 0
  fi
done
exit 0
' || true)
  if [ -n "${bootstrap:-}" ]; then
    ARTIFACTORY_ADMIN_PASSWORD=$(printf "%s\n" "$bootstrap" | awk 'NF {line=$0} END{print line}')
    # Handle "user:password" and plain password forms.
    if [[ "$ARTIFACTORY_ADMIN_PASSWORD" == *:* ]]; then
      ARTIFACTORY_ADMIN_PASSWORD="${ARTIFACTORY_ADMIN_PASSWORD##*:}"
    fi
  fi
fi

if [ -z "$ARTIFACTORY_ADMIN_PASSWORD" ]; then
  ARTIFACTORY_ADMIN_PASSWORD=password
  log "using fallback admin password 'password' (override with ARTIFACTORY_ADMIN_PASSWORD if needed)"
fi

repos_json=$(curl -fsS \
  -u "${ARTIFACTORY_ADMIN_USER}:${ARTIFACTORY_ADMIN_PASSWORD}" \
  "${ARTIFACTORY_BASE_URL}/artifactory/api/repositories")
resolved_repo_key=$(python3 - <<'PY' "$repos_json" "$ARTIFACTORY_REPO_KEY"
import json
import sys

repos = json.loads(sys.argv[1])
requested = sys.argv[2]
keys = [item.get("key") for item in repos if item.get("key")]
if requested in keys:
    print(requested)
    raise SystemExit(0)

for item in repos:
    if item.get("type") == "LOCAL" and str(item.get("packageType", "")).lower() == "generic" and item.get("key"):
        print(item["key"])
        raise SystemExit(0)

print("")
PY
)
if [ -z "$resolved_repo_key" ]; then
  fail "no compatible local generic repository found in Artifactory OSS (check ${ARTIFACTORY_BASE_URL}/artifactory/api/repositories)"
fi
if [ "$resolved_repo_key" != "$ARTIFACTORY_REPO_KEY" ]; then
  log "repository ${ARTIFACTORY_REPO_KEY} not found; using ${resolved_repo_key}"
fi
ARTIFACTORY_REPO_KEY="$resolved_repo_key"

cat > "${OUTPUT_DIR}/input/readme.txt" <<EOF
HardwareOps Artifactory adapter demo artifact
name=${ARTIFACT_NAME}
version=${ARTIFACT_VERSION}
createdAt=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
date -u +"%Y-%m-%dT%H:%M:%SZ" > "${OUTPUT_DIR}/input/build.txt"

artifact_path="${OUTPUT_DIR}/${ARTIFACT_NAME}-${ARTIFACT_VERSION}.tar.gz"
pack_args=(
  --name "$ARTIFACT_NAME"
  --version "$ARTIFACT_VERSION"
  --type "$ARTIFACT_TYPE"
  --input-dir "${OUTPUT_DIR}/input"
  --out "$artifact_path"
)

if [ "$ARTIFACT_SIGN" = "1" ]; then
  if [ -z "$SIGNING_KEY" ]; then
    SIGNING_KEY="$SIGNING_DIR/ed25519.key"
  fi
  if [ ! -f "$SIGNING_KEY" ] || [ ! -f "$SIGNING_PUB" ]; then
    export SIGNING_DIR
    export SIGNING_KEY
    export SIGNING_PUB_KEY_PATH="$SIGNING_PUB"
    export SIGNING_KEY_ID
    # shellcheck disable=SC1091
    source "$BASE_DIR/scripts/ensure-signing-key.sh"
    SIGNING_KEY="$SIGNING_KEY"
    SIGNING_PUB="$SIGNING_PUB"
    SIGNING_KEY_ID="$SIGNING_KEY_ID"
  fi
  [ -f "$SIGNING_KEY" ] || fail "signing enabled but key not found: $SIGNING_KEY"
  pack_args+=(--signing-key "$SIGNING_KEY" --signature-out "$SIGNATURE_OUT")
  if [ -n "$SIGNING_KEY_ID" ]; then
    pack_args+=(--signing-key-id "$SIGNING_KEY_ID")
  fi
fi

pack_json=$(python3 "$BASE_DIR/scripts/artifact-pack.py" "${pack_args[@]}")
artifact_sha=$(python3 - <<'PY' "$pack_json"
import json,sys
print(json.loads(sys.argv[1]).get("sha256",""))
PY
)
artifact_signature=$(python3 - <<'PY' "$pack_json"
import json,sys
print(json.loads(sys.argv[1]).get("signature",""))
PY
)
artifact_signature_key_id=$(python3 - <<'PY' "$pack_json"
import json,sys
print(json.loads(sys.argv[1]).get("signatureKeyId",""))
PY
)

object_rel_path="${ARTIFACTORY_REPO_KEY}/${ARTIFACTORY_PATH_PREFIX}/${ARTIFACT_NAME}-${ARTIFACT_VERSION}.tar.gz"
download_uri="${ARTIFACTORY_BASE_URL}/artifactory/${object_rel_path}"

log "uploading artifact to ${download_uri}"
curl -fsS \
  -u "${ARTIFACTORY_ADMIN_USER}:${ARTIFACTORY_ADMIN_PASSWORD}" \
  -H "Content-Type: application/gzip" \
  --upload-file "$artifact_path" \
  "$download_uri" >/dev/null

credentials_file="${OUTPUT_DIR}/pull-credentials.json"
python3 - <<'PY' "$credentials_file" "$CREDENTIAL_REF" "$ARTIFACTORY_ADMIN_USER" "$ARTIFACTORY_ADMIN_PASSWORD"
import json
import sys
payload = {
    sys.argv[2]: {
        "username": sys.argv[3],
        "password": sys.argv[4],
    }
}
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2)
    fh.write("\n")
PY
chmod 600 "$credentials_file"

pull_payload_file="${OUTPUT_DIR}/pull-request.json"
python3 - <<'PY' "$pull_payload_file" "$ARTIFACT_NAME" "$ARTIFACT_VERSION" "$ARTIFACT_TYPE" "$download_uri" "$artifact_sha" "$CREDENTIAL_REF" "$artifact_signature" "$artifact_signature_key_id"
import json
import sys
payload = {
    "name": sys.argv[2],
    "version": sys.argv[3],
    "type": sys.argv[4],
    "source": {
        "kind": "artifactory",
        "uri": sys.argv[5],
        "credentialRef": sys.argv[7],
    },
    "sha256": sys.argv[6],
}
if sys.argv[8]:
    payload["signature"] = sys.argv[8]
if sys.argv[9]:
    payload["signatureKeyId"] = sys.argv[9]
with open(sys.argv[1], "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2)
    fh.write("\n")
PY

env_file="${OUTPUT_DIR}/artifactory-demo.env"
{
  printf "ARTIFACTORY_BASE_URL=%q\n" "$ARTIFACTORY_BASE_URL"
  printf "ARTIFACTORY_DOWNLOAD_URI=%q\n" "$download_uri"
  printf "ARTIFACTORY_REPO_KEY=%q\n" "$ARTIFACTORY_REPO_KEY"
  printf "ARTIFACT_SHA256=%q\n" "$artifact_sha"
  printf "ARTIFACT_NAME=%q\n" "$ARTIFACT_NAME"
  printf "ARTIFACT_VERSION=%q\n" "$ARTIFACT_VERSION"
  printf "ARTIFACT_TYPE=%q\n" "$ARTIFACT_TYPE"
  printf "ARTIFACT_SIGNED=%q\n" "$([ -n "$artifact_signature" ] && echo 1 || echo 0)"
  printf "ARTIFACT_SIGNATURE=%q\n" "$artifact_signature"
  printf "ARTIFACT_SIGNATURE_KEY_ID=%q\n" "$artifact_signature_key_id"
  printf "SIGNING_PUB=%q\n" "$SIGNING_PUB"
  printf "CREDENTIAL_REF=%q\n" "$CREDENTIAL_REF"
  printf "CREDENTIALS_FILE=%q\n" "$credentials_file"
  printf "PULL_PAYLOAD_FILE=%q\n" "$pull_payload_file"
  printf "ARTIFACT_PATH=%q\n" "$artifact_path"
} >"$env_file"

log "setup complete"
echo
echo "Artifacts + credentials generated:"
echo "  env file:          $env_file"
echo "  pull payload file: $pull_payload_file"
echo "  credentials file:  $credentials_file"
echo "  artifact uri:      $download_uri"
echo "  sha256:            $artifact_sha"
if [ -n "$artifact_signature" ]; then
  echo "  signed:            yes (${artifact_signature_key_id:-unknown-key})"
else
  echo "  signed:            no"
fi
echo
echo "Control-plane must start with credential resolver config:"
echo "  ARTIFACT_PULL_CREDENTIALS_FILE=$credentials_file"
echo
echo "Then run end-to-end pull test:"
echo "  ./scripts/test-artifactory-adapter.sh RUN_SETUP=0 SETUP_OUTPUT_DIR=$OUTPUT_DIR"
