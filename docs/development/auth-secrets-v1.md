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
