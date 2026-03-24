# Cloud-Native Pull Adapters (S3, GCS) and Vault Credentials

HardwareOps can pull artifacts directly from Amazon S3 and Google Cloud Storage in addition to the existing HTTP and Artifactory adapters. Credentials for all pull sources — including the new cloud adapters — can be backed by HashiCorp Vault KV in addition to static files and AWS Secrets Manager.

---

## 1) Pull adapter overview

The pull endpoint (`POST /api/v1/artifacts/pull`) accepts a `source` block that selects the adapter:

```json
{
  "source": {
    "kind": "s3",
    "uri": "s3://my-bucket/releases/firmware-v2.tar.gz",
    "credentialRef": "prod-s3-creds"
  }
}
```

Supported `source.kind` values:

| Kind | URI scheme | Use case |
|---|---|---|
| `http` | `https://` or `http://` | Generic HTTPS artifact server |
| `artifactory` | `https://` or `http://` | JFrog Artifactory (Artifactory-specific auth headers) |
| `s3` | `s3://bucket/key` | Amazon S3 |
| `gcs` | `gs://bucket/object` or `gcs://bucket/object` | Google Cloud Storage |

---

## 2) Amazon S3 adapter

### 2.1 URI format

```
s3://bucket-name/path/to/artifact.tar.gz
```

Bucket and key are parsed from the URI. Region is taken from the credential entry or defaults to `us-east-1`.

### 2.2 Credential keys

Create a credential entry using any of the supported credential sources (static file, AWS Secrets Manager, Vault). The credential values for an S3 entry:

| Key | Required | Description |
|---|---|---|
| `access_key_id` | for static auth | AWS access key ID |
| `secret_access_key` | for static auth | AWS secret access key |
| `session_token` | no | AWS session token (for assumed roles or temporary credentials) |
| `region` | no | AWS region (default: `us-east-1`) |

When `access_key_id` and `secret_access_key` are **not** provided, the adapter uses the AWS SDK default credential chain: environment variables → `~/.aws/credentials` → ECS task role → EC2 instance profile → IRSA (EKS). This is the recommended configuration for AWS-native deployments.

### 2.3 Static credential entry (file or inline JSON)

```json
{
  "prod-s3-creds": {
    "access_key_id": "AKIAIOSFODNN7EXAMPLE",
    "secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    "region": "us-west-2"
  }
}
```

Point to this file with `ARTIFACT_PULL_CREDENTIALS_FILE=/etc/hardwareops/pull-creds.json`.

### 2.4 IAM-native (no credential entry required)

In ECS, EKS (IRSA), or EC2 deployments, leave `credentialRef` empty or omit it entirely:

```json
{
  "source": {
    "kind": "s3",
    "uri": "s3://prod-artifacts/firmware-v2.tar.gz"
  }
}
```

The adapter picks up credentials from the task role, pod identity, or instance profile automatically.

### 2.5 Full pull example

```bash
SHA256=$(sha256sum firmware-v2.tar.gz | awk '{print $1}')

curl -X POST https://hardwareops.internal/api/v1/artifacts/pull \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "firmware",
    "version": "2.0.0",
    "type": "firmware",
    "sha256": "'"$SHA256"'",
    "sizeBytes": 4194304,
    "source": {
      "kind": "s3",
      "uri": "s3://prod-artifacts/firmware-v2.tar.gz",
      "credentialRef": "prod-s3-creds"
    }
  }'
```

---

## 3) Google Cloud Storage adapter

### 3.1 URI format

Both schemes are accepted:

```
gs://bucket-name/path/to/artifact.tar.gz
gcs://bucket-name/path/to/artifact.tar.gz
```

### 3.2 Credential keys

| Key | Description |
|---|---|
| `oauth_token` | Pre-obtained OAuth2 bearer token. Short-lived; useful for testing or scripted one-shot pulls. |
| `service_account_json` | Full contents of a Google service account JSON key file. The adapter uses it to obtain tokens automatically. |

When neither key is present, the adapter uses **Application Default Credentials**: GKE Workload Identity, GCE instance service account, or the `GOOGLE_APPLICATION_CREDENTIALS` environment variable.

### 3.3 Service account credential entry

```json
{
  "prod-gcs-creds": {
    "service_account_json": "{\"type\":\"service_account\",\"project_id\":\"acme-prod\",\"private_key_id\":\"...\",\"private_key\":\"-----BEGIN RSA PRIVATE KEY-----\\n...\"}"
  }
}
```

For multiline keys, ensure newlines within the JSON string are escaped as `\n`.

### 3.4 Workload Identity (GKE, no credential entry required)

In GKE with Workload Identity configured, omit `credentialRef`:

```json
{
  "source": {
    "kind": "gcs",
    "uri": "gs://prod-artifacts/firmware-v2.tar.gz"
  }
}
```

### 3.5 Full pull example

