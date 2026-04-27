# Parcel — Scalability & Security Analysis
**Date:** 2026-03-15
**Scope:** Full codebase audit across control-plane, agent, and UI
**Ceiling target:** 1,000–10,000 concurrent devices

---

## Context

This document covers two concerns:
1. **Scalability** — bottlenecks and design patterns that will degrade or fail at thousands of devices
2. **Security** — vulnerabilities and best-practice gaps that could allow malicious code onto customer devices or unauthorized access to the platform

Parts 1 and 2 of the duplicate-artifact work (tie-break resolver, ingest policy) and Part 3 (UI) are complete. This analysis is independent of that work.

---

## Part A — Scalability Analysis

### A1. CRITICAL — Unbounded `ListDesiredStateGroups` / `ListDesiredStateDevices`

**Files:**
- `control-plane/internal/store/postgres/postgres.go` lines 1679–1758
- `control-plane/internal/httpapi/handlers/desired_state.go` lines 326–390
- `control-plane/internal/releaseautoupdate/manager.go` lines 256–360

Both store methods execute a `SELECT … ORDER BY updated_at DESC` with no `LIMIT`. They are called in three places: the `ListDesiredState` HTTP handler (which returns the full result set to the client), and the release auto-update manager which calls both every time it runs (default: every 60 seconds).

**Impact at 10,000 devices:**
- 10,000+ rows deserialized into memory per operation, each containing a JSONB components field
- ~10–20 MB of heap allocated per run in the manager alone
- HTTP response can be 10–100 MB of JSON with no pagination
- The UI browser tab will be unable to render 10,000 rows

**Fix:** Add `LIMIT`/`OFFSET` parameters to both store methods and their callers. The `ListArtifacts` function already has this pattern (`const pageSize = 500` loop in manager.go line 516) — apply the same approach.

---

### A2. CRITICAL — N+1 Query in `existingGroupComponents`

**File:** `control-plane/internal/httpapi/handlers/desired_state.go` lines 522–536

```go
func existingGroupComponents(st store.Store, groupID string) map[string]DesiredComponentResponse {
    groups, err := st.ListDesiredStateGroups()  // loads ALL groups
    for _, group := range groups {
        if group.GroupID == groupID { ... }      // finds one
    }
}
```

Called from `PutDesiredStateGroup`. Every write to a single group's desired state loads the entire groups table to find that group by ID.

**Impact:** At 1,000 groups, every `PUT /desired-state/groups/{id}` does a full table scan. Should be a single `SELECT … WHERE group_id = $1`.

**Fix:** Add `GetDesiredStateGroup(groupID string) (DesiredStateGroup, bool, error)` to the store interface and use it here.

---

### A3. CRITICAL — Auto-Update Manager Full-Load Every 60 Seconds

**File:** `control-plane/internal/releaseautoupdate/manager.go` lines 222–360

Every run loads:
- All artifacts via paginated loop (~5–10 MB at 10K artifacts)
- All desired-state groups via unbounded query
- All desired-state devices via unbounded query
- Builds full in-memory index, sorts all version buckets

**Memory cost per run at 10K scale: ~30–50 MB.** GC pressure: ~500 MB/hour.

There is also no queue for triggered runs — if the current run takes >60 seconds, the next scheduled run is silently dropped (`return RunSummary{}, errors.New("already running")`), which can cause devices to miss updates under load.

**Fix options:**
- Process groups/devices in pages rather than loading all at once
- Track a `last_modified` watermark so the manager only processes groups/devices changed since last run
- Add a small buffer queue (size 1) so a triggered run isn't lost if the manager is busy

---

### A4. MAJOR — Missing Database Indexes

**Files:** `control-plane/migrations/` (all 25 files reviewed)

Queries that will full-scan at scale due to missing indexes:

| Missing Index | Query That Needs It | Location |
|---|---|---|
| `devices(status)` | `ListDevices` filtering by status | postgres.go |
| `desired_state_group(updated_at)` | `ORDER BY updated_at DESC` | postgres.go line 1679 |
| `desired_state_device(updated_at)` | `ORDER BY updated_at DESC` | postgres.go line 1724 |
| `artifacts(status)` | lifecycle operations filtering by status | postgres.go |
| `artifacts(name, type)` composite | name+type queries in auto-update | manager.go |

