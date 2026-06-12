# Security and Recovery

This guide covers the minimum security controls customers should understand and operate.

## 1. Artifact trust

Parcel supports:
- unsigned artifact acceptance where appropriate
- warning-only trust modes
- strict verified-only artifact modes

Recommended:
- connected/managed environments: `require_verified`
- controlled on-prem with gradual rollout: `warn_unsigned`
- fully disconnected lab environments only: `allow_unsigned`

If trusted signing keys are configured, keep those key IDs documented and rotated through change control.

## 2. Local authentication recovery

Four recovery paths are available, from self-service to emergency:

### Layer 1 — Self-service recovery codes (airgapped-safe)

Users generate one-time recovery codes from the UI (Settings → Security). These work with no email or admin involvement and are the recommended default for on-prem and airgapped deployments.

### Layer 2 — Admin-issued reset token

An admin issues a short-lived one-time reset token for a user:

```
POST /api/v1/users/{userId}/password-reset-token
{ "reason": "user locked out", "ttlMinutes": 15 }
```

The response contains `token`. The admin delivers it out-of-band. When `sendEmail: true` is included and SMTP is configured, the link is emailed directly to the user instead.

### Layer 3 — Email-based forgot-password (connected deployments)

When `SMTP_HOST` is configured, the login screen shows a **Forgot password** tab. The user enters their email address and receives a reset link valid for 30 minutes.

To confirm SMTP is active before relying on this path:

```bash
curl -s https://<host>/api/v1/auth/status | jq .smtpEnabled
# true → forgot-password email flow is available
```

To invite a new user directly by email (sends a 72-hour account-setup link):

```
POST /api/v1/users/{userId}/invite
```

See `docs/reference/email-delivery.md` for SMTP configuration and provider examples (AWS SES, SendGrid, Google Workspace, local relay).

### Layer 4 — Break-glass local CLI (total lockout)

For when all other paths are unavailable:

```bash
control-plane auth breakglass reset-password --email admin@example.com --reason "..."
control-plane auth breakglass create-admin --email recovery@example.com --reason "..."
```

Requires local shell access on the control-plane host. All actions are audited.

## 3. Password reset recommendations

- Keep at least two admin accounts
- Generate recovery codes for all users during rollout
- Configure SMTP in connected deployments so users can self-serve
- Document who can issue reset tokens and what the expected TTL is
- Reserve break-glass for emergency use only — it requires host access

## 4. Proxy trust

Do not trust all proxies.

Use exact proxy or gateway CIDRs only:

```env
TRUST_PROXY=1
TRUST_PROXY_CIDRS=<restricted-cidrs-only>
```

## 5. Bootstrap and runtime secrets

At install time, rotate:
- `AUTH_JWT_SECRET`
- `AUTH_BOOTSTRAP_PASSWORD`
- `BOOTSTRAP_TOKEN`

If using service tokens for CI fallback:
- keep them short-lived
- rotate them regularly
- prefer workload identity where supported

## 6. Audit expectations

Critical operations should always leave audit records:
- login and recovery actions
- service token lifecycle
- workload identity exchange
- artifact registration
- certificate rotation
- device decommission
- artifact deletion (`artifact.delete`) and force deletion (`artifact.force_delete`)

## 7. Ransomware protections

Parcel ships layered controls against the two ransomware blast radii — destruction of the control plane's data, and abuse of Parcel as a delivery channel to your fleet (see `docs/compliance/policies/ransomware-protection-policy.md` for the full policy):

**Always on (no configuration):**
- Artifact deletion requires a two-phase soft delete (30-day deprecation window). Bypassing it with `force=true` requires the **admin** role and is audited under a distinct `artifact.force_delete` action.
- Artifact ingest and deletion are rate limited (`ARTIFACT_UPLOAD_RPM`, default 60/min; `ARTIFACT_DELETE_RPM`, default 20/min).
- Bulk-deletion anomaly detection: more than 10 deletions/deprecations by one actor within 5 minutes raises a `security.anomaly.bulk_artifact_deletion` event on the live event stream and an ERROR log. Treat it as a P1 until scoped.
- Agents verify artifact signatures independently before applying — a compromised control-plane API cannot push an executable payload to devices without the signing key.

**Opt-in (recommended for production):**
- **Object Lock / WORM** on the artifact store: AWS via the `artifact_store_enable_object_lock` Terraform variable; on-prem MinIO via `S3_OBJECT_LOCK=1` (+ `S3_OBJECT_LOCK_RETENTION_DAYS`, default 35). Locked objects cannot be deleted or overwritten within retention, even by the application credential. Object locking can only be enabled at bucket creation.
- **Isolated backup bucket** (AWS, on by default in the `customer_stack` module): the application role can only *write* backups — it cannot read, delete, or re-policy them — so a compromised credential cannot destroy the backups alongside the primary store.

**If you suspect an active event:** enable maintenance mode first (`MAINTENANCE_MODE=1`) to halt all artifact delivery to the fleet, then revoke the suspect credential. The full playbook is in `docs/incidents/ir-runbook.md` §9.
