# Security and Recovery

This guide covers the minimum security controls customers should understand and operate.

## 1. Artifact trust

HardwareOps supports:
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

See `docs/email-delivery.md` for SMTP configuration and provider examples (AWS SES, SendGrid, Google Workspace, local relay).

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