```bash
curl -X POST https://hardwareops.internal/api/v1/artifacts/pull \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "firmware",
    "version": "2.0.0",
    "type": "firmware",
    "sha256": "abc123...",
    "sizeBytes": 4194304,
    "source": {
      "kind": "gcs",
      "uri": "gs://prod-artifacts/releases/firmware-v2.tar.gz",
      "credentialRef": "prod-gcs-creds"
    }
  }'
```

---

## 4) HashiCorp Vault credential backend

In addition to static files and AWS Secrets Manager, pull credentials can be loaded from **Vault KV v2**. The Vault backend merges with other sources — you can combine Vault entries with static fallbacks.

### 4.1 Environment variables

| Variable | Required | Description |
|---|---|---|
| `ARTIFACT_PULL_CREDENTIALS_VAULT_ADDR` | yes (Vault) | Vault server address, e.g. `https://vault.internal:8200` |
| `ARTIFACT_PULL_CREDENTIALS_VAULT_TOKEN` | yes (Vault) | Vault token with `read` permission on the KV path |
| `ARTIFACT_PULL_CREDENTIALS_VAULT_PATH` | yes (Vault) | KV v2 secret path, e.g. `secret/data/hardwareops/pull-creds` |

Vault and AWS Secrets Manager backends are mutually exclusive. Inline JSON/file credentials can coexist with either.

### 4.2 Vault secret format

The secret at the configured path must contain a `data` object (KV v2 envelope) whose value is the same JSON map format used by the file-based backend:

```
# Write the secret (Vault CLI)
vault kv put secret/hardwareops/pull-creds \
  data='{"prod-s3-creds":{"access_key_id":"AKIA...","secret_access_key":"...","region":"us-west-2"},"prod-gcs-creds":{"service_account_json":"..."}}'
```

The control-plane reads `GET <vault_addr>/v1/<vault_path>` and extracts the `data.data` field.

### 4.3 Startup behavior

On control-plane startup, credentials are loaded from all configured sources. If Vault is unreachable at startup, the startup fails with an error. After startup, credentials in memory are used for all pull requests until the next reload.

### 4.4 Reloading credentials

Trigger a reload without restarting the control-plane:

```bash
curl -X POST https://hardwareops.internal/api/v1/artifacts/pull-credentials/reload \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

Response includes `vault_backed`, `vault_addr`, `vault_path`, and per-source ref counts:

```json
{
  "configured": true,
  "vault_backed": true,
  "vault_addr": "https://vault.internal:8200",
  "vault_path": "secret/data/hardwareops/pull-creds",
  "resolverAvailable": true,
  "credentialRefCount": 3,
  "credentialRefs": ["prod-gcs-creds", "prod-s3-creds", "staging-s3-creds"],
  "vaultCredentialRefs": 3,
  "lastLoadedAt": "2026-03-17T12:00:00Z"
}
```

### 4.5 Credential rotation runbook

1. Update the Vault secret with new credentials:
   ```bash
   vault kv put secret/hardwareops/pull-creds data='{"prod-s3-creds":{...new...}}'
   ```
2. Trigger a reload (no restart required):
   ```bash
   curl -X POST https://hardwareops.internal/api/v1/artifacts/pull-credentials/reload \
     -H "Authorization: Bearer $ADMIN_TOKEN"
   ```
3. Verify the reload audit event in `GET /api/v1/audit` (action: `artifact.pull_credentials.reload`).
4. Test a pull ingest with the updated credentials.

---

## 5) Credential status and monitoring

```bash
curl https://hardwareops.internal/api/v1/artifacts/pull-credentials \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

Key fields to monitor:

| Field | What to check |
|---|---|
| `configured` | `true` if at least one credential source is configured |
| `resolverAvailable` | `true` if credentials were loaded successfully |
| `credentialRefCount` | Number of named credential refs available |
| `lastLoadedAt` | Timestamp of the last successful load |
| `vault_backed` | `true` when Vault backend is active |

Alert if `resolverAvailable` becomes `false` after a successful startup — indicates a reload failure.

---

## 6) Security notes

- Credential values are never returned by the API — only ref names and counts.
- The `vault_token` should be scoped to `read` on the specific KV path only.
- Rotate Vault tokens before expiry; Vault dynamic credentials with short TTLs are recommended over long-lived static tokens.
- The `credentialRef` in pull requests is an opaque string — it is logged in audit events but credential values are not.
- S3: Prefer IAM roles/IRSA over static credentials. If static credentials are required, scope them to `s3:GetObject` on the specific bucket only.
- GCS: Prefer Workload Identity over service account key files. If a key file is required, scope the service account to `Storage Object Viewer` on the specific bucket only.

---

## Related docs

- `docs/development/artifact-ingest.md` — full pull ingest workflow and adapter framework
- `docs/development/phase-c-internals.md` — adapter implementation details
- `docs/icd.md` — API contract for pull and credential endpoints
- `docs/operations.md` — pull credential reload operations procedure
