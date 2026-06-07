# Email Delivery

Parcel supports optional SMTP email delivery for two workflows:

1. **Self-service forgot-password** — a user who is locked out clicks "Forgot password" on the login screen, enters their email, and receives a reset link.
2. **Admin invite / token delivery** — an admin triggers `POST /api/v1/users/{userId}/invite` (or enables `sendEmail` on `POST /api/v1/users/{userId}/password-reset-token`) and the recipient receives a link directly.

When `SMTP_HOST` is not set, a silent no-op mailer is used: all existing operator-token and recovery-code flows continue to work, and the UI hides SMTP-dependent controls.

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `APP_PUBLIC_URL` | _(empty)_ | Base URL of the UI, used to build reset links in emails. Example: `https://parcel.internal` |
| `SMTP_HOST` | _(empty)_ | SMTP server hostname. Leave blank to disable email delivery. |
| `SMTP_PORT` | `587` | SMTP port. 587 = STARTTLS (recommended), 465 = implicit TLS, 25 = plain. |
| `SMTP_USER` | _(empty)_ | SMTP username. Leave blank for unauthenticated relays. |
| `SMTP_PASSWORD` | _(empty)_ | SMTP password. |
| `SMTP_FROM` | `Parcel <noreply@example.com>` | Sender address shown in emails. |
| `SMTP_TLS_MODE` | `starttls` | TLS mode: `starttls`, `tls`, or `none`. |
| `SMTP_TIMEOUT` | `10s` | Dial + send timeout per message. |
| `SMTP_SKIP_VERIFY` | `0` | Skip TLS certificate verification. **Dev only.** Blocked when `HARDENED_PROFILE=1`. |

---

## TLS Mode Guide

| Mode | Port | How it works | When to use |
|---|---|---|---|
| `starttls` | 587 | Dial plain TCP → STARTTLS upgrade → send | Cloud SMTP relays (AWS SES, SendGrid, Google Workspace) |
| `tls` | 465 | Dial directly over TLS (SMTPS) → send | Providers that require implicit TLS |
| `none` | 25 | Plain TCP, no encryption | Local Postfix relay on loopback only |

---

## Provider Examples

### AWS SES SMTP Relay

```
SMTP_HOST=email-smtp.us-east-1.amazonaws.com
SMTP_PORT=587
SMTP_USER=<SES SMTP username>
SMTP_PASSWORD=<SES SMTP password>
SMTP_FROM=Parcel <noreply@yourverifieddomain.com>
SMTP_TLS_MODE=starttls
APP_PUBLIC_URL=https://parcel.yourdomain.com
```

### SendGrid

```
SMTP_HOST=smtp.sendgrid.net
SMTP_PORT=587
SMTP_USER=apikey
SMTP_PASSWORD=<your SendGrid API key>
SMTP_FROM=Parcel <noreply@yourverifieddomain.com>
SMTP_TLS_MODE=starttls
APP_PUBLIC_URL=https://parcel.yourdomain.com
```

### Google Workspace Relay

```
SMTP_HOST=smtp-relay.gmail.com
SMTP_PORT=587
SMTP_USER=admin@yourdomain.com
SMTP_PASSWORD=<app password>
SMTP_FROM=Parcel <noreply@yourdomain.com>
SMTP_TLS_MODE=starttls
APP_PUBLIC_URL=https://parcel.yourdomain.com
```

### Local Postfix Relay (dev / on-prem)

```
SMTP_HOST=localhost
SMTP_PORT=25
SMTP_TLS_MODE=none
SMTP_FROM=parcel@yourdomain.com
APP_PUBLIC_URL=https://parcel.internal
```

---

## How Reset Links Work

When an email is sent, the link has the form:

```
{APP_PUBLIC_URL}/?view=reset-token&email=alice%40corp.example&token=<plaintext-token>
```

- The UI detects these URL parameters on load and automatically switches to the reset-token view with the form pre-filled.
- Tokens are **one-time use** — once consumed they cannot be reused.
- **Forgot-password tokens** expire after **30 minutes**.
- **Invite tokens** expire after **72 hours** (recipients may not act immediately).
- Tokens issued by admins via the operator panel have a configurable TTL (default 15 minutes, max 24 hours).

---

## Testing Without a Real Mail Server

Use [Mailhog](https://github.com/mailhog/MailHog) or [Mailtrap](https://mailtrap.io) in dev:

```bash
# Run Mailhog locally (catches all outbound mail)
docker run -p 1025:1025 -p 8025:8025 mailhog/mailhog

# Point Parcel at Mailhog
SMTP_HOST=localhost
SMTP_PORT=1025
SMTP_TLS_MODE=none
SMTP_SKIP_VERIFY=0   # not needed for plain
APP_PUBLIC_URL=https://localhost:8080
```

Open `http://localhost:8025` to view captured emails.

---

## Security Notes

- **User existence is never revealed** via the `POST /api/v1/auth/forgot-password` endpoint. The response is always HTTP 200 with the same message, regardless of whether the email is registered.
- `SMTP_SKIP_VERIFY=1` disables TLS certificate verification and is intended for local dev only. It is explicitly blocked when `HARDENED_PROFILE=1`.
- Audit events for forgot-password requests record whether an email was found and sent (`emailFound`, `emailSent`), but **not** the user ID, to prevent log-based enumeration.
