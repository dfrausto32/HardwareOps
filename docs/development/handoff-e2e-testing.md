# Hand-off: E2E pipeline & comprehensive testing initiative

_Last updated: 2026-06-04 — branch `claude/main-action-failure-zA2O6` (PR #28), head `a9fc7be`._

This is a continuation brief for the next CLI session. It captures **where the
E2E pipeline stands, what changed and why, the one open decision, and the
plan to grow a real-world-representative test suite** (see also the new
"Quality Engineering" section in `docs/development/roadmap.md`).

---

## TL;DR

- The **"E2E Integration Tests"** GitHub Actions workflow (`.github/workflows/e2e.yml`
  + `scripts/e2e-suite.sh`) had **never passed** — early failures were masked by a
  flaky unit test. It is now **green**: `5 passed, 0 failed, 1 skipped`.
- One **real product bug** was found and fixed: global-plane sync workers were
  bound to the HTTP request context (a runtime-registered regional plane never
  synced until restart). Kept + covered by a regression test.
- Auth code that had been added **only to make the federation scenario pass**
  (global-plane bootstrap-admin + password-login endpoint) was **reverted** —
  it was unhardened and the auth model should be designed deliberately.
- The federation scenario (`global-plane-sync`) is **skipped by default**,
  pending a decision on **how operators authenticate to the global-plane**.

## Current state (what's on the branch)

**Product changes (keep — stand on their own):**
- `fix(global-plane): bind plane sync workers to the manager context`
  (`control-plane/internal/globalplane/sync/manager.go`) — real bug fix.
- `test(global-plane): lock in plane-worker manager-context fix`
  (`control-plane/internal/globalplane/sync/manager_test.go`) — regression test;
  verified it fails without the fix.
- The earlier `fix(auth): de-flake login-backoff unit test` (injectable
  `loginClock` in `control-plane/internal/auth/auth.go`) — benign, no behavior change.

**Test-only changes (keep):**
- `scripts/e2e-suite.sh` now runs the control-plane over **TLS + optional mTLS**
  (dev CA + server cert generated in-suite), with `AUTH_MODE=local`,
  `AUTH_JWT_SECRET`, `WEBHOOK_ENCRYPTION_KEY`.
- `scripts/test-multi-agent.sh` (new) — enrolls N distinct devices via the proven
  manual-CSR → `/devices/enroll` path, one enrollment token per device, checks
  each in over mTLS.
- `scripts/artifact-e2e.sh` — heredoc fix + operator-JWT auth.

**Reverted (do NOT silently re-add):**
- Global-plane `EnsureBootstrapAdmin` (was in `store_postgres_auth.go` + `cmd/global-plane/main.go`).
- Global-plane `/api/v1/auth/login` handler + route (was `internal/globalplane/httpapi/handlers/auth.go` + `router.go`).
- These live in git history at commits `8ba11c0` and `a56c8a8` if a **hardened**
  version is wanted later (see open decision).

**Deferred:**
- Scenario 6 `global-plane-sync` is guarded by `E2E_RUN_GLOBAL_SYNC` (skipped by
  default). Its federation service-token scopes are already corrected to
  `device.read,artifact.read,federation.push` (the sync **pulls** `/devices`,
  `/artifacts`, `/health/summary`; `federation.push` alone returns 403). It is
  ready to re-enable once the auth decision below is made.

## THE open decision (blocks re-enabling scenario 6)

**How should an operator (and the E2E test) authenticate to the global-plane?**
The global-plane validates JWTs/service-tokens but has **no login endpoint** and
enforces a distinct issuer (`parcel-global`), so a regional token is rejected.
Options:
1. **Hardened local login** — re-add a login endpoint that mirrors the regional
   one *with* its protections (login backoff/rate-limit, audit logging, TOTP/MFA,
   email normalization). `docs/reference/global-plane.md` says the global-plane uses "the
   same JWT/service-token mechanism as regional planes," so this is design-aligned.
2. **Seeded service token** — at global-plane startup, seed a `federation.manage`
   service token (env-gated, like the reverted bootstrap-admin) and have operators/
   tests use it. Smaller auth surface; diverges slightly from "local login" docs.
3. **SSO-only** — rely on OIDC for operator access; tests would mock the IdP.

Once chosen: implement it, set `E2E_RUN_GLOBAL_SYNC=1` for scenario 6, and confirm
federation E2E goes green (the global-plane log should show
`plane e2e-regional: synced N devices`).

## How to run things (local)

```bash
# Go tests (must stay green)
cd control-plane && go test ./...
cd agent && go test ./...

# The specific sync regression test
go test ./internal/globalplane/sync/ -run TestRefreshUsesManagerContext -v

# Full E2E suite locally (needs Docker for Postgres + MinIO)
./scripts/e2e-suite.sh                       # 5 pass, global-plane-sync skipped
E2E_RUN_GLOBAL_SYNC=1 ./scripts/e2e-suite.sh # also runs scenario 6 (will fail until auth decided)
```

CI: `.github/workflows/e2e.yml` runs two jobs on push/PR to `main` — `Unit tests`
and `Full stack E2E`. Logs upload as an artifact on failure.

## Immediate next steps

1. **Merge PR #28** (pipeline is green; it is the regression baseline).
2. **Decide global-plane operator auth** (above) → re-enable scenario 6.
3. **Start the testing-suite initiative** — see the new
   **"Quality Engineering — Comprehensive Test & Simulation Pipeline"** section in
   `docs/development/roadmap.md` (Q1–Q5): close the Go-test coverage backlog
   (OIDC/LDAP/TOTP, `certs`, `license`, reenroll, untested global-plane handlers),
   add the extended/nightly E2E suite, build the **embedded-device simulation repo**
   (`parcel-device-sim`, e.g. RP2040/Pi Pico via Renode/QEMU), and add
   **unreliable-network simulation** (offline/bandwidth-constrained — the product's
   core value prop).

## Pointers
- Roadmap: `docs/development/roadmap.md` → "Quality Engineering" section.
- Global-plane operator runbook: `docs/reference/global-plane.md`.
- E2E suite: `scripts/e2e-suite.sh`; multi-agent: `scripts/test-multi-agent.sh`.
- Test patterns: `CLAUDE.md` → "Test Patterns".