**Fix:** New migration adding these indexes. All are non-blocking (`CREATE INDEX CONCURRENTLY`) on live instances.

---

### A5. MAJOR — WebSocket Event Hub Can Block All Subscribers

**File:** `control-plane/internal/events/events.go`

```go
func (h *Hub) Publish(event Event) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for ch := range h.subs {
        select {
        case ch <- event:
        default:   // silently drops if buffer (64) full
        }
    }
}
```

Two problems:
1. `Publish` holds the read lock while iterating and sending to all subscriber channels. A slow channel write (e.g., OS scheduler delay) stalls event delivery for all other subscribers during that lock hold.
2. The per-subscriber buffer is only 64 events. A briefly-slow WebSocket client silently drops messages with no error feedback.

**Impact at 1,000 concurrent WebSocket subscribers:** a single slow client can cause 1–5 second latency spikes across all others.

**Fix:** Send to each channel from a goroutine (fire-and-forget with per-subscriber timeout), or fan-out to per-subscriber dispatch goroutines so slow clients are isolated. Drop slow subscribers rather than blocking fast ones.

---

### A6. MAJOR — Per-IP Rate Limiting, Not Per-Device

**File:** `control-plane/internal/httpapi/ratelimit.go` lines 72–90

Rate limiting is keyed by client IP. Devices behind a NAT or corporate proxy share one bucket. A single runaway device can exhaust the rate limit for all 10,000 devices on the same network.

**Fix:** For device endpoints (`/devices/checkin`, `/devices/apply-result`), key rate limiting by device ID extracted from the mTLS certificate after the TLS handshake, falling back to IP for unauthenticated paths.

---

### A7. MAJOR — Default pgxpool Not Tuned

**File:** `control-plane/cmd/control-plane/main.go` line 61

```go
pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
```

Default `pgxpool` sets `MaxConns = 4 × numCPU`. On a 4-core instance that is 16 connections. At 10,000 devices polling every 60 seconds (~167 req/sec), checkins alone will queue waiting for connections, causing timeouts.

**Fix:** Expose `DB_MAX_CONNS`, `DB_MIN_CONNS`, `DB_MAX_CONN_IDLE_TIME` as env vars and apply them to `pgxpool.Config`. A reasonable baseline for 10K devices: `MaxConns=50`, `MinConns=5`.

---

### A8. MEDIUM — Rate Limiter Entry Map Unbounded Growth

**File:** `control-plane/internal/httpapi/ratelimit.go` lines 92–101

The `prune()` method only runs when `len(entries) >= 1024`, and only removes expired entries. With 10,000 unique source IPs, the map grows to 10,000 entries and is pruned infrequently.

**Fix:** Always prune on every `allow()` call (amortized cost is low), or replace with a TTL-based LRU cache.

---

### A9. MEDIUM — Agent Sends Full State on Every Poll (No Delta)

**File:** `agent/internal/client/client.go` lines 292–320

Each checkin POSTs the complete current state (all components, labels, capabilities) and receives the full desired state. There is no ETag, `If-None-Match`, or incremental diff mechanism.

**Impact at 10,000 devices / 60-second poll:** sustained ~167 req/sec, each with ~5 KB payload = ~835 KB/sec upstream + equivalent downstream. At shorter intervals this compounds quickly.

**Fix (medium-term):** Add ETag/hash of desired state to checkin response. Agent skips sending full desired state re-fetch if hash unchanged. This is a protocol change requiring coordinated agent + server work.

---

### A10. LOW — WebSocket Write Timeout Per-Message (Slow Client Holds Goroutine)

**File:** `control-plane/internal/httpapi/handlers/events.go` lines 92–94

Each WebSocket message write has a 5-second context timeout. A slow client holds the goroutine for up to 5 seconds per message. At 1,000 devices, this creates 1,000 goroutines potentially blocked for 5 seconds each.

**Fix:** Implement per-connection inactivity deadline rather than per-write timeout; evict unresponsive connections after 2–3 missed writes.

---

### Scalability Summary

