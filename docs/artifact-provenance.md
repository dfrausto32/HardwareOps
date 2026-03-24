# Artifact Supply-Chain Provenance

HardwareOps supports **keyless cosign signatures** and **in-toto / SLSA attestations** as an optional layer on top of the existing Ed25519 trusted-key verification. Provenance lets you enforce that artifacts were built by a specific CI pipeline identity before they can be deployed to devices.

---

## 1) Overview

| Concept | What it means in HardwareOps |
|---|---|
| **Keyless signature** | An ECDSA signature produced by `cosign sign --keyless`, backed by a short-lived Fulcio-issued certificate that embeds the signer's OIDC identity (e.g. a GitHub Actions job URL). No long-lived signing key required. |
| **Attestation** | An in-toto statement (SLSA provenance, SBOM, custom predicate) stored alongside an artifact. Attestations can be keyless-verified at upload time so the builder identity is recorded. |
| **Provenance policy** | A per-artifact or global rule that requires at least one attestation matching a given predicate type, builder ID pattern, and/or issuer before an artifact can be assigned to devices. |

Provenance sits alongside — not instead of — the existing signature policy. Both can be required simultaneously.

---

## 2) Configuration

### 2.1 Control-plane environment variables

| Variable | Default | Description |
|---|---|---|
| `ARTIFACT_FULCIO_ROOT_CERT` | Sigstore public Fulcio root | PEM-encoded Fulcio root CA used to validate signing certificates. Leave unset to use the public Sigstore root. Set to your corporate Fulcio root for private deployments. |
| `ARTIFACT_REKOR_URL` | `https://rekor.sigstore.dev` | Rekor transparency log base URL. |
| `ARTIFACT_REQUIRE_REKOR_LOG` | `0` | Set to `1` to require a live Rekor lookup when no offline bundle is embedded in the signature. Adds latency; recommended for high-assurance environments. |

None of these variables are required for the attestation store to work — only for keyless signature verification at upload time.

---

## 3) Uploading a keyless-signed artifact

When you register or pull an artifact with `signatureType = "keyless"`, the control-plane verifies the Fulcio certificate chain and ECDSA signature and records the builder identity.

### 3.1 Using the pull ingest path (recommended for CI)

```bash
# Sign with cosign keyless and capture the bundle
cosign sign-blob \
  --bundle firmware-v2.tar.gz.bundle \
  firmware-v2.tar.gz

# Extract the base64 signature from the bundle
SIGNATURE=$(jq -r '.base64Signature' firmware-v2.tar.gz.bundle)
BUNDLE=$(cat firmware-v2.tar.gz.bundle)

# Pull + register in one step
curl -X POST https://hardwareops.internal/api/v1/artifacts/pull \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "firmware",
    "version": "2.0.0",
    "type": "firmware",
    "source": {
      "kind": "s3",
      "uri": "s3://build-artifacts/firmware-v2.tar.gz"
    },
    "sha256": "'"$SHA256"'",
    "signature": '"$(echo "$BUNDLE" | jq -c .)"',
    "signatureType": "keyless"
  }'
```

The control-plane will:
1. Pull the artifact bytes and verify the SHA-256.
2. Parse and chain-validate the Fulcio certificate.
3. Verify the ECDSA signature.
4. Extract the builder OIDC subject (e.g. `https://github.com/acme/firmware/.github/workflows/release.yml@refs/heads/main`) and issuer.
5. Store the artifact with `verification_status = verified`, `signature_type = keyless`, and the builder identity recorded.

### 3.2 Registering a pre-pulled artifact with a keyless signature

```json
POST /api/v1/artifacts
{
  "name": "firmware",
  "version": "2.0.0",
  "type": "firmware",
  "objectKey": "artifacts/firmware-v2.tar.gz",
  "sha256": "abc123...",
  "sizeBytes": 4194304,
  "signature": "{\"base64Signature\":\"...\",\"cert\":\"-----BEGIN CERTIFICATE-----\\n...\",\"bundle\":{...}}",
  "signatureType": "keyless"
}
```

---

## 4) Attestations

Attestations are in-toto statements stored alongside an artifact. They are separate from the artifact signature and support richer supply-chain metadata (SLSA provenance, SBOMs, test results, custom predicates).

### 4.1 Upload an attestation

```bash
curl -X POST https://hardwareops.internal/api/v1/artifacts/$ARTIFACT_ID/attestations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "predicateType": "https://slsa.dev/provenance/v1",
    "payload": {
      "buildDefinition": {
        "buildType": "https://github.com/actions/runner/github-hosted",
        "externalParameters": {
          "ref": "refs/heads/main",
          "workflow": ".github/workflows/release.yml"
        }
      },
      "runDetails": {
        "builder": {"id": "https://github.com/acme/firmware/.github/workflows/release.yml@refs/heads/main"},
        "metadata": {"invocationId": "https://github.com/acme/firmware/actions/runs/12345"}
      }
    },
    "signatureType": "keyless",
    "keylessBundle": "{\"base64Signature\":\"...\",\"cert\":\"-----BEGIN CERTIFICATE-----\\n...\"}"
  }'
```

When `signatureType = "keyless"`, the control-plane verifies the bundle and populates `builderId` and `builderIssuer` from the Fulcio certificate automatically.

When `signatureType` is omitted or empty, the attestation is stored as-is for audit purposes without cryptographic verification.

