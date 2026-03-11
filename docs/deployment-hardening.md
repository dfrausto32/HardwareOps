# Deployment Hardening Guide (Proxy Trust + Anti-Tamper)

Canonical deployment flow lives in `deploy.md`.  
Use this file as the detailed hardening reference.

This guide covers how to deploy with safe proxy-header trust settings in:
- AWS (Terraform-based production stacks)
- On-prem bundles

It also covers how to make config tampering difficult for operators with host access.

---

## 1) Why this matters

Control-plane now only trusts forwarded headers (`X-Forwarded-For`, `X-Real-IP`, `X-Client-Cert`) when the immediate remote peer is in `TRUST_PROXY_CIDRS`.

If this is misconfigured, attackers can spoof source IP/cert headers and bypass identity/rate-limit assumptions.

---

## 2) AWS production deployment

### What is already wired

Terraform module default env for control-plane includes:
- `TRUST_PROXY=1`
- `TRUST_PROXY_CIDRS=<private_subnet_cidrs>`

Source: `deploy/aws/terraform/modules/customer_stack/main.tf`.

That means only traffic forwarded by gateway tasks running from the ECS private subnets is trusted for forwarded headers.

### Optional stricter override

You can set `control_plane_env` in `terraform.tfvars` to narrower CIDRs (for example dedicated gateway subnets only):

```hcl
control_plane_env = {
  TRUST_PROXY_CIDRS = "10.40.64.0/20,10.40.80.0/20,10.40.96.0/20"
}
```

### Verify after deploy

1) Inspect effective task env in ECS task definition:

```bash
aws ecs describe-task-definition --task-definition <task-def-arn> \
  --query 'taskDefinition.containerDefinitions[?name==`control-plane`].environment'
```

2) Confirm values include:
- `TRUST_PROXY=1`
- `TRUST_PROXY_CIDRS` matches the private subnet CIDRs (or your explicit override)

3) Validate behavior:
- Requests from untrusted remote IP with spoofed `X-Forwarded-For` must not change rate-limit/audit source IP.
- Requests from trusted proxy IP should use forwarded value.

---

## 3) On-prem deployment hardening

### Required runtime settings

In `.env.onprem`:

```env
TRUST_PROXY=1
TRUST_PROXY_CIDRS=<gateway-or-proxy-subnets>
CLIENT_CERT_HEADER=X-Client-Cert
HARDENED_PROFILE=1
AUTH_MODE=local
AUTH_JWT_SECRET=<32+ char random secret>
AUTH_BOOTSTRAP_PASSWORD=<12+ char random password>
AUTH_TOKEN_TTL=12h
AUTH_LOGIN_RPM=30
AUTH_LOGIN_BACKOFF_ENABLED=1
AUTH_LOGIN_BACKOFF_THRESHOLD=3
AUTH_LOGIN_BACKOFF_BASE=2s
AUTH_LOGIN_BACKOFF_MAX=5m
AUTH_LOGIN_BACKOFF_WINDOW=15m
LICENSE_ENFORCE=1
DEVICE_IDENTITY_MODE=enforce
DEVICE_IDENTITY_REQUIRE_ON_ENROLL=1
DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=1
ARTIFACT_PULL_ALLOWED_HOSTS=<comma-separated approved pull hosts>
ARTIFACT_PULL_ALLOW_INSECURE_HTTP=0
ARTIFACT_TRUST_VERIFICATION_MODE=require_verified
ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS=<comma-separated key ids>
ARTIFACT_TRUST_ALLOWED_SIGNATURE_TYPES=ed25519,cosign
TRUSTED_SIGNING_KEYS_FILE=/opt/hardwareops/signing/trusted-signing-keys.json
ARTIFACT_SIGNATURE_REQUIRE_DEFAULT=1
ARTIFACT_SIGNATURE_ENFORCE_INGEST=1
ARTIFACT_SIGNATURE_KEY_ID=<pinned signing key id>
UPGRADE_RUNNER_MODE=remote
UPGRADE_RUNNER_URL=http://maintenance-runner:8090
UPGRADE_RUNNER_TOKEN=<16+ char random token>
BACKUP_RUNNER_MODE=remote
BACKUP_RUNNER_URL=http://maintenance-runner:8090
BACKUP_RUNNER_TOKEN=<16+ char random token>
```

With `HARDENED_PROFILE=1`, `TRUST_PROXY=1` now requires `TRUST_PROXY_CIDRS` to be explicitly set, and artifact trust bootstrap must include a strict verification mode plus a trusted signing key source. Do not rely on broad private-network fallbacks.

Do **not** use:
- `TRUST_PROXY_CIDRS=0.0.0.0/0`
- `TRUST_PROXY_CIDRS=::/0`
- `TRUST_PROXY_CIDRS=10.0.0.0/8`
- `TRUST_PROXY_CIDRS=172.16.0.0/12`
- `TRUST_PROXY_CIDRS=192.168.0.0/16`
- `TRUST_PROXY_CIDRS=100.64.0.0/10`
- `TRUST_PROXY_CIDRS=fc00::/7`
- `TRUST_PROXY_CIDRS=fe80::/10`
- Placeholder secrets/tokens such as `change-me`, `dev-token`, or `password`

Use only your reverse proxy host/subnet(s).

### Privilege boundary requirement

