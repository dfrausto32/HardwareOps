# Phase C Feature Internals

Developer reference for four features shipped in Phase C: LDAP/AD authentication, HashiCorp Vault credential backend, Supply-chain provenance policy (Cosign/Sigstore), and Cloud-native pull adapters (S3/GCS).

---

## 1) LDAP/AD Authentication

### 1.1 Code layout

| File | Role |
|---|---|
| `internal/auth/ldap.go` | `LDAPProvider` struct, `Authenticate()`, `mapRole()` |
| `internal/httpapi/handlers/auth_ldap.go` | `POST /api/v1/auth/ldap/login` handler |
| `internal/httpapi/handlers/auth.go` | `AuthStatusResponse.ldapEnabled` field |
| `internal/config/config.go` | `AUTH_LDAP_*` env var loading and `LDAPEnabled()` helper |
| `internal/httpapi/deps.go` | `Dependencies.LDAPProvider` |
| `internal/httpapi/router.go` | Conditional route registration |
| `ui/src/api.ts` | `ldapLogin()` |
| `ui/src/App.jsx` | Conditional LDAP tab |

### 1.2 Authentication flow

`LDAPProvider.Authenticate(ctx, username, password)` executes in five steps:

1. **Service-account bind** — opens an LDAP connection to `AUTH_LDAP_URL` and binds with `AUTH_LDAP_BIND_DN` + `AUTH_LDAP_BIND_PASSWORD`. This is a read-only service account used for directory searches.

2. **User search** — searches `AUTH_LDAP_BASE_DN` using the filter `AUTH_LDAP_USER_FILTER` (default `(uid=%s)` with `%s` replaced by the provided username). Fetches `mail`, `displayName`, and `memberOf` (or the configured attribute names). Returns `ErrLDAPInvalidCredentials` if no result.

3. **Credential rebind** — opens a second LDAP connection and binds with the found user DN + the supplied password. A bind failure here means wrong credentials; returns `ErrLDAPInvalidCredentials`.

4. **Store upsert** — calls `store.GetUserByExternalID("ldap", userDN)`. Falls back to `GetUserByEmail(mail)` if not found by DN (handles accounts created before LDAP was configured). If still not found, calls `CreateUser(...)` with `auth_provider = "ldap"`, `external_id = userDN`. Rejects users with `disabled = true` regardless of directory state.

5. **Token issuance** — calls `manager.IssueToken(user)` to produce an identical JWT to local/OIDC login. Updates `last_login_at`.

### 1.3 Role mapping

`mapRole(groups []string) string` iterates the user's `memberOf` values against `roleMap` (parsed from `AUTH_LDAP_ROLE_MAP`). The first key that exactly matches a group DN wins. If no match, returns `defaultRole`.

The role map is a `map[string]string` in memory — the JSON value is parsed once at startup via `NewLDAPProvider`.

### 1.4 Library

`go-ldap/ldap/v3` (MIT) is the only new dependency. It handles the LDAP wire protocol, search requests, and rebind. There is no connection pooling in the initial implementation — each `Authenticate` call opens and closes connections. This is intentional simplicity; pooling can be added if latency under load becomes an issue.

### 1.5 Conditional route registration

In `router.go`, the LDAP route is only registered when `deps.LDAPProvider != nil`:

```go
if deps.LDAPProvider != nil {
    r.With(loginLimiter.Middleware).Post("/auth/ldap/login",
        handlers.LDAPLogin(logger, deps.LDAPProvider, deps.Store, deps.TrustProxy))
}
```

`GET /api/v1/auth/status` returns `ldapEnabled: true` when the provider is non-nil, which the UI uses to conditionally render the LDAP login tab.

### 1.6 Audit events

| Action | Emitted when |
|---|---|
| `user.login` | Successful LDAP authentication |
| `user.login_failed` | Invalid credentials, user not found, or account disabled |

Both events include the IP (via `trustProxy` handling) and the `username` field in metadata.

---

## 2) HashiCorp Vault Credential Backend

### 2.1 Code layout

| File | Role |
|---|---|
| `internal/artifactingest/credentials_vault.go` | `LoadStaticCredentialsFromVault()` HTTP loader |
| `internal/artifactingest/credential_manager.go` | `Reload()` Vault branch; extended `PullCredentialStatus` |
| `internal/config/config.go` | `ARTIFACT_PULL_CREDENTIALS_VAULT_*` env vars |

