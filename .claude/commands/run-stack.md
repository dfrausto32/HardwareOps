# /run-stack

Start the full Parcel development stack in the correct order and verify each component is healthy.

## Prerequisites check

Before starting, verify:
```bash
# Docker running?
docker info > /dev/null 2>&1 || echo "ERROR: Docker not running"

# Dev CA exists?
ls ./dev-ca.crt ./dev-ca.key 2>/dev/null || echo "WARN: Dev CA missing — run step 2 below"

# .env exists?
ls deploy/compose/.env 2>/dev/null || echo "WARN: deploy/compose/.env missing"
```

## Full startup sequence

### Step 1 — Dependencies (Postgres + MinIO)
```bash
cp deploy/compose/.env.example deploy/compose/.env 2>/dev/null || true
make dev-up
```
Wait a few seconds, then verify:
```bash
docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "postgres|minio"
```

### Step 2 — Dev CA (if missing)
```bash
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ./dev-ca.key -out ./dev-ca.crt \
  -days 365 -subj "/CN=Parcel Dev CA"
```

### Step 3 — Control-plane (run in a terminal)
```bash
export DATABASE_URL=postgres://parcel:parcel@localhost:5432/parcel?sslmode=disable
export CA_CERT_PATH=./dev-ca.crt
export CA_KEY_PATH=./dev-ca.key
export AUTO_MIGRATE=1
export DISABLE_HTTP2=1
export MAINTENANCE_MODE=0
export CORS_ALLOWED_ORIGINS=http://localhost:5173
ENABLE_TLS=1 ./scripts/run-control-plane.sh
```
Verify (in another terminal):
```bash
curl --cacert ./dev-ca.crt https://localhost:8080/healthz
```

### Step 4 — UI (run in a terminal)
```bash
cd ui-generic && npm install --silent
VITE_API_BASE_URL=https://localhost:8080 VITE_SIMULATE_PROD=1 npm run dev
```
Opens at http://localhost:5173

### Step 5 — Demo agents (optional)
```bash
DEMO_COUNT=3 ./scripts/run-demo-agent.sh
```

### Step 6 — Global plane (optional, for federation features)
```bash
# Ensure parcel_global database exists
psql postgres://parcel:parcel@localhost:5432 -c "CREATE DATABASE parcel_global;" 2>/dev/null || true
# Generate encryption key if not set
export GLOBAL_TOKEN_ENCRYPTION_KEY=$(openssl rand -base64 32)
export GLOBAL_DATABASE_URL=postgres://parcel:parcel@localhost:5432/parcel_global?sslmode=disable
export AUTH_JWT_SECRET=change-me-32-char-secret
export GLOBAL_MIGRATIONS_DIR=migrations/global
cd control-plane && go run ./cmd/global-plane
```
Verify: `curl http://localhost:8090/healthz`

## Quick health check (all services)

```bash
echo "--- Postgres ---"
pg_isready -h localhost -p 5432 -U parcel
echo "--- MinIO ---"
curl -s http://localhost:9000/minio/health/live && echo " OK"
echo "--- Control-plane ---"
curl -s --cacert ./dev-ca.crt https://localhost:8080/healthz
echo "--- UI ---"
curl -s -o /dev/null -w "%{http_code}" http://localhost:5173
```