Keep Docker socket privilege out of the control-plane container:

- `control-plane` must run with `UPGRADE_RUNNER_MODE=remote` and `BACKUP_RUNNER_MODE=remote`.
- Only `maintenance-runner` should mount `/var/run/docker.sock`.
- Rotate `UPGRADE_RUNNER_TOKEN` / `BACKUP_RUNNER_TOKEN` like any other operational secret.

### Verify running container values

```bash
docker exec -it hardwareops-control-plane-1 env | egrep 'TRUST_PROXY|TRUST_PROXY_CIDRS|CLIENT_CERT_HEADER'
```

And verify hardening profile variables:

```bash
docker exec -it hardwareops-control-plane-1 env | egrep 'HARDENED_PROFILE|AUTH_MODE|LICENSE_ENFORCE|DEVICE_IDENTITY_MODE|DEVICE_IDENTITY_REQUIRE_ON_ENROLL|DEVICE_IDENTITY_REQUIRE_ON_CHECKIN'
```

And verify remote runner wiring:

```bash
docker exec -it hardwareops-control-plane-1 env | egrep 'UPGRADE_RUNNER_MODE|UPGRADE_RUNNER_URL|BACKUP_RUNNER_MODE|BACKUP_RUNNER_URL'
docker compose -f docker-compose.onprem.bundle.yml ps maintenance-runner
```

---

## 4) Make on-prem config hard to change

If customers have root, tampering cannot be made impossible, but it can be made costly/noisy.

### Baseline controls

1) Restrict file ownership/permissions:

```bash
sudo chown root:root .env.onprem docker-compose.onprem.bundle.yml
sudo chmod 0400 .env.onprem
sudo chmod 0444 docker-compose.onprem.bundle.yml
```

2) Set immutable bit (Linux ext filesystems):

```bash
sudo chattr +i .env.onprem docker-compose.onprem.bundle.yml
```

3) Restrict Docker access:
- Remove non-admin users from `docker` group.
- Use sudo-only operational access.

4) Keep deployment path root-owned (`/opt/hardwareops/stack`) and writable by admins only.

### Operational controls (recommended)

1) Store a known-good checksum for `.env.onprem`:

```bash
sha256sum .env.onprem > .env.onprem.sha256
```

2) During upgrades/restarts, verify checksum before `docker compose up`.

3) Monitor container env drift:
- Alert if `TRUST_PROXY`/`TRUST_PROXY_CIDRS` changes from baseline.

---

## 5) Recommendation profile

For customer deployments:

1. Enable `TRUST_PROXY=1`.
2. Set `TRUST_PROXY_CIDRS` to exact proxy/gateway network ranges.
3. Lock `.env.onprem` + compose file ownership/permissions and immutable bit.
4. Add checksum verification in deployment SOP.

This gives strong practical protection against header spoofing and casual tampering.

---

## 6) Device Identity Policy (Anti-Clone)

### Goal

Make "one physical device = one enrolled identity" harder to bypass. Not tamper-proof against a fully hostile host, but raises the bar against accidental cloning and simple copy/replay abuse.

### Identity source

Agents include a hardware identity in `capabilities.hw.identity` on check-in:
- `id`: stable SHA-256 hash derived from machine-id (fallback: hostname, or `HARDWARE_IDENTITY` override).
- `source`: where the identity was derived (`machine-id`, `hostname`, `env`, demo-specific).

### Policy controls

```env
DEVICE_IDENTITY_MODE=audit          # disabled | audit | enforce
DEVICE_IDENTITY_REQUIRE_ON_ENROLL=0 # set to 1 in enforce mode to reject missing identity on enroll
DEVICE_IDENTITY_REQUIRE_ON_CHECKIN=0 # set to 1 in enforce mode to reject missing identity on check-in
```

With `HARDENED_PROFILE=1` and `DEVICE_IDENTITY_MODE=enforce`, both require flags default on.

Modes:
- `disabled`: no hardware-identity checks.
- `audit`: detect conflicts and emit telemetry/audit without blocking traffic.
- `enforce`: reject identity conflicts with HTTP `409`.

### Server behavior

**Enrollment** (`POST /api/v1/devices/enroll`):
- Rejects with `409` if hardware identity already belongs to another device.
- Stores hardware identity in device metadata when present.

**Check-in** (`POST /api/v1/devices/checkin`):
- Detects identity mismatch/reuse and emits runtime + audit signals.
- In `enforce` mode, rejects conflicts with `409`.

### Signals

- Runtime event: `device.identity_conflict`
- Audit action: `device.identity_violation` (and `device.enroll_rejected` on enroll conflicts)

### Verification quick check

```bash
# 1. Start control-plane with enforce mode
DEVICE_IDENTITY_MODE=enforce ./scripts/run-control-plane.sh

# 2. Enroll/check-in one device normally.
# 3. Attempt a second enroll/check-in with the same hardware identity.
# 4. Expect:
#    - HTTP 409 on the conflicting request
#    - device.identity_conflict event in runtime event history
#    - device.identity_violation entry in audit log
```

### Limits

- Helps against accidental cloning and simple copy/replay.
- Not tamper-proof on fully hostile hardware; a privileged attacker can forge software-reported identity.
- Stronger guarantees require hardware attestation (TPM/secure element), deferred to a later phase.