### 2.2 Vault KV v2 HTTP call

The loader makes a single `GET <vault_addr>/v1/<vault_path>` with `X-Vault-Token` header. The response is a standard KV v2 envelope:

```json
{
  "data": {
    "data": {
      "ref-name-1": {"access_key_id": "...", "secret_access_key": "..."},
      "ref-name-2": {"service_account_json": "..."}
    }
  }
}
```

The loader extracts `data.data` and returns it as `map[string]map[string]string`, which is the same shape as the static file loader's output. No Vault SDK is used — just `net/http`.

### 2.3 Credential merge order

`Reload()` loads from sources in this order and merges with `MergeStaticCredentialSets`:

1. Static (file + inline JSON)
2. AWS Secrets Manager
3. Vault

Later sources override earlier ones for the same ref name. This allows a Vault secret to shadow a static fallback.

### 2.4 Extension points

To add a new credential backend, implement a function with this signature:

```go
func LoadStaticCredentialsFromMySource(ctx context.Context, ...config args) (map[string]map[string]string, error)
```

Then add a branch in `Reload()` alongside the existing three, appending to `sets` and updating `status`.

---

## 3) Supply-Chain Provenance Policy (Cosign/Sigstore)

### 3.1 Code layout

| File | Role |
|---|---|
| `internal/artifacttrust/provenance.go` | `VerifyKeylessSignature`, `CheckProvenancePolicy`, helpers |
| `internal/artifacttrust/trust.go` | `SignatureTypeKeyless`, `ArtifactPolicyInput`, `VerifyArtifactKeyless`, `ResolvedPolicy.Provenance`, `PolicyOverride` provenance fields |
| `internal/store/store.go` | `AttestationRecord`, `ProvenancePolicy`, `ArtifactTrustPolicy.ProvenancePolicyJSON`, store interface methods |
| `internal/store/postgres/postgres.go` | Attestation CRUD + trust policy provenance column |
| `internal/store/memory/memory.go` | In-memory attestation CRUD |
| `internal/httpapi/handlers/attestations.go` | `POST/GET /api/v1/artifacts/{id}/attestations` handlers |
| `internal/httpapi/handlers/artifact_trust.go` | Trust policy request/response extended with provenance fields |
| `migrations/0028_artifact_attestations.sql` | DB schema: `artifact_attestations` table, `provenance_policy` column on `artifact_trust_policy` |

### 3.2 Keyless signature verification (`VerifyKeylessSignature`)

The flow mirrors what `cosign verify --keyless` does, implemented in pure stdlib:

1. **Parse Fulcio cert** — `pem.Decode` + `x509.ParseCertificate` on `bundle.Cert`.

2. **Chain validation** — `cert.Verify(x509.VerifyOptions{Roots: fulcioRoots, CurrentTime: cert.NotBefore})`. Setting `CurrentTime = cert.NotBefore` avoids false expiry rejections when verifying historical artifacts (Fulcio certs are ~10-minute short-lived).

3. **Identity extraction** (`extractFulcioIdentity`) — reads two custom OID extensions:
   - `1.3.6.1.4.1.57264.1.1` (issuer v1): raw ASCII bytes
   - `1.3.6.1.4.1.57264.1.8` (issuer v2): ASN.1 UTF8String

   Builder subject comes from the first URI SAN (`cert.URIs[0].String()`), which holds the OIDC job-subject claim (e.g. a GitHub Actions workflow ref URI).

4. **ECDSA signature verify** — `ecdsa.VerifyASN1(pub, digestBytes, sigBytes)` where `digestBytes` is the raw SHA-256 hash of the artifact (decoded from the hex digest string).

5. **Rekor verification** (optional) — if `bundle.RekorBundle != nil`, validates that `signedEntryTimestamp` is non-empty valid base64 and `logIndex >= 0`. If `opts.RequireRekorLog` is set and no offline bundle is present, performs a live `POST /api/v1/index/retrieve` search against the configured Rekor URL.

No external dependencies are used — only `crypto/x509`, `crypto/ecdsa`, `encoding/asn1`, and `net/http`.

### 3.3 Policy evaluation

`CheckProvenancePolicy(attestations []AttestationRecord, policy ProvenancePolicy) error` is a linear scan:

- Returns nil immediately if `!policy.RequireProvenance`.
- Returns an error if `len(attestations) == 0`.
- Iterates attestations; for each, checks all non-empty policy fields (exact match for predicate type and issuer; `matchesBuilderPattern` for builder ID). Returns nil on the first attestation that satisfies all constraints.
- Returns an error if no attestation satisfies the policy.

