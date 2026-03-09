# Production-Like Docker Test Plan

This plan validates end-to-end behavior in the Docker lab from `prod-docker-lab.md`.

Use it as:
- release readiness checklist
- regression gate after major platform/security changes
- reproducible demo validation flow

---

## 1) Test Scope

### Core Features

- Auth + admin bootstrap
- Device enroll/check-in and status transitions
- Artifact upload/pull/apply flows
- Group desired-state operations
- Upgrade/backup/restore workflows
- Certificate rotation and re-enroll
- Metrics and event retention behavior

### Security and Hardening

- Auth brute-force/backoff controls
- Pull ingest host/size/timeout boundaries
- Proxy trust allowlist behavior
- Signature policy defaults and enforcement
- Maintenance privilege boundary (`maintenance-runner`)

---

## 2) Preconditions

- Lab stack is running (`docs/testing/prod-docker-lab.md`)
- You can login to `https://hwops.localhost`
- At least 1 demo agent connected

Optional for stronger coverage:
- 3+ demo agents connected
- signed test artifacts uploaded

---

## 3) P0 Release Gate (must pass)

### P0-1 Control plane boot + auth

1. Open UI and login with bootstrap admin.
2. Verify no repeated restart loop:
   - `docker compose ... ps`
   - `docker compose ... logs control-plane --tail=200`

Expected:
- login works
- control-plane is healthy and stable

### P0-2 First-contact approval onboarding

1. Build the packaged Linux agent bundle:
   - `AGENT_PLATFORMS=linux/<arch> CONTROL_PLANE_PLATFORMS=linux/<arch> BUILD_STACK=0 ./scripts/build-installers.sh`
   - Use `amd64` on x86_64 hosts and `arm64` on ARM hosts.
2. Run `./scripts/testing/prod-lab-first-contact.sh`.
3. Capture the request ID, device ID, and final success output.

Expected:
- packaged installer path is exercised, not `run-demo-agent.sh`
- agent stays alive in approval mode until the request is approved
- same installed agent writes `device.crt` plus `device-id`
- bootstrap state is cleared after issuance
- device reaches active mTLS check-in without a second install pass

### P0-3 Device connectivity

1. Run `./scripts/run-demo-agent.sh` with lab CA.
2. Confirm devices appear active in UI.

Expected:
- check-ins every interval
- no sustained TLS/check-in failures

### P0-4 Signed artifact ingest + apply

1. Upload signed artifact (or use `scripts/multi-app-artifacts.sh`).
2. Set desired state for a connected device/group.
3. Confirm apply success and current version update.

Expected:
- desired/current converge
- apply status is success with expected artifact ID

### P0-5 Pull ingest hardening

1. Execute pull ingest from allowed host.
2. Execute pull ingest from blocked host/internal IP target.

Expected:
- allowed source succeeds
- blocked target is rejected with policy error

### P0-6 Backup + restore smoke

1. Create backup from UI (Settings -> Backups).
2. Restore latest backup in lab.
3. Validate devices/artifacts/settings are back.

Expected:
- backup and restore complete
- system returns to healthy state

### P0-7 Certificate rotation smoke

1. Trigger CA rotate from Security page.
2. Trigger/observe agent re-enroll.
3. Verify device cert metadata fingerprint moved to active CA.

Expected:
- new active CA fingerprint visible
- devices continue check-ins post-rotation

### P0-8 Metrics + events visible

1. Check `/metrics` endpoint.
2. Open UI Metrics page and Logs page history.

Expected:
- key counters populated (devices, check-ins, DB pool, S3 bytes/objects)
- event history query works

---

## 4) Edge Cases (should pass before customer demo)

### E-1 Maintenance mode behavior

1. Enable maintenance mode.
2. Verify write operations are blocked where expected.
3. Disable maintenance mode and re-test writes.

Expected:
- behavior matches policy
- state changes resume after disable

### E-2 Upgrade preflight failures

1. Place invalid/missing upgrade bundle.
2. Run preflight/apply.

Expected:
- clear preflight failure
- no partial destructive state

### E-3 Artifact lifecycle safety

1. Deprecate artifact that is not in use.
2. Attempt delete on in-use artifact.

Expected:
- safe deprecate/restore works
- delete is blocked while referenced

### E-4 Group batch operations

1. Multi-select groups and apply desired-state edit.
2. Run rollback via CSV flow.

Expected:
- batch changes apply correctly
- rollback returns prior values

---

## 5) Security Validation Cases

### S-1 Login brute-force backoff

1. Repeatedly login with wrong password for same user/IP.
2. Inspect response codes and retry windows.

Expected:
- rate limit/backoff activates
- retries become progressively delayed/blocked
- audit trail records blocked attempts

### S-2 Trusted proxy boundary

1. Send requests with spoofed `X-Forwarded-For` from untrusted source.
2. Compare source IP attribution in logs/audit.

Expected:
- untrusted forwarded headers are ignored
- trusted ranges only are honored

### S-3 Signature policy enforcement

1. Upload unsigned artifact under enforced ingest policy.
2. Check desired-state merge policy for signature requirements.

Expected:
- unsigned ingest blocked when enforcement enabled
- agent apply policy defaults require signature in protected mode

### S-4 Maintenance runner isolation

1. Inspect running services and mounted docker socket.

Expected:
- `maintenance-runner` has docker socket
- `control-plane` does not have docker socket mount

---

## 6) Evidence to Capture

For each run, retain:
- lab env file used (`<LAB_ROOT>/.env.onprem`, default `/tmp/hardwareops-prod-docker/.env.onprem`)
- control-plane + runner logs
- screenshots of UI pass/fail states
- `curl` outputs for key endpoints (`/healthz`, `/metrics`, cert rotation status)
- list of known failures with reproduction steps

---

## 7) Exit Criteria

A run is **pass** when:
- all P0 cases pass
- no untriaged security case failures
- any edge-case failures have mitigation + documented owner

If any P0 fails, treat build as not production-ready.