| Priority | Issue | Location | Impact at 10K |
|---|---|---|---|
| CRITICAL | Unbounded ListDesiredState queries | postgres.go 1679, 1724 | Full table in memory per op |
| CRITICAL | N+1 query on group desired-state write | desired_state.go 522 | Full table scan per write |
| CRITICAL | Auto-update loads all devices every 60s | manager.go 222–360 | ~50MB/run, GC pressure |
| MAJOR | Missing DB indexes (5 columns) | migrations/ | Full table scans at scale |
| MAJOR | WebSocket hub blocks on slow subscriber | events.go | Cascading latency 1K+ clients |
| MAJOR | Rate limiting is per-IP not per-device | ratelimit.go 72 | NAT collapse of 10K devices |
| MAJOR | pgxpool not tuned | main.go 61 | Queue exhaustion at high concurrency |
| MEDIUM | Rate limiter map unbounded growth | ratelimit.go 92 | Memory leak under load |
| MEDIUM | No delta sync in agent checkin | client/client.go 292 | O(n) bandwidth at large scale |
| LOW | WebSocket slow-client goroutine hold | events.go 92 | Goroutine accumulation |

---

## Part B — Control-Plane Security Audit

### Strengths (No Action Required)

These areas are well-implemented and should not be changed:

- **JWT:** HS256 algorithm enforced, expiry validated, user-disabled recheck on every request (`auth/auth.go`)
- **Service tokens:** Stored as SHA256 hashes, never plaintext (`auth/service_token.go`)
- **OIDC:** `crypto/rand` state parameter, HttpOnly+SameSite=Lax cookie, `go-oidc` library validation
- **Passwords:** bcrypt with DefaultCost (12), constant-time comparison
- **mTLS:** Per-request fingerprint validation against DB; device deletion immediately blocks access
- **SQL injection:** All queries fully parameterized, no string interpolation observed in postgres.go
- **CSR validation:** Signature check, SAN limits (max 10 DNS SANs), no URI/email SANs, 16 KB size cap, no wildcards
- **SSRF:** Host allowlist + private/loopback IP block in `artifactingest/http_pull_adapter.go`
- **CORS:** Exact-match origin whitelist, no wildcard
- **RBAC:** Three-role hierarchy (viewer/operator/admin), service token scope enforcement
- **Artifact signing:** Three-tier policy with hardened mode enforcing `require_verified`
- **Audit logging:** All sensitive operations logged with actor, action, resource, metadata
- **Enrollment rate limiting:** Per-IP + max-active-per-profile guards

---

### B1. MEDIUM — OIDC Token Passed as URL Query Parameter

**File:** `control-plane/internal/httpapi/handlers/auth_oidc.go` line 72

```go
redirectURL := "/?oidc_token=" + token
```

Tokens in URLs are recorded in browser history, proxy access logs, and server access logs. A leaked log file exposes valid sessions.

**Fix:** Use the URL fragment (`/#oidc_token=…`) so the token is not sent to the server and not logged by proxies, or use a short-lived server-side code that the SPA exchanges in a POST.

---

### B2. MEDIUM — Certificate Revocation Gap (No CRL/OCSP)

**File:** `control-plane/internal/certs/manager.go`

Device certificates are issued with a 1-year validity (enrollment.go line 221). Revocation is implemented by deleting the device record, which blocks new checkins. However, the TLS certificate itself remains cryptographically valid until expiry. A stolen cert can still complete a TLS handshake against a restarted or different control-plane instance that doesn't have the device in its DB.

**Fix (pragmatic):** Reduce default cert validity to 90 days. Implement automatic renewal (agent requests new cert when < 30 days remain). This limits the stolen-cert window without requiring full CRL infrastructure.

**Fix (complete):** Implement a certificate serial blocklist table checked at mTLS middleware; mark serials as revoked on device deletion.

---

### B3. MEDIUM — Internal Error Details Exposed to Clients

**File:** `control-plane/internal/httpapi/handlers/artifacts.go` line 741

```go
http.Error(w, err.Error(), http.StatusBadRequest)
```

Storage backend errors (S3 paths, MinIO bucket names, internal config values) can leak in error responses during artifact pull operations. Similar patterns exist in other handlers.