`matchesBuilderPattern(builderID, pattern string) bool` checks for trailing `*` wildcard; otherwise exact match.

### 3.4 Policy flow at desired-state set time

`handlers.ValidateDesiredArtifactWithAttestations` → `artifacttrust.ArtifactAllowedByPolicy(ArtifactPolicyInput, ResolvedPolicy)`:

1. Standard signature policy checks (verification mode, allowed key IDs, allowed types).
2. `CheckProvenancePolicy(input.Attestations, policy.Provenance)`.

Attestations are fetched with `st.ListAttestations(artifactID)` in `desired_state.go` before each policy check. This is a synchronous DB read at request time; it is not cached.

### 3.5 Keyless ingest path (`verifyStoredArtifact`)

When `signatureType == "keyless"`, the existing `verifyStoredArtifact` helper bypasses the trusted signing key registry and calls `VerifyArtifactKeyless(signature, artifactSHA, sigPolicy.KeylessOpts)` directly. The `signature` field contains the full JSON-encoded `KeylessCosignBundle`. On success, `VerificationResult.Status = verified`.

`KeylessVerifyOptions` is carried in `ArtifactSignaturePolicy.KeylessOpts`, which is populated in `router.go` from `deps.ArtifactFulcioRootCert`, `deps.ArtifactRekorURL`, and `deps.ArtifactRequireRekorLog`.

### 3.6 Database schema notes

`artifact_attestations` indexes:
- Primary: `attestation_id` (uuid, gen_random_uuid)
- `artifact_attestations_artifact_id_idx`: covers the common `ListAttestations(artifactID)` query
- `artifact_attestations_builder_id_idx`: partial index (`WHERE builder_id IS NOT NULL`) for future builder-scoped queries

`provenance_policy` on `artifact_trust_policy` is a nullable JSONB column added via `ADD COLUMN IF NOT EXISTS` — backward-compatible with existing policies.

### 3.7 Testing

`internal/artifacttrust` package tests cover `CheckProvenancePolicy`, `matchesBuilderPattern`, `VerifyArtifactKeyless` (with mock bundles), and policy resolution including provenance override merging.

For integration testing against real Sigstore:
```bash
./scripts/test-artifact-trust.sh
```

---

## 4) Cloud-Native Pull Adapters (S3 / GCS)

### 4.1 Code layout

| File | Role |
|---|---|
| `internal/artifactingest/s3_pull_adapter.go` | `S3PullAdapter`, `parseS3URI`, `buildS3Client` |
| `internal/artifactingest/gcs_pull_adapter.go` | `GCSPullAdapter`, `parseGCSURI`, `buildGCSTokenSource` |
| `internal/artifactingest/s3_pull_adapter_test.go` | URI parsing, mock S3 client, credential forwarding |
| `internal/artifactingest/gcs_pull_adapter_test.go` | URI parsing, token source variants, error paths |
| `internal/httpapi/handlers/artifacts.go` | Registry registration in `pullArtifact()` |

### 4.2 PullAdapter interface contract

Both adapters implement:

```go
type PullAdapter interface {
    Kind() string
    Pull(ctx context.Context, req PullRequest) (PullResponse, error)
}
```

`PullResponse.Body` must be closed by the caller. The body is the raw artifact byte stream; SHA-256 verification and size enforcement happen in the handler after the adapter returns.

The adapter is responsible for:
- URI validation
- Credential application
- Making the remote request
- Returning a `PullResponse` with `Body`, `ContentType`, and `ContentLength`

The adapter is **not** responsible for checksum verification, size limits, or object store upload — those are all handled by the calling handler.

### 4.3 S3 adapter implementation details

**URI parsing** (`parseS3URI`): strict — rejects anything that doesn't start with `s3://`. Returns separate bucket and key strings. The key preserves path separators (`s3://bucket/path/to/key` → key = `path/to/key`).

**Client construction** (`buildS3Client`): uses `aws-sdk-go-v2/config.LoadDefaultConfig` with an optional `StaticCredentialsProvider` injected when `access_key_id` and `secret_access_key` are present in the resolved credentials. When not present, the SDK default chain is used in its entirety (env vars → shared credentials file → ECS task role → EC2/EKS metadata).

