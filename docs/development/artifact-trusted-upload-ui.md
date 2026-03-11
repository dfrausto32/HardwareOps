# Artifact Trusted Upload UI

This document defines the recommended UI model for manual artifact uploads when operators need trusted/signed artifacts.

## Goal

Support manual uploads without ever moving a private signing key into the control-plane or browser session.

## Recommended Flow

1. Operator prepares the artifact bundle locally.
2. Operator signs the artifact digest locally using a detached signature workflow.
3. Operator opens the control-plane upload modal and provides:
   - artifact bundle
   - detached signature file
   - signing key ID
4. Control-plane computes the artifact digest and validates the upload policy.
5. Control-plane verifies the detached signature against a trusted public key registry.

## UI Contract

Artifact upload modal fields:
- `name`
- `version`
- `bundle`
- `detached signature` (optional unless policy requires signed uploads)
- `signature type` (`ed25519` | `cosign`)
- `signing key ID` (optional unless policy requires/pins a key)

Behavior:
- if no signature is provided, upload proceeds only when policy allows unsigned artifacts
- if signature is provided, the UI sends:
  - `signature`
  - `signatureType`
  - `signatureKeyId`
- artifact list shows verification state:
  - `verified`
  - `legacy`
  - `failed`
  - `untrusted`
  - `unsigned`

## Current Shipped Behavior

Current UI supports manual entry of:
- detached signature file
- signature type
- signing key ID

Current backend behavior:
- stores `signature`, `signatureType`, and `signatureKeyId`
- verifies detached signatures against trusted public keys during create/upload/pull/complete
- persists verification state (`verified`, `legacy`, `failed`, `untrusted`, `unsigned`)
- distributes the active trust bundle to agents during enroll/claim/check-in
- enforces verification mode during desired-state selection and apply

So today:
- `verified` means the control-plane validated the detached signature against an active trusted key
- `legacy` means the artifact predates trusted verification or was registered through a compatibility path
- `unsigned` / `failed` / `untrusted` reflect policy-relevant ingest outcomes

## Remaining Gaps

Still out of scope in this slice:
- Sigstore keyless verification (Fulcio/Rekor)
- provenance / attestation policy
- browser-side local signing helper
- richer trust policy editing UX for per-group or per-device override management

## Security Model

Preferred manual model:
- private signing key stays with operator tooling
- UI only receives artifact + detached signature + type + key ID
- control-plane verifies with trusted public keys and stores the verification outcome

Avoid:
- browser-stored private keys
- control-plane-held operator signing keys
- trusting a `signed` flag without verification

## Recommended Operator Tooling

Short term:
- local signing script + detached signature upload

Later:
- local helper backed by OS keychain / HSM / YubiKey / PKCS#11
- remote enterprise signer / KMS-backed signing flow
