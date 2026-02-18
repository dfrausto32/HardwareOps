# Artifact Ingest Test Plan (Push + Pull)

This runbook gives a repeatable local test for both ingest paths:
- **Push:** `/api/v1/artifacts/presign-upload` + `/api/v1/artifacts/complete`
- **Pull:** `/api/v1/artifacts/pull`

## Quick path (automated)

After control-plane is running, execute:

```bash
AUTH_EMAIL=admin@example.com AUTH_PASSWORD=change-me \
./scripts/test-artifact-ingest.sh
```

Defaults:
- Pull request uses adapter-ready `source.kind=http` + `source.uri`.
- Set `PULL_SOURCE_MODE=legacy` to validate backward-compatible `sourceUrl`.
- For credentialRef coverage, set `ARTIFACT_PULL_CREDENTIALS_FILE` or `ARTIFACT_PULL_CREDENTIALS_JSON` before starting control-plane and call pull with `source.credentialRef`.
- CI helper scripts:
  - Push: `scripts/ci-upload-artifact.sh`
  - Pull: `scripts/ci-pull-artifact.sh`

Artifactory adapter local smoke:
```bash
./scripts/setup-artifactory-demo.sh
./scripts/test-artifactory-adapter.sh RUN_SETUP=0
```

Artifactory adapter cloud smoke (with Secrets Manager-backed resolver):
```bash
BASE_URL=https://app.<customer-domain> \
INSECURE=1 \
RUN_SETUP=0 \
SETUP_OUTPUT_DIR=/tmp/hardwareops-artifactory-demo \
./scripts/test-artifactory-adapter.sh
```

Use the manual steps below if you want to inspect each API call directly.

## 1) Test environment

From repo root:

```bash
cp deploy/compose/.env.example deploy/compose/.env
make dev-up
```

Start control-plane (new shell):

```bash
AUTH_MODE=local \
AUTH_JWT_SECRET=dev-jwt-secret \
AUTH_BOOTSTRAP_EMAIL=admin@example.com \
AUTH_BOOTSTRAP_PASSWORD=change-me \
MAINTENANCE_MODE=0 \
ENABLE_TLS=1 \
./scripts/run-control-plane.sh
```

## 2) Create auth tokens for ingest calls

From repo root (another shell):

```bash
BASE_URL=https://localhost:8080
CA_CERT_PATH=./dev-ca.crt

LOGIN_JSON=$(curl -sS --fail --cacert "$CA_CERT_PATH" \
  -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"change-me"}')

ADMIN_JWT=$(python3 - <<'PY' "$LOGIN_JSON"
import json, sys
print(json.loads(sys.argv[1])["token"])
PY
)

SERVICE_JSON=$(curl -sS --fail --cacert "$CA_CERT_PATH" \
  -X POST "$BASE_URL/api/v1/auth/service-tokens" \
  -H "Authorization: Bearer $ADMIN_JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"ingest-test","scopes":["artifact.publish"],"ttlHours":1}')

CI_TOKEN=$(python3 - <<'PY' "$SERVICE_JSON"
import json, sys
print(json.loads(sys.argv[1])["token"])
PY
)
```

## 3) Prepare a test artifact payload

```bash
mkdir -p /tmp/hwops-ingest/input
cat > /tmp/hwops-ingest/input/readme.txt <<'EOF'
artifact ingest test payload
EOF
date -u +"%Y-%m-%dT%H:%M:%SZ" > /tmp/hwops-ingest/input/build.txt
```

## 4) Push ingest test (CI-style)

```bash
ARTIFACT_NAME=push-demo \
ARTIFACT_VERSION=0.0.1 \
ARTIFACT_TYPE=app_bundle \
INPUT_DIR=/tmp/hwops-ingest/input \
OUT_PATH=/tmp/hwops-ingest/push-demo.tar.gz \
BASE_URL="$BASE_URL" \
CA_CERT_PATH="$CA_CERT_PATH" \
CI_SERVICE_TOKEN="$CI_TOKEN" \
./scripts/ci-upload-artifact.sh
```

Expected:
- Script returns artifact JSON.
- Artifact appears in `/api/v1/artifacts`.
- Audit includes `artifact.upload.presign` and `artifact.upload.complete`.

## 5) Pull ingest test (server-side fetch)

Start a local file server in `/tmp/hwops-ingest`:

```bash
cd /tmp/hwops-ingest
python3 -m http.server 18080
```

In another shell:

```bash
PULL_SHA=$(sha256sum /tmp/hwops-ingest/push-demo.tar.gz | awk '{print $1}')

curl -sS --fail --cacert "$CA_CERT_PATH" \
  -X POST "$BASE_URL/api/v1/artifacts/pull" \
  -H "Authorization: Bearer $CI_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(python3 - <<'PY' "$PULL_SHA"
import json, sys
print(json.dumps({
  "name": "pull-demo",
  "version": "0.0.1",
  "type": "app_bundle",
  "sourceUrl": "http://localhost:18080/push-demo.tar.gz",
  "sha256": sys.argv[1]
}))
PY
)"
```

Expected:
- 200 response with artifact JSON.
- Audit includes `artifact.pull`.

## 6) Verify artifacts and audit records

```bash
curl -sS --fail --cacert "$CA_CERT_PATH" \
  -H "Authorization: Bearer $ADMIN_JWT" \
  "$BASE_URL/api/v1/artifacts?limit=20"

curl -sS --fail --cacert "$CA_CERT_PATH" \
  -H "Authorization: Bearer $ADMIN_JWT" \
  "$BASE_URL/api/v1/audit?limit=50"
```

## 7) Negative tests (required)

### 7.1 SHA mismatch should fail

```bash
curl -sS --cacert "$CA_CERT_PATH" \
  -X POST "$BASE_URL/api/v1/artifacts/pull" \
  -H "Authorization: Bearer $CI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"pull-bad-sha","version":"0.0.1","sourceUrl":"http://localhost:18080/push-demo.tar.gz","sha256":"deadbeef"}'
```

Expected: `400` with `sha256 mismatch`.

### 7.2 Host allowlist should block disallowed source

Restart control-plane with:

```bash
ARTIFACT_PULL_ALLOWED_HOSTS=example.com
```

Run the pull call again with `localhost` source URL.

Expected: `403` with `sourceUrl host not allowed`.

## 8) Cleanup

```bash
pkill -f "python3 -m http.server 18080" || true
rm -rf /tmp/hwops-ingest
```

---

## Notes
- If running control-plane inside Docker instead of `run-control-plane.sh`, `localhost` in `sourceUrl` resolves inside the container. Use a reachable host/IP for the control-plane runtime.
- Keep service-token TTL short for test runs.