**Response (201 Created):**
```json
{
  "attestationId": "uuid",
  "artifactId": "uuid",
  "predicateType": "https://slsa.dev/provenance/v1",
  "payload": {...},
  "signatureType": "keyless",
  "builderId": "https://github.com/acme/firmware/.github/workflows/release.yml@refs/heads/main",
  "builderIssuer": "https://token.actions.githubusercontent.com",
  "verifiedAt": "2026-03-17T12:00:00Z",
  "createdAt": "2026-03-17T12:00:00Z"
}
```

### 4.2 List attestations for an artifact

```bash
curl https://hardwareops.internal/api/v1/artifacts/$ARTIFACT_ID/attestations \
  -H "Authorization: Bearer $TOKEN"
```

Returns an array of attestation objects with the same shape as above.

---

## 5) Provenance policy

Provenance policy controls which attestations must be present before an artifact can be assigned to devices. It is enforced at desired-state set time — operators will get a policy error if they try to assign an artifact that doesn't satisfy the policy.

### 5.1 Global provenance policy

Set alongside the global signature policy:

```bash
curl -X PUT https://hardwareops.internal/api/v1/artifact-trust/policy \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "verificationMode": "require_verified",
    "provenance": {
      "requireProvenance": true,
      "requiredPredicateType": "https://slsa.dev/provenance/v1",
      "requiredBuilderId": "https://github.com/acme/firmware/.github/workflows/release.yml@*",
      "requiredBuilderIssuer": "https://token.actions.githubusercontent.com"
    }
  }'
```

Field reference:

| Field | Type | Description |
|---|---|---|
| `requireProvenance` | bool | When `true`, at least one attestation must match all other constraints. |
| `requiredPredicateType` | string | If set, the matching attestation must have this exact predicate type URI. |
| `requiredBuilderId` | string | Builder OIDC subject. Supports trailing `*` wildcard (prefix match). |
| `requiredBuilderIssuer` | string | Builder OIDC issuer. Must be an exact match. |

### 5.2 Per-desired-state policy override

Individual desired-state entries can tighten (but not loosen) the global provenance policy:

```json
PUT /api/v1/desired-state/groups/production
{
  "components": {
    "firmware": {
      "artifactId": "uuid",
      "policy": {
        "requireProvenance": true,
        "requiredBuilderId": "https://github.com/acme/firmware/.github/workflows/release.yml@refs/tags/*"
      }
    }
  }
}
```

### 5.3 Wildcard builder ID matching

`requiredBuilderId` supports a trailing `*` wildcard:

| Pattern | Matches |
|---|---|
| `https://github.com/acme/firmware/.github/workflows/release.yml@refs/heads/main` | Exact main branch only |
| `https://github.com/acme/firmware/.github/workflows/release.yml@refs/tags/*` | Any tag ref |
| `https://github.com/acme/firmware/*` | Any workflow in the repo |

---

## 6) Viewing the trust policy

```bash
curl https://hardwareops.internal/api/v1/artifact-trust/policy \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "verificationMode": "require_verified",
  "allowedSignatureTypes": ["keyless"],
  "provenance": {
    "requireProvenance": true,
    "requiredPredicateType": "https://slsa.dev/provenance/v1",
    "requiredBuilderId": "https://github.com/acme/firmware/.github/workflows/release.yml@*",
    "requiredBuilderIssuer": "https://token.actions.githubusercontent.com"
  },
  "updatedAt": "2026-03-17T12:00:00Z"
}
```

---

## 7) CI integration example (GitHub Actions)

```yaml
# .github/workflows/release.yml
jobs:
  build-and-publish:
    permissions:
      id-token: write   # required for keyless cosign
      contents: read
    steps:
      - uses: actions/checkout@v4
      - name: Build firmware
        run: make firmware

      - name: Install cosign
        uses: sigstore/cosign-installer@v3

      - name: Sign artifact
        run: |
          cosign sign-blob --bundle firmware.bundle firmware.tar.gz

      - name: Exchange workload identity token
        run: ./scripts/ci-exchange-workload-identity.sh

      - name: Upload to HardwareOps
        run: |
          SHA256=$(sha256sum firmware.tar.gz | awk '{print $1}')
          BUNDLE=$(cat firmware.bundle | jq -c .)
          curl -X POST $HARDWAREOPS_URL/api/v1/artifacts/upload \
            -H "Authorization: Bearer $HWOPS_TOKEN" \
            -F "name=firmware" \
            -F "version=${{ github.ref_name }}" \
            -F "type=firmware" \
            -F "sha256=$SHA256" \
            -F "signature=$BUNDLE" \
            -F "signatureType=keyless" \
            -F "file=@firmware.tar.gz"
```

---

## 8) Verification checklist

After configuring provenance:

1. Upload an artifact with a valid keyless bundle → verify `verificationStatus = verified` in the artifact detail.
2. Upload an attestation → verify `verifiedAt` is set and `builderId` is populated.
3. Try to assign the artifact to a group without the required attestation → verify a `400` error with a provenance policy message.
4. Add the attestation and retry the assignment → verify it succeeds.
5. Check audit events for `artifact_trust.policy.update`.

---

## Related docs

- `docs/development/phase-c-internals.md` — implementation details (Fulcio OIDs, Rekor verification, policy evaluation)
- `docs/development/artifact-ingest.md` — artifact ingest methods overview
- `docs/icd.md` — API contract including attestation endpoints
- `docs/deployment-hardening.md` — hardened profile policy floors
