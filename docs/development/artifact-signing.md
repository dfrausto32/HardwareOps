# Artifact signing options (current Ed25519 vs. future Cosign/Sigstore)

This doc explains the **current v1 signing flow** and what adopting **Cosign/Sigstore** later would mean, including benefits and a migration path that keeps backward compatibility.

## 1) Current v1 baseline (Ed25519, offline)
**What we do today**
- A CI job or packer computes `sha256(artifact.tar.gz)`.
- We sign that hash with **Ed25519** using an offline/private key.
- The signature and key ID are stored with artifact metadata.
- The agent (later) verifies the signature using the public key before apply.

**Why it works**
- Simple, fast, offline-friendly.
- Small key material, easy to distribute a public key.
- Minimal dependencies (no external services).

**What’s missing**
- No public transparency log.
- No keyless identity verification.
- Fewer “standard” supply‑chain features vs industry tooling.

## 2) What Cosign/Sigstore is
**Cosign** is a signing/verifying tool that supports artifacts in OCI registries and standalone archives.
**Sigstore** is an ecosystem that can provide:
- **Keyless signing** (OIDC identity → short‑lived certs)
- **Transparency logs** (immutable public records of signatures)
- **Provenance** (build metadata, SLSA attestation)

Sigstore typically involves these components:
- **Fulcio** (issues short‑lived signing certs)
- **Rekor** (transparency log)
- **Cosign** (sign/verify tool)

You can run Sigstore components on‑prem, or use the public Sigstore service if you have outbound access.

## 3) Benefits of Cosign/Sigstore (vs custom Ed25519)
- **Stronger auditability**: Rekor provides an immutable log of signatures.
- **Keyless signing**: no long‑lived private keys to store; identity‑based certs.
- **Supply‑chain compatibility**: aligns with common enterprise security policies.
- **Provenance support**: attach SBOMs and build attestations.

## 4) Tradeoffs / requirements
- **Complexity**: introduces new services (Fulcio/Rekor) or reliance on external services.
- **Connectivity**: public Sigstore requires outbound access.
- **Operational burden**: running a private Sigstore stack requires extra ops.

## 5) Migration path (no breaking changes)
We can support both formats by adding a `signatureType` and optional `cosignBundle` field in artifact metadata:

**Recommended metadata additions**
- `signatureType`: `ed25519` | `cosign`
- `signature`: base64 for Ed25519
- `signatureKeyId`: key identifier for Ed25519
- `cosignBundle`: JSON (optional), used for cosign verification

**Flow**
1) Keep **Ed25519** as default for v1.
2) Add support for `signatureType=cosign` in the upload path.
3) Agent chooses verifier based on `signatureType`.
4) UI displays “Signed (Ed25519)” vs “Signed (Cosign)” badges.

This lets you adopt Cosign later without invalidating existing artifacts.

## 6) Recommendation for now
- **Stay with Ed25519** for v1 (lowest friction, offline-friendly).
- **Design for Cosign** by extending metadata (as above).
- When ready, add optional Cosign signing in CI and verification on the agent.

## 7) When you should choose Cosign/Sigstore
Consider Cosign/Sigstore if you need:
- Audit‑grade trail of signatures.
- Keyless signing tied to enterprise identity.
- Stronger supply‑chain posture (SBOM + provenance).

If you’re mostly on‑prem or air‑gapped, Ed25519 is typically the pragmatic starting point.