**Fix:** Standardize on a `writeError(w, status, "public message")` wrapper that logs full `err.Error()` server-side and returns only a sanitized message to the client. The pattern already exists for some paths — apply it consistently.

---

### B4. MEDIUM — Missing HTTP Security Headers

**File:** `control-plane/internal/httpapi/middleware.go`

No `Content-Security-Policy`, `Strict-Transport-Security`, or `X-Frame-Options` headers are set in the middleware chain. These do not affect the API but matter for the web console served on the same origin.

**Fix:** Add to the existing middleware:
```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Content-Security-Policy: default-src 'self'; script-src 'self'; ...
```

---

### Control-Plane Security Summary

| Severity | Finding | Location |
|---|---|---|
| MEDIUM | OIDC token in URL query param | auth_oidc.go 72 |
| MEDIUM | No CRL/OCSP; 1-year cert window after revocation | certs/manager.go |
| MEDIUM | Internal error details leaked to clients | artifacts.go 741 |
| MEDIUM | Missing CSP / HSTS / X-Frame-Options headers | middleware.go |
| SECURE | JWT algorithm enforcement | auth/auth.go |
| SECURE | Service token hashing | auth/service_token.go |
| SECURE | OIDC CSRF state param | auth/oidc.go |
| SECURE | mTLS per-request fingerprint check | handlers/mtls.go |
| SECURE | SQL fully parameterized | store/postgres/ |
| SECURE | SSRF allowlist | artifactingest/ |
| SECURE | Artifact signing policy hardened mode | artifacttrust/ |

---

## Part C — Agent-Side Security Audit

### C1. CRITICAL — Environment Variable Injection in Preapply Script Execution

**File:** `agent/internal/planexec/exec.go` line 273

```go
if len(ctx.Env) > 0 {
    cmd.Env = append(os.Environ(), ctx.Env...)
}
```

A crafted `plan.yaml` can inject `LD_PRELOAD`, `LD_LIBRARY_PATH`, `PYTHONPATH`, or `PATH` overrides into the subprocess environment. On interpreted scripts (shell, Python) this enables arbitrary code execution with the agent's privileges (typically root under systemd).

**Fix:** Start with a minimal, allowlisted environment (`PATH=/usr/bin:/bin`, `HOME`, `USER`). Do not inherit the parent process environment for preapply scripts.

---

### C2. CRITICAL — Arbitrary Command Execution via plan.yaml

**File:** `agent/internal/planexec/exec.go` lines 238–287

The `command` parameter from `plan.yaml` is resolved relative to the working directory (the extracted artifact). There is no validation that the resolved path:
- Is a regular file (not a symlink)
- Is within the artifact directory
- Has not been substituted via a path traversal

A symlink in the archive (`scripts/run.sh -> /bin/bash`) or a command of `../../bin/sh` could cause execution of arbitrary binaries.

**Fix:** Require commands to be relative paths with no `..` components; verify the resolved absolute path is inside `ctx.WorkingDir`; reject symlinks in preapply command positions.

---

### C3. HIGH — Incomplete Path Traversal Check in Tar Extraction

**File:** `agent/internal/artifacts/apply.go` lines 473–476

```go
if strings.Contains(hdr.Name, "..") || strings.HasPrefix(hdr.Name, "/") {
    return fmt.Errorf("invalid path in archive: %s", hdr.Name)
}
path := filepath.Join(dest, hdr.Name)
```

Two gaps:
1. Symlinks are created without validating their targets. An archive entry `data/conf -> ../../../etc/cron.d/evil` passes the path check and is extracted as a valid symlink.
2. `filepath.Join` cleans the path but the check runs on the raw header name before joining, so a name like `subdir/./../../etc/passwd` could pass the `..` string check depending on encoding.

**Fix:** After `filepath.Join`, verify `strings.HasPrefix(absPath, dest+"/")`. Reject symlinks whose targets resolve outside `dest`. Use `filepath.EvalSymlinks` post-extraction to detect escaped paths.

---

### C4. HIGH — Preapply Scripts Execute Before File Integrity Is Verified

**File:** `agent/internal/artifacts/apply.go` lines 108–158

