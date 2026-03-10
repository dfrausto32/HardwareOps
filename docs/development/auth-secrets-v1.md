# V1 Auth + Secrets Spec (Local Users)

This document defines the **v1 authentication and secrets posture** for HardwareOps. It prioritizes **local users** while leaving explicit extension points for **OIDC/SSO** later.

## Goals
- Provide a secure, minimal auth layer for v1 (local users + JWT).
- Enforce fixed roles (admin / operator / viewer) at the API layer.
- Preserve audit actor identity so future SSO/SSO groups can map cleanly.
- Keep secrets management simple for v1 (env/file), but define upgrade paths.

## Non‑Goals (v1)
- Full OIDC/SSO integration.
- UI user management and password reset flows.
- MFA, password complexity enforcement beyond bcrypt hashing.

## Auth Model (v1)
**Mode:** Local users only (`AUTH_MODE=local`)

**Token:** JWT (HS256) with:
- `sub` = `userId`
- `email`
- `roles[]`
- `iss` = `AUTH_ISSUER`
- `exp` = `AUTH_TOKEN_TTL`

**Headers:** `Authorization: Bearer <token>`

**Fallback:** `?token=` query parameter (for WebSocket in dev).

### Roles
Fixed role set (no custom roles in v1):
- **Admin**: users, auth config, maintenance, upgrades, audit.
- **Operator**: groups, devices, artifacts, desired state.
- **Viewer**: read‑only dashboards, logs, audit (if permitted).

Role enforcement is via API middleware, not UI. The UI can optionally hide actions.

## API Endpoints (v1)
### Auth
- `POST /api/v1/auth/login` → returns JWT + expiry.
- `GET /api/v1/auth/me` → returns current user identity.
- `GET /api/v1/auth/status` → returns `{enabled, mode}` for UI gating.
- `POST /api/v1/auth/register` → create user using a voucher token.

### Vouchers (Admin only)
- `POST /api/v1/auth/vouchers` → create invite voucher (returns token once).

### Users (Admin only)
- `POST /api/v1/users` → create local user.
- `GET /api/v1/users` → list users.
- `PATCH /api/v1/users/{userId}` → update display name, roles, disabled, password.

## Storage Schema (v1)
`users` table:
- `user_id` (uuid)
- `email` (unique)
- `display_name`
- `password_hash` (bcrypt)
- `roles` (jsonb array)
- `disabled` (bool)
- `auth_provider` (local now)
- `external_id` (reserved for SSO)
- `created_at`, `updated_at`, `last_login_at`

## Bootstrap Admin
When `AUTH_MODE=local`, a **bootstrap admin** can be created if no users exist:
- `AUTH_BOOTSTRAP_EMAIL`
- `AUTH_BOOTSTRAP_PASSWORD`

This is intended for first‑time setup and should be rotated once real users are created.

### Admin Secret Handling (v1)
Treat `AUTH_BOOTSTRAP_PASSWORD` like a **root credential**:
- Store it in a password manager or host secret store (not in git).
- Rotate after first login by creating a new admin and disabling the bootstrap user.
- Avoid sharing it broadly; use it only for initial setup or emergency recovery.

## Voucher Workflow (v1)
1) Admin signs in and creates a voucher with roles + TTL.
2) New user redeems voucher via `/auth/register` with email + password.
3) System marks voucher as used (one‑time use).

Optional: vouchers can be restricted to a specific email.

## Audit Actor Identity
Audit events store:
- actor type (`user` / `device` / `system`)
- actor id / email
- actor roles
- auth method (local / mtls / future oidc)

When auth is enabled, actor identity is derived from the JWT subject and roles.

## Secrets (v1)
Secrets are **env/file based** for v1:
- `AUTH_JWT_SECRET`
- `AUTH_BOOTSTRAP_PASSWORD`
- Object store credentials (`S3_ACCESS_KEY`, `S3_SECRET_KEY`)
- Signing keys (artifact signing)

Recommended storage:
- `.env` on single‑node dev
- OS secret store or systemd drop‑in on servers

## OIDC/SSO Drop‑ins (v2)
Reserved config for future:
- `AUTH_MODE=oidc`
- `AUTH_OIDC_ISSUER`
- `AUTH_OIDC_CLIENT_ID`
- Role mapping: OIDC groups → fixed roles

Implementation will keep **local break‑glass admin** as a fallback.

## Acceptance Criteria (v1)
- Local users can log in and receive JWTs.
- Role middleware blocks unauthorized actions.
- Audit logs record actor identity and auth method.
- Secrets are documented and can be injected via env.

---

## OIDC SSO Integration (v2)

OIDC authorization code flow is now supported alongside local auth. Both modes can be active simultaneously.

### Environment Variables

| Variable | Type | Default | Description |
|---|---|---|---|
| `AUTH_OIDC_ISSUER` | string | _(unset)_ | OIDC issuer URL (e.g. `https://accounts.google.com`). Setting this enables OIDC. |
| `AUTH_OIDC_CLIENT_ID` | string | _(required when OIDC enabled)_ | OAuth2 client ID registered with the IdP. |
| `AUTH_OIDC_CLIENT_SECRET` | string | _(required when OIDC enabled)_ | OAuth2 client secret. Store in a secrets manager, not git. |
| `AUTH_OIDC_REDIRECT_URL` | string | _(required when OIDC enabled)_ | Callback URL registered with the IdP (e.g. `https://hwops.example.com/api/v1/auth/oidc/callback`). |
| `AUTH_OIDC_SCOPES` | string | `openid email profile groups` | Space or comma separated OIDC scopes to request. |
| `AUTH_OIDC_GROUP_CLAIM` | string | `groups` | Name of the claim in the ID token that contains the user's groups. |
| `AUTH_OIDC_ROLE_MAP` | string (JSON) | _(unset)_ | JSON object mapping IdP group names to HardwareOps roles. Example: `{"hwops-admins":"admin","hwops-ops":"operator"}`. Required when `HARDENED_PROFILE=1`. |
| `AUTH_OIDC_DEFAULT_ROLE` | string | `viewer` | Role assigned to users whose groups do not match any entry in `AUTH_OIDC_ROLE_MAP`. Cannot be `admin` when `HARDENED_PROFILE=1`. |

