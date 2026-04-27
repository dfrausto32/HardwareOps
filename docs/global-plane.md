# Global Aggregation Plane (E1)

The global-plane is a read-only aggregation service that sits above one or more regional Parcel control planes. It polls each regional plane on a configurable interval, caches device, artifact, and health data, and serves a unified API for cross-region visibility.

Agents are unaffected — they continue talking to their regional control plane only.

---

## Architecture

```
┌─────────────────────────────────┐
│       Global Control Plane      │
│  - Regional plane registry      │
│  - Device directory cache       │
│  - Artifact cache               │
│  - Health cache                 │
└──────┬──────────────┬───────────┘
       │  service tokens (poll)
┌──────▼──────┐  ┌──────▼──────┐
│  Regional   │  │  Regional   │  ...
│  Plane A    │  │  Plane B    │
└──────┬──────┘  └──────┬──────┘
   mTLS poll         mTLS poll
devices/agents     devices/agents
```

The global-plane is **additive and non-load-bearing**: regional planes operate independently if the global plane is unreachable.

---

## Running the Global Plane

### Required environment variables

| Variable | Description |
|---|---|
| `GLOBAL_DATABASE_URL` | Postgres connection string for the global-plane DB (separate from regional DBs) |
| `AUTH_JWT_SECRET` | JWT signing secret for the global-plane local auth |
| `GLOBAL_TOKEN_ENCRYPTION_KEY` | Base64-encoded 32-byte AES-256 key for encrypting regional service tokens at rest |

Generate an encryption key:
```bash
openssl rand -base64 32
```

### Optional variables

| Variable | Default | Description |
|---|---|---|
| `GLOBAL_HTTP_ADDR` | `:8090` | Listen address |
| `GLOBAL_MIGRATIONS_DIR` | `migrations/global` | Path to global-plane SQL migrations |
| `GLOBAL_SYNC_DEFAULT_INTERVAL` | `60` | Default sync interval in seconds (min 10) |
| `CORS_ALLOWED_ORIGINS` | _(none)_ | Comma-separated allowed origins |
| `ENABLE_TLS` | `0` | Set to `1` to enable TLS |
| `TLS_CERT_PATH` | _(none)_ | TLS certificate path (when `ENABLE_TLS=1`) |
| `TLS_KEY_PATH` | _(none)_ | TLS key path (when `ENABLE_TLS=1`) |

### Run

```bash
export GLOBAL_DATABASE_URL=postgres://parcel:parcel@localhost:5432/parcel_global?sslmode=disable
export AUTH_JWT_SECRET=<your-secret>
export GLOBAL_TOKEN_ENCRYPTION_KEY=$(openssl rand -base64 32)
./global-plane
```

The binary runs migrations automatically on startup, then starts background sync goroutines and the HTTP server.

---

## Registering a Regional Plane

### Step 1 — Create a service token on the regional plane

On the regional control plane, create a service token with `device.read` and `artifact.read` scopes (or `federation.push` for explicit federation intent):

```bash
curl -X POST https://<regional-host>/api/v1/auth/service-tokens \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"name": "global-plane-sync", "scopes": ["device.read", "artifact.read"]}'
```

Copy the returned `token` value. It is shown only once.

### Step 2 — Register the plane on the global-plane

```bash
curl -X POST https://<global-host>/api/v1/planes \
  -H "Authorization: Bearer <global-admin-jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "us-east-1",
    "baseUrl": "https://<regional-host>",
    "serviceToken": "<token-from-step-1>",
    "syncIntervalSeconds": 60
  }'
```

For regional planes with self-signed CAs, include the CA PEM in `tlsCaPem`:

```json
{
  "name": "on-prem-lab",
  "baseUrl": "https://192.168.1.10:8080",
  "serviceToken": "...",
  "tlsCaPem": "-----BEGIN CERTIFICATE-----\n..."
}
```

The global-plane immediately starts a sync goroutine for the new plane.

---

## API Endpoints

All routes are under `/api/v1`. Authentication uses the same JWT/service-token mechanism as regional planes.

### Plane management

| Method | Path | Access | Description |
|---|---|---|---|
| `GET` | `/planes` | admin or `federation.manage` scope | List all registered regional planes with sync status |
| `POST` | `/planes` | admin or `federation.manage` scope | Register a new regional plane |
| `GET` | `/planes/{planeId}` | admin or `federation.manage` scope | Get a specific plane |
| `PATCH` | `/planes/{planeId}` | admin | Update plane configuration or service token |
| `DELETE` | `/planes/{planeId}` | admin | Remove a plane (cascades cache deletion) |

### Aggregated data

| Method | Path | Access | Description |
|---|---|---|---|
| `GET` | `/devices` | viewer or `device.read` scope | Aggregated device directory across all planes |
| `GET` | `/devices?planeId=<id>` | viewer or `device.read` scope | Filter to a specific plane |
| `GET` | `/health/summary` | viewer or `device.read` scope | Aggregated health roll-up with per-plane breakdown |
| `GET` | `/artifacts` | viewer or `artifact.read` scope | Aggregated artifact list across all planes |
| `GET` | `/artifacts?planeId=<id>` | viewer or `artifact.read` scope | Filter to a specific plane |
| `GET` | `/healthz` | no auth | Liveness check |

---

## Sync Behaviour

- Each registered plane gets its own background goroutine polling at `syncIntervalSeconds`.
- On each poll cycle: devices → health summary → artifacts are fetched and upserted into the cache.
- If a poll fails, the error is recorded in `lastSyncError` on the plane record. The goroutine applies exponential backoff (10s base, max 5 minutes) before retrying.
- `lastSyncAt` reflects the last successful sync. A stale `lastSyncAt` combined with a non-empty `lastSyncError` indicates a connectivity or auth problem with that regional plane.
- Disabling or deleting a plane stops its goroutine immediately.

---

## Sync Status Check

```bash
curl -s https://<global-host>/api/v1/planes \
  -H "Authorization: Bearer <token>" | jq '.[] | {name, lastSyncAt, lastSyncError}'
```

---

## What Is Not Included in E1

The global-plane is **read-only**. It does not:
- Accept artifact uploads
- Set desired state on regional planes
- Issue device enrollment profiles
- Replicate artifact blobs (that is E2)
- Push global policies (that is E3)

These capabilities are planned for E2–E4.
