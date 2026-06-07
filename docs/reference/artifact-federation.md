# Artifact Federation (E2)

Artifact Federation lets operators upload a single artifact to the **global plane** and have its metadata (and optionally its blob) propagated to every registered **regional plane** automatically.

## Architecture

```
Operator
  │
  │  POST /api/v1/federation/artifacts/upload  (multipart/form-data)
  ▼
Global Plane ──── stores blob ──▶ Global MinIO
  │
  │  fan-out goroutine per enabled regional plane
  │  POST /api/v1/federation/artifacts  (JSON, bearer = encrypted service token)
  ▼
Regional Plane ──── records metadata ──▶ federation_artifact_ingest table
  │
  │  (background: operator configures MinIO bucket replication)
  ▼
Regional MinIO (blob replicated under same object key)
```

### Replication status tracking

The global plane tracks per-region status in `artifact_replication_status`:

| phase | description |
|-------|-------------|
| `pending` | row created, metadata push not yet attempted |
| `replicating` | metadata pushed, blob not yet confirmed |
| `confirmed` | regional plane reports blob present locally |

A background **replication reconciler** runs every 60 seconds and polls each regional plane's `GET /api/v1/federation/artifacts/{id}/blob-status`. When a regional plane confirms the blob, the reconciler updates the row to `confirmed`.

## Global-plane API

### Upload artifact

```
POST /api/v1/federation/artifacts/upload
Content-Type: multipart/form-data

Fields:
  name     string   required
  version  string   required
  type     string   required  (app_bundle | config_bundle | data_bundle | firmware | container_image)
  file     binary   required  (.tar.gz bundle)
```

Response `201 Created`:
```json
{
  "artifactId": "uuid",
  "name": "my-app",
  "version": "1.0.0",
  "sha256": "abc123...",
  "sizeBytes": 1048576,
  "totalRegions": 3
}
```

Requires operator role. MinIO must be configured (`GLOBAL_MINIO_ENDPOINT`).

### List federated artifacts

```
GET /api/v1/federation/artifacts[?name=<filter>]
```

Returns all global artifacts with per-artifact `confirmedRegions` / `totalRegions` counts.

### Get single artifact

```
GET /api/v1/federation/artifacts/{artifactId}
```

### Replication status

```
GET /api/v1/federation/artifacts/{artifactId}/replication-status
```

Response:
```json
{
  "artifactId": "uuid",
  "confirmedRegions": 2,
  "totalRegions": 3,
  "regions": [
    {
      "planeId": "...",
      "metadataPushedAt": "...",
      "blobStatus": "confirmed",
      "blobConfirmedAt": "..."
    }
  ]
}
```

### Presign download URL

```
GET /api/v1/federation/artifacts/{artifactId}/presign
```

Returns a short-lived presigned download URL from global MinIO. Expiry controlled by `GLOBAL_PRESIGN_EXPIRES_SECONDS` (default 3600).

## Regional-plane API (called by global plane)

These routes require a service token with `federation.push` scope.

### Receive artifact metadata

```
POST /api/v1/federation/artifacts
Authorization: Bearer <service-token-with-federation.push>
Content-Type: application/json

{
  "artifactId": "uuid",
  "globalObjectKey": "artifacts/uuid.tar.gz",
  "globalPresignBaseUrl": "https://global-plane-host:8090"
}
```

Upserts a row into `federation_artifact_ingest`.

### Blob status

```
GET /api/v1/federation/artifacts/{artifactId}/blob-status
Authorization: Bearer <service-token-with-federation.push>
```

Response:
```json
{ "confirmed": true }
```

The regional plane checks its local MinIO for `globalObjectKey`. If found, it marks the ingest record as confirmed and returns `true`.

## Configuration

### Global-plane environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GLOBAL_MINIO_ENDPOINT` | — | MinIO host:port (e.g. `minio:9000`). Required for uploads. |
| `GLOBAL_MINIO_ACCESS_KEY` | — | MinIO access key. Leave empty to use IAM role. |
| `GLOBAL_MINIO_SECRET_KEY` | — | MinIO secret key. |
| `GLOBAL_MINIO_BUCKET` | `global-artifacts` | Bucket name for artifact blobs. |
| `GLOBAL_MINIO_USE_TLS` | `0` | Set `1` for HTTPS MinIO. |
| `GLOBAL_PRESIGN_EXPIRES_SECONDS` | `3600` | Presigned URL TTL. |
| `GLOBAL_PUBLIC_BASE_URL` | — | Externally-reachable URL sent to regional planes as the presign base. E.g. `https://global-plane.example.com`. |

### Regional-plane service token

The global plane uses the service tokens already stored (encrypted) when a regional plane is registered. To allow the global plane to push federation metadata, those tokens must have the `federation.push` scope.

When registering a regional plane, create a service token on the regional CP first:

```bash
# On the regional control plane
curl -X POST https://regional-cp/api/v1/auth/service-tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"name":"global-plane-federation","scopes":["device.read","artifact.read","federation.push"]}'
```

Then register that token in the global plane via **Register plane** in the UI or via:

```bash
curl -X POST https://global-plane/api/v1/planes \
  -H "Authorization: Bearer $GLOBAL_ADMIN_TOKEN" \
  -d '{"name":"us-east-1","baseUrl":"https://regional-cp","serviceToken":"<token>","syncIntervalSeconds":60}'
```

## Blob replication

E2 does **not** implement an active blob-pull agent. Blob replication must be arranged externally, for example via:

- **MinIO bucket replication** — configure bidirectional or unidirectional replication between the global MinIO and regional MinIO instances using MinIO's built-in replication rules. The regional plane will automatically confirm the blob when `StatObject` succeeds on the replicated key.
- **Manual transfer** — operators can download the artifact via the global presign URL and upload it to the regional MinIO under the same object key.

Once the blob arrives in regional MinIO under `globalObjectKey`, the next reconciler poll (≤60 s) will confirm it and update the replication status.
