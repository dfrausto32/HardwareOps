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

### Bootstrap admin (local auth)

To seed the first operator account on first run set:

| Variable | Description |
|---|---|
| `AUTH_BOOTSTRAP_EMAIL` | Email for the initial admin user |
| `AUTH_BOOTSTRAP_PASSWORD` | Password for the initial admin user |

`EnsureBootstrapAdmin` is idempotent: it is a no-op once any user exists, so it is safe to leave these set in production configs.

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
| `TRUST_PROXY` | `0` | Set to `1` to honor `X-Forwarded-For`/`X-Real-IP` headers for source IP in audit events |

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

## Operator Authentication

The global-plane uses the same JWT / service-token mechanism as regional planes, with a distinct issuer (`parcel-global`) so a regional JWT is rejected.

### Login (local auth)

```bash
# Obtain a JWT for the bootstrap admin (or any local user).
curl -X POST https://<global-host>/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "<password>"}'
# → {"token":"<jwt>","expiresAt":"..."}
```

The login endpoint enforces progressive backoff after repeated failures (5-attempt threshold, 30s–15m backoff window). All login outcomes are written to the global audit log.

### SSO (OIDC)

Set the following env vars alongside the existing global-plane config (same names as the regional control-plane):

| Variable | Required | Description |
|---|---|---|
| `AUTH_OIDC_ISSUER` | yes | IdP discovery URL (e.g. `https://accounts.google.com`) |
| `AUTH_OIDC_CLIENT_ID` | yes | OAuth2 client ID |
| `AUTH_OIDC_CLIENT_SECRET` | yes | OAuth2 client secret |
| `AUTH_OIDC_REDIRECT_URL` | yes | Callback URL: `https://<global-host>/api/v1/auth/oidc/callback` |
| `AUTH_OIDC_ROLE_MAP` | no | JSON map of IdP group → role, e.g. `{"platform-admins":"admin"}` |
| `AUTH_OIDC_DEFAULT_ROLE` | no | Role for unmapped users (default: `viewer`) |
| `AUTH_OIDC_GROUP_CLAIM` | no | JWT claim for group membership (default: `groups`) |
| `GLOBAL_POST_LOGIN_URL` | no | Where to redirect after SSO (typically the regional UI origin) |

Works with any OIDC IdP: Okta, Azure AD (Entra), Google Workspace, Auth0, Keycloak, etc.

**OIDC callback flow:**
1. Operator navigates to `GET /api/v1/auth/oidc/login` (or clicks "Sign in with SSO" in the UI)
2. Browser redirected to IdP → operator authenticates
3. IdP redirects to `GET /api/v1/auth/oidc/callback?code=...&state=...`
4. Global-plane exchanges code → upserts user with mapped role → issues JWT
5. If `GLOBAL_POST_LOGIN_URL` is set: redirects to `<post-login-url>?global_oidc_token=<jwt>`
6. The regional UI's GlobalPage reads `global_oidc_token` from the URL and stores it

Use the returned JWT as `Authorization: Bearer <jwt>` on all subsequent requests.

---

## API Endpoints

All routes are under `/api/v1`. Authentication uses the same JWT/service-token mechanism as regional planes.

### Auth

| Method | Path | Access | Description |
|---|---|---|---|
| `POST` | `/auth/login` | public | Local-auth login; returns JWT |
| `GET` | `/auth/status` | public | Whether local auth is enabled |

### Audit log

| Method | Path | Access | Description |
|---|---|---|---|
| `GET` | `/audit` | viewer+ | Query audit events (filter by action, actor, target, time range) |
| `GET` | `/audit/export` | viewer+ | Download audit log as CSV |

Query params for `/audit`: `action`, `actorType`, `actorId`, `actorEmail`, `targetType`, `targetId`, `status`, `since` (RFC3339), `until` (RFC3339), `limit` (default 200, max 5000), `offset`.

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