### Role Map JSON Format

`AUTH_OIDC_ROLE_MAP` is a flat JSON object where keys are IdP group names and values are HardwareOps role names (`admin`, `operator`, `viewer`).

```json
{
  "hwops-admins": "admin",
  "hwops-operators": "operator",
  "hwops-viewers": "viewer"
}
```

When a user belongs to multiple groups, the highest-precedence role wins: `admin > operator > viewer`.

### State Cookie

| Property | Value |
|---|---|
| Cookie name | `hwops_oidc_state` |
| TTL | 600 seconds (10 minutes) |
| Flags | `HttpOnly`, `SameSite=Lax`, `Path=/` |

The state cookie prevents CSRF during the OAuth2 redirect flow.

### OIDC Auth Flow

1. UI calls `GET /api/v1/auth/status` — response now includes `oidcEnabled` and `oidcLoginURL`.
2. User clicks "Sign in with SSO" — browser navigates to `/api/v1/auth/oidc/login`.
3. Server generates state, sets cookie, redirects to IdP.
4. IdP authenticates user and redirects to `/api/v1/auth/oidc/callback?code=...&state=...`.
5. Server verifies state cookie, exchanges code for tokens, upserts user, issues JWT.
6. Server redirects browser to `/?oidc_token=<jwt>`.
7. SPA detects `oidc_token` query param, persists the token, clears param from URL.

### User Upsert Logic

On each OIDC login:
1. Look up user by `external_id = sub` and `auth_provider = oidc`.
2. If not found, look up by `email` (account linking with existing local user).
3. If still not found, create new user with `auth_provider=oidc` and `external_id=sub`.
4. Update roles from IdP groups on every login.

### Audit Events

| Action | When |
|---|---|
| `auth.oidc.login` | Successful OIDC sign-in |
| `auth.oidc.login.failed` | Failed OIDC sign-in (state mismatch, exchange error, IdP error) |

### Per-IdP Quickstart

#### Okta

1. Create an OIDC Web Application in Okta.
2. Set redirect URI to `https://<your-domain>/api/v1/auth/oidc/callback`.
3. Enable "Groups" claim in the ID token (Okta → Application → Sign On → OpenID Connect ID Token → Groups claim filter).
4. Set env vars:
   ```
   AUTH_OIDC_ISSUER=https://<your-okta-domain>/oauth2/default
   AUTH_OIDC_CLIENT_ID=<client-id>
   AUTH_OIDC_CLIENT_SECRET=<client-secret>
   AUTH_OIDC_REDIRECT_URL=https://<your-domain>/api/v1/auth/oidc/callback
   AUTH_OIDC_GROUP_CLAIM=groups
   AUTH_OIDC_ROLE_MAP={"hwops-admins":"admin","hwops-operators":"operator"}
   ```

#### Azure AD (Entra ID)

1. Register an application in Entra ID (App registrations).
2. Set redirect URI (Web) to `https://<your-domain>/api/v1/auth/oidc/callback`.
3. Add a `groups` claim via Token configuration → Add groups claim → Security groups.
4. Create a client secret under Certificates & secrets.
5. Set env vars:
   ```
   AUTH_OIDC_ISSUER=https://login.microsoftonline.com/<tenant-id>/v2.0
   AUTH_OIDC_CLIENT_ID=<application-client-id>
   AUTH_OIDC_CLIENT_SECRET=<client-secret-value>
   AUTH_OIDC_REDIRECT_URL=https://<your-domain>/api/v1/auth/oidc/callback
   AUTH_OIDC_GROUP_CLAIM=groups
   AUTH_OIDC_SCOPES=openid email profile groups
   AUTH_OIDC_ROLE_MAP={"<group-object-id-for-admins>":"admin","<group-object-id-for-ops>":"operator"}
   ```
   Note: Entra groups appear as object IDs in the token by default.

#### Google Workspace

1. Create an OAuth2 Web Application credential in Google Cloud Console.
2. Set authorized redirect URI to `https://<your-domain>/api/v1/auth/oidc/callback`.
3. Google does not include group membership in standard OIDC tokens. Use the Admin SDK Directory API or a custom claim via a Google Workspace Marketplace app.
4. Set env vars:
   ```
   AUTH_OIDC_ISSUER=https://accounts.google.com
   AUTH_OIDC_CLIENT_ID=<client-id>.apps.googleusercontent.com
   AUTH_OIDC_CLIENT_SECRET=<client-secret>
   AUTH_OIDC_REDIRECT_URL=https://<your-domain>/api/v1/auth/oidc/callback
   AUTH_OIDC_SCOPES=openid email profile
   AUTH_OIDC_DEFAULT_ROLE=viewer
   # AUTH_OIDC_GROUP_CLAIM and AUTH_OIDC_ROLE_MAP require custom claim setup
   ```