**Testability**: `S3PullAdapter.newClient` is a factory field (`func(ctx, Credentials) (s3ObjectGetter, error)`) that defaults to `buildS3Client`. Tests inject a mock `s3ObjectGetter`:

```go
type s3ObjectGetter interface {
    GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}
```

This avoids any network calls or AWS credential requirements in unit tests.

### 4.4 GCS adapter implementation details

**URI parsing** (`parseGCSURI`): accepts both `gs://` and `gcs://` schemes. Returns separate bucket and object strings; the object is passed through `url.PathEscape` before being appended to the XML API URL.

**Authentication** (`buildGCSTokenSource`): three paths in priority order:
1. `oauth_token` key → `oauth2.StaticTokenSource` (used for testing and one-shot scripts)
2. `service_account_json` key → `google.CredentialsFromJSON(ctx, []byte(saJSON), gcsScopeReadOnly)`
3. No creds → `google.FindDefaultCredentials(ctx, gcsScopeReadOnly)` (GKE Workload Identity, GCE instance SA, `GOOGLE_APPLICATION_CREDENTIALS`)

**Download**: GCS XML API (`GET https://storage.googleapis.com/{bucket}/{url-escaped-object}`). The XML API is simpler than the JSON API for binary downloads and requires only a Bearer token — no Google Cloud Storage SDK needed. OAuth2 scope used: `https://www.googleapis.com/auth/devstorage.read_only`.

**Testability**: `GCSPullAdapter.newTokenSource` is a factory field (`func(ctx, Credentials) (oauth2.TokenSource, error)`). Tests inject `oauth2.StaticTokenSource` to bypass real GCP credential resolution.

### 4.5 Adding a new pull adapter

1. Create `internal/artifactingest/<name>_pull_adapter.go` implementing `PullAdapter`.
2. Create `internal/artifactingest/<name>_pull_adapter_test.go` with an injectable client/transport field for unit tests.
3. Register in `handlers/artifacts.go` inside `pullArtifact()`:
   ```go
   adapterRegistry := artifactingest.NewPullAdapterRegistry(
       // existing adapters ...
       artifactingest.NewMyAdapter(timeout),
   )
   ```
4. Document the URI scheme and credential keys in `docs/cloud-pull-adapters.md`.

No configuration changes are required if the adapter uses the existing `Credentials` map — operators configure credentials through the existing credential resolver system.

### 4.6 Dependencies added in Phase C

| Module | Version | Reason |
|---|---|---|
| `github.com/aws/aws-sdk-go-v2/service/s3` | v1.97.1 | S3 GetObject |
| `github.com/aws/aws-sdk-go-v2/service/internal/s3shared` | v1.19.20 | S3 SDK transitive dep |
| `github.com/aws/aws-sdk-go-v2/internal/v4a` | v1.4.21 | S3 SDK transitive dep |
| `cloud.google.com/go/compute/metadata` | v0.3.0 | `golang.org/x/oauth2/google` transitive dep |

---

## 5) ICD delta (new endpoints)

| Method | Path | Access | Notes |
|---|---|---|---|
| POST | `/api/v1/auth/ldap/login` | public, rate limited | LDAP credential exchange; returns JWT |
| POST | `/api/v1/artifacts/{artifactId}/attestations` | operator | Submit in-toto attestation; keyless bundles verified at upload |
| GET | `/api/v1/artifacts/{artifactId}/attestations` | viewer | List attestations for an artifact |

Existing endpoints with schema changes:
- `GET /api/v1/artifact-trust/policy` — response now includes optional `provenance` object
- `PUT /api/v1/artifact-trust/policy` — request now accepts optional `provenance` object
- `GET /api/v1/auth/status` — response now includes `ldapEnabled: bool`
- `GET /api/v1/artifacts/pull-credentials` — response now includes `vault_backed`, `vault_addr`, `vault_path`, `vaultCredentialRefs`

---

## 6) Vulnerability Scanning (`internal/vulnscan/`)

### 6.1 Package layout