Current order:
1. Extract tar.gz
2. Parse `manifest.json` (checks version/type only)
3. Load `plan.yaml`
4. **Run preapply steps** ← executes here
5. Verify individual file SHA256 checksums

Preapply scripts can read, copy, or act on artifact files whose checksums have not yet been validated. A MITM or a compromised MinIO can deliver a modified file that preapply sees before the SHA check catches it.

**Fix:** Move all file SHA256 verification to immediately after extraction (before any plan loading or script execution). Fail hard if any checksum mismatches.

---

### C5. HIGH — State File Has No Integrity Check

**File:** `agent/internal/state/state.go` lines 48–66

The state file is loaded with `json.Decode` and no HMAC or signature check. An attacker with write access to the agent's filesystem can:
- Downgrade `CurrentVersion` to trigger rollback to a vulnerable version
- Modify `SigningTrustKeys` to accept unsigned artifacts
- Falsify `LastApplyStatus` to suppress error reporting

**Fix:** Sign the state file with an HMAC keyed from a device-local secret (e.g., derived from the device private key). Verify on every load; treat a failed verification as a corrupt/tampered state and re-enroll or halt.

---

### C6. HIGH — Artifact Signature Verification Defaults to Warn, Not Fail

**File:** `agent/internal/artifacts/apply.go` lines 231–282

The default verification mode is `warn_unsigned`. If `opts.TrustKeys` and `opts.SigningPublicKeyPath` are both empty, **all artifacts are accepted** regardless of signature, with only a log warning. Verification only fails closed if explicitly configured to `require_verified`.

**Fix:** Default to `require_verified`. Require an explicit opt-in configuration (`ARTIFACT_SIGNING_MODE=allow_unsigned`) to accept unsigned artifacts.

---

### C7. HIGH — CA Certificate Not Pinned; Falls Back to System CA Store

**File:** `agent/cmd/agent/main.go` lines 259–294

If `CA_CERT_PATH` is not set, `tlsConfig.RootCAs` is `nil`, causing Go to use the system CA store. An attacker with access to the device OS can install a rogue CA certificate and MITM all control-plane traffic, delivering arbitrary artifacts.

**Fix:** Require `CA_CERT_PATH` to be set; fail to start if absent. Never fall back to system CAs for control-plane connections.

---

### C8. MEDIUM — Enrollment Token Is Not Single-Use on Client Side

**File:** `agent/cmd/agent/bootstrap.go` lines 107–149

The `ENROLLMENT_PROFILE_TOKEN` is sent as a plain value. If an attacker reads it from the environment (via `/proc` exposure, log output, or container inspect), they can enroll a rogue device using the same token. The server may allow multiple uses of the same token.

**Fix (server-side):** Invalidate enrollment profile tokens after first successful use, or enforce a device-count cap per token. **Fix (client-side):** Clear the token from environment after successful enrollment.

---

### C9. MEDIUM — Device Key Uses 2048-bit RSA

**File:** `agent/cmd/agent/bootstrap.go` lines 306–326

```go
key, err := rsa.GenerateKey(crand.Reader, 2048)
```

2048-bit RSA is below current NIST recommendations (3072-bit minimum through 2030). For long-lived device certificates (1 year validity), this is a concern.

**Fix:** Use Ed25519 (`ed25519.GenerateKey`) for new enrollments. Smaller key, faster operations, higher security margin.

---

### C10. MEDIUM — State File Created World-Readable

**File:** `agent/internal/state/state.go` line 70

```go
f, err := os.Create(path)   // inherits umask, typically 0o644
```

The state file contains device ID, component versions, and artifact IDs. It is readable by any local user.

**Fix:** Use `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)`.

---

### C11. MEDIUM — Forced Rollback Via Repeated Control-Plane Errors

**File:** `agent/cmd/agent/main.go` lines 481–489

If `artifacts.Apply()` fails (e.g., the control-plane delivers a deliberately corrupted artifact), the agent automatically rolls back to `oldVersion`. A compromised control-plane can therefore force a device to downgrade to any previously-installed version by repeatedly delivering failing artifacts.

**Fix:** Add a rollback attempt counter. After N consecutive rollbacks, enter a safe-halt state and require operator intervention rather than continuing to accept new versions.

---

