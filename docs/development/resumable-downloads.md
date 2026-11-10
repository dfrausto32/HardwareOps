# Resumable Downloads for Intermittent / Contested Networks

Status: planned (roadmap tracks T1–T4). Reviewed 2026-11-10.

## Problem

Devices fetch artifacts over links that drop, stall, or are deliberately degraded. Today any interruption restarts the download at byte 0, and long downloads cannot finish at all on slow links.

## Current behavior (findings)

| Area | Location | Issue |
|---|---|---|
| Truncate on every attempt | `agent/internal/artifacts/apply.go` `downloadAndVerify` | `O_TRUNC` on `<id>.tar.gz.tmp`; on error the partial is deleted. |
| No Range support | `client.DownloadArtifact`, `HTTPDownloader` | Plain `GET`, only `200` accepted; `206` would be rejected as an error. |
| Whole-request timeout | `agent/internal/client/client.go` `NewWithTLS` | `http.Client.Timeout = 30s` includes body read, so any download >30s fails deterministically. |
| No in-attempt retry | `agent/cmd/agent/main.go` apply path | Failure is reported via `failReport`; retry only happens on a later check-in, from zero. |
| Presign lifetime | control-plane `PresignedGetObject` (`objectstore/minio.go`) | URL expires; a resumed transfer after expiry needs a fresh presign. |
| Server side | MinIO | Presigned GET already supports `Range`/`ETag`/`If-Range`; no control-plane change needed for the presigned path. |

## Design

### Partial-file state
`downloads/<artifactID>.<sha256-prefix>.part` + `.meta` sidecar (JSON): `{artifactSha256, size, etag, committedBytes, updatedAt}`. Keyed by SHA-256 so a replaced artifact never reuses a stale partial.

### Resume protocol (agent)
1. If a matching `.part` exists, truncate to `committedBytes`, hash the prefix into a fresh SHA-256 (or restore saved hash state), and request `Range: bytes=<committed>-` with `If-Range: <etag>`.
2. `206` → append. `200` → server ignored range/validator changed: truncate and restart. `416` → partial already complete or invalid: verify hash, else restart. `401/403` → re-presign (T2) and continue at the same offset.
3. Checkpoint (fsync file + sidecar) every N MiB / T seconds.
4. On EOF: compare full SHA-256 to `meta.SHA256`; mismatch deletes the partial (fail closed). Signature verification runs unchanged afterwards. Nothing is applied from an unverified partial.

### Timeouts and retry
Download client uses dial / TLS-handshake / response-header / idle-read timeouts (e.g. abort if no bytes for 60s) and no overall deadline. In-attempt retry: exponential backoff with full jitter, capped, bounded by a per-apply budget; after the budget the apply stays "in progress" (T3) rather than failed.

### Interface change
Extend F1 `Transport`/`Downloader` with an offset-aware method, e.g. `DownloadArtifactRange(url string, offset int64, validator string) (*RangeResponse, error)` returning status, `Content-Range`, ETag, and body. Keep `DownloadArtifact` for non-resumable transports; BLE/serial (F4/F6) implement chunked resume natively behind the same seam.

## Phasing

| Phase | Deliverable | Exit test |
|---|---|---|
| T1 | `.part` + sidecar, Range/If-Range, retry+jitter, timeout rework | httptest server dropping mid-body N times → identical verified file; >30s transfer succeeds; changed ETag → restart |
| T2 | Re-presign on expiry, fsync checkpoints, partial GC, per-component lock | TTL-expiry and `kill -9` scenarios resume |
| T3 | `downloading` progress status, no spurious failures, transfer windows / bandwidth cap | flapping-link scenario shows advancing progress, no failed apply-result |
| T4 | Per-chunk hashes, gateway/peer cache (optional) | decided after field data |

## Test plan
- Unit: table tests for 200/206/416/401, validator mismatch, prefix re-hash, corrupted partial, sidecar/file disagreement.
- Fault-injection server (Go `httptest`) that cuts the body at random offsets.
- Q4 integration: toxiproxy/`tc netem` between agent and MinIO — reset-peer, bandwidth throttle, full partition for longer than presign TTL.
- Assertion that bytes transferred over the whole run ≈ artifact size (+ small overhead), proving resume rather than restart.

## Open questions
1. Persist hash state vs. re-hash prefix on resume (re-hash is simpler; costs CPU on large artifacts on small devices).
2. Should partials survive artifact deprecation/prune, and what disk cap applies per device?
3. On-prem Caddy proxy path: confirm Range/ETag pass through for non-presigned downloads.
4. New device status values (`downloading`) require ICD (`docs/reference/icd.md`) and UI updates.