| File | Role |
|---|---|
| `internal/vulnscan/scanner.go` | `Scanner` interface, `ScanRequest`, `ScanResult`, `VulnerabilityFinding`, `SeverityCounts` types |
| `internal/vulnscan/objectstore.go` | `ObjectStore` narrow interface + `downloadToTemp()` helper |
| `internal/vulnscan/grype.go` | `GrypeScanner` — subprocess, parses `grype -o json` output |
| `internal/vulnscan/trivy.go` | `TrivyScanner` — subprocess, parses `trivy fs --format json` output |
| `internal/vulnscan/artifact_scan_job.go` | `ArtifactScanJob` — goroutine-based background runner; writes `artifact_vulnerability_scans` records; emits `artifact.scan_completed` WebSocket event |
| `internal/vulnscan/nessus.go` | `NessusClient` — pure `net/http`; `X-ApiKeys` auth; `ListScans`, `GetScanHosts`, `GetHostFindings` |
| `internal/vulnscan/nessus_sync.go` | `NessusSyncJob` — background ticker; device matching; writes `device_vulnerability_scans`; emits `device.scan_synced` |
| `internal/httpapi/handlers/vuln_scans.go` | 7 HTTP handlers + `NessusSyncJobTrigger` interface |
| `migrations/0029_vulnerability_scans.sql` | DB schema (`artifact_vulnerability_scans`, `device_vulnerability_scans`) |

### 6.2 Scanner interface

```go
type Scanner interface {
    Type() string    // "grype" | "trivy"
    Scan(ctx context.Context, req ScanRequest) (ScanResult, error)
}
```

`ArtifactScanJob` wraps any `Scanner`. It is constructed with a narrow `vulnscan.ObjectStore` interface (subset of `objectstore.MinIO`) so the ingest handlers receive only an `ArtifactVulnScanTrigger` interface (defined in `handlers/artifacts.go`), keeping the `vulnscan` package out of the handler import graph.

### 6.3 Artifact scan flow

```
ingest handler (upload/pull/complete/create)
  └─► emitArtifactRegisteredEvent(...)
  └─► if vulnScan != nil && shouldScanArtifact(artifact.Type, skipTypes)
          vulnScan.Trigger(artifactID, objectKey, sha256)
              └─► goroutine: create pending record → running → scan → completed/failed
                  └─► store.UpdateArtifactVulnScan(...)
                  └─► hub.Broadcast("artifact.scan_completed", ...)
```

The trigger is fire-and-forget; ingest response returns before scan completes.

### 6.4 Nessus sync flow

```
NessusSyncJob.Start(ctx)
  └─► ticker fires every VULN_NESSUS_SYNC_INTERVAL (default 1h)
      └─► NessusClient.ListScans() → filter by VULN_NESSUS_SCAN_IDS if set
          └─► for each scan → GetScanHosts(scanID)
              └─► for each host → match to Parcel device
                  └─► compare nessus host.hostname / host.ip against device.MetadataJSON
                      keys: hwops.network.hostname, hwops.network.ip
                  └─► if matched: GetHostFindings → store.UpsertDeviceVulnScan(...)
                      └─► hub.Broadcast("device.scan_synced", ...)
```

`NessusSyncJob.Trigger()` forces an immediate sync cycle (called by `POST /api/v1/vulnerability-scans/nessus/sync`).

### 6.5 Nil-safe interface pattern (router.go)

When `deps.ArtifactScanJob` is a typed nil `*vulnscan.ArtifactScanJob`, assigning it directly to an interface variable results in a non-nil interface with nil value, defeating the `if vulnScan != nil` guard. The router uses explicit nil checks before interface assignment:

```go
var vulnScanTrigger handlers.ArtifactVulnScanTrigger
if deps.ArtifactScanJob != nil {
    vulnScanTrigger = deps.ArtifactScanJob
}
```

Same pattern applies to `NessusSyncJob` / `NessusSyncJobTrigger`.

### 6.6 Config env vars

| Env var | Default | Notes |
|---|---|---|
| `VULN_ARTIFACT_SCANNER` | `disabled` | `grype` \| `trivy` \| `disabled` |
| `VULN_ARTIFACT_SCANNER_BIN` | `` | Path to binary; defaults to `grype`/`trivy` found in `PATH` |
| `VULN_ARTIFACT_SKIP_TYPES` | `` | Comma-separated artifact types to skip (e.g. `firmware`) |
| `VULN_NESSUS_URL` | `` | Leave blank to disable device scanning |
| `VULN_NESSUS_ACCESS_KEY` | `` | Tenable API access key |
| `VULN_NESSUS_SECRET_KEY` | `` | Tenable API secret key |
| `VULN_NESSUS_SYNC_INTERVAL` | `1h` | Background sync interval |
| `VULN_NESSUS_SCAN_IDS` | `` | Optional comma-separated Nessus scan IDs to include; empty = all |