### C12. MEDIUM — Partial Download Leaves Corrupt File on Disk

**File:** `agent/internal/artifacts/apply.go` lines 199–229

If a download is interrupted mid-transfer, `io.Copy` returns an error, but the partial file at `downloads/{artifactID}.tar.gz` is not deleted. A subsequent retry will find a partial file and may attempt to reuse it.

**Fix:** Write to a `.tmp` suffix; rename atomically only after SHA256 verification passes; delete the `.tmp` file on any error.

---

### Agent-Side Security Summary

| Severity | Finding | Location |
|---|---|---|
| CRITICAL | Env var injection in preapply exec | planexec/exec.go 273 |
| CRITICAL | Arbitrary command execution via plan.yaml | planexec/exec.go 238 |
| HIGH | Symlink + traversal in tar extraction | artifacts/apply.go 473 |
| HIGH | Preapply runs before file SHA verification | artifacts/apply.go 108–158 |
| HIGH | State file not integrity-checked | state/state.go 48 |
| HIGH | Signature verification defaults to warn | artifacts/apply.go 231 |
| HIGH | CA cert not pinned; system CA fallback | cmd/agent/main.go 259 |
| MEDIUM | Enrollment token reuse not prevented | bootstrap.go 107 |
| MEDIUM | 2048-bit RSA (below NIST 2030 guidance) | bootstrap.go 306 |
| MEDIUM | State file world-readable (0644) | state/state.go 70 |
| MEDIUM | Forced rollback via repeated errors | cmd/agent/main.go 481 |
| MEDIUM | Partial download not cleaned up | artifacts/apply.go 199 |

---

## Recommended Priority Order for Remediation

### Immediate (block production deployments to new customers)
1. **C1** — Env var injection → fix exec.go environment construction
2. **C2** — Arbitrary command via plan.yaml → validate command path containment
3. **C3** — Tar symlink traversal → post-join bounds check + symlink target validation
4. **C6** — Signature verification default → flip to fail-closed

### High Priority (address in next release)
5. **C4** — Move SHA verification before preapply
6. **C5** — HMAC-sign state file
7. **C7** — Require CA cert; remove system CA fallback
8. **A1/A2/A3** — Paginate ListDesiredState + fix N+1 query + page auto-update runs

### Medium Priority (next two sprints)
9. **A4** — Add missing DB indexes (one migration)
10. **A5** — Fix WebSocket hub isolation
11. **A6** — Per-device rate limiting on checkin endpoints
12. **A7** — Tune pgxpool
13. **B1** — OIDC token out of URL
14. **B2** — Shorten cert validity to 90 days + auto-renewal
15. **B3** — Sanitize error messages
16. **C9** — Migrate device key gen to Ed25519
17. **C10** — Fix state file permissions (one-liner)
18. **C12** — Atomic download with `.tmp` + delete on error

### Lower Priority (monitor and schedule)
19. **B4** — Add HTTP security headers
20. **C8** — Token single-use enforcement
21. **C11** — Rollback loop safeguard
22. **A8/A9/A10** — Rate limiter LRU, delta sync protocol, WS goroutine eviction

---

## Design Decisions (Confirmed)

1. **Agent execution context:** Some deployments run as root, some don't. Treat worst-case (root) as baseline. **C1 and C2 remain immediate blockers.**

2. **Cert validity:** Mixed — online deployments can use shorter validity, air-gapped need 1 year. **Fix:** Make validity configurable per enrollment profile (`cert_validity_days`, default 90 for standard profiles, allow up to 365 for air-gapped). Implement a certificate serial blocklist in the DB for revocation regardless of validity period (avoids requiring OCSP/CRL infrastructure).

3. **Enrollment token model:** Multi-use with a per-profile cap (`max_enrollments` field on the enrollment profile, default 1 for single-device tokens, configurable up to unlimited for batch). The server enforces the cap atomically (DB counter with check constraint). This covers C8.

4. **Delta sync:** In scope for near-term scalability work. ETag-based protocol: server returns a `desired_state_hash` in the checkin response; agent skips the full desired-state fetch if hash matches the last known hash. Changes the agent protocol but is backward-compatible (old agents without ETag support just always fetch).
