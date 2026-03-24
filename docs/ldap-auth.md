# LDAP / Active Directory Authentication

## What it is

LDAP/AD authentication lets enterprise users log in with their directory credentials without standing up an OIDC provider. Authentication only — no directory sync, no SCIM. User records are created or updated in HardwareOps on first successful login and mapped to roles via group membership.

---

## 1) Configuration

Set the following environment variables on the control-plane process before startup.

| Variable | Required | Default | Description |
|---|---|---|---|
| `AUTH_LDAP_URL` | yes | — | `ldap://` or `ldaps://` URL of the directory server |
| `AUTH_LDAP_BASE_DN` | yes | — | Search base DN (e.g. `ou=users,dc=example,dc=com`) |
| `AUTH_LDAP_BIND_DN` | yes | — | Service account DN used to search the directory |
| `AUTH_LDAP_BIND_PASSWORD` | yes | — | Service account password |
| `AUTH_LDAP_USER_FILTER` | no | `(uid=%s)` | Search filter; `%s` is replaced with the submitted username. Use `(sAMAccountName=%s)` for AD |
| `AUTH_LDAP_MAIL_ATTR` | no | `mail` | LDAP attribute for user email |
| `AUTH_LDAP_DISPLAY_ATTR` | no | `displayName` | LDAP attribute for display name |
| `AUTH_LDAP_GROUP_ATTR` | no | `memberOf` | LDAP attribute listing group memberships |
| `AUTH_LDAP_ROLE_MAP` | no | `{}` | JSON map from LDAP group DN to HardwareOps role. Example: `{"CN=Admins,DC=example,DC=com":"admin"}` |
| `AUTH_LDAP_DEFAULT_ROLE` | no | `viewer` | Role assigned when no group mapping matches |

---

## 2) Quick-start examples

### 2.1 OpenLDAP

```bash
export AUTH_LDAP_URL="ldap://ldap.internal:389"
export AUTH_LDAP_BASE_DN="ou=users,dc=example,dc=com"
export AUTH_LDAP_BIND_DN="cn=hardwareops-svc,ou=serviceaccounts,dc=example,dc=com"
export AUTH_LDAP_BIND_PASSWORD="svc-account-secret"
export AUTH_LDAP_ROLE_MAP='{"cn=hw-admins,ou=groups,dc=example,dc=com":"admin","cn=hw-operators,ou=groups,dc=example,dc=com":"operator"}'
```

The default `AUTH_LDAP_USER_FILTER` of `(uid=%s)` works for most OpenLDAP deployments. `AUTH_LDAP_DEFAULT_ROLE` defaults to `viewer`, so users who belong to no mapped group get read-only access.

### 2.2 Active Directory

```bash
export AUTH_LDAP_URL="ldaps://ad.corp.example.com:636"
export AUTH_LDAP_BASE_DN="ou=Employees,dc=corp,dc=example,dc=com"
export AUTH_LDAP_BIND_DN="CN=hardwareops-svc,OU=ServiceAccounts,DC=corp,DC=example,DC=com"
export AUTH_LDAP_BIND_PASSWORD="svc-account-secret"
export AUTH_LDAP_USER_FILTER="(sAMAccountName=%s)"
export AUTH_LDAP_ROLE_MAP='{"CN=HW-Admins,OU=Groups,DC=corp,DC=example,DC=com":"admin","CN=HW-Operators,OU=Groups,DC=corp,DC=example,DC=com":"operator"}'
export AUTH_LDAP_DEFAULT_ROLE="viewer"
```

AD returns group memberships via the `memberOf` attribute by default, so `AUTH_LDAP_GROUP_ATTR` does not need to be overridden. Use `ldaps://` with port 636 — plain `ldap://` should not be used against AD in production.

---

## 3) Role mapping

`AUTH_LDAP_ROLE_MAP` is a JSON object. Keys are exact group DN strings as they appear in the user's `memberOf` attribute (or whichever attribute `AUTH_LDAP_GROUP_ATTR` specifies). Values must be one of `"admin"`, `"operator"`, or `"viewer"`.

```json
{
  "CN=HW-Admins,OU=Groups,DC=corp,DC=example,DC=com": "admin",
  "CN=HW-Operators,OU=Groups,DC=corp,DC=example,DC=com": "operator",
  "CN=HW-ReadOnly,OU=Groups,DC=corp,DC=example,DC=com": "viewer"
}
```

Evaluation order: the user's group list is scanned against the map keys; the **first match wins**. If the user belongs to no mapped group, `AUTH_LDAP_DEFAULT_ROLE` is applied.

Role assignment is re-evaluated on every login — changing a user's group membership in the directory takes effect at their next login without any manual update in HardwareOps.

> **Note:** The hardened security profile rejects `AUTH_LDAP_DEFAULT_ROLE=admin`. Set an explicit mapping for administrative groups and leave the default as `viewer` or `operator`.

---

## 4) Login API

```
POST /api/v1/auth/ldap/login
```

**Request body**

```json
{
  "username": "alice",
  "password": "her-directory-password"
}
```

**Response**

```json
{
  "token": "eyJhbGciOi...",
  "expiresAt": "2026-03-18T12:00:00Z",
  "user": {
    "id": "usr_01HZ...",
    "email": "alice@example.com",
    "displayName": "Alice Example",
    "role": "operator",
    "authProvider": "ldap"
  }
}
```

**curl example**

```bash
curl -s -X POST https://control-plane.example.com/api/v1/auth/ldap/login \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "her-directory-password"}' | jq .
```

Use the returned `token` as a Bearer token for subsequent API calls:

```bash
curl -s https://control-plane.example.com/api/v1/devices \
  -H "Authorization: Bearer eyJhbGciOi..."
```

Rate limiting applies to this endpoint on the same basis as local login — repeated failures from a single IP will be throttled.

To check whether LDAP is enabled on a running instance:

```bash
curl -s https://control-plane.example.com/api/v1/auth/status | jq .ldapEnabled
# true
```

---

## 5) UI behavior

When `AUTH_LDAP_URL` is set, the login page shows a **Sign in with LDAP** tab alongside the local login form. Clicking the tab reveals username and password fields. On a successful bind, the browser session works identically to a local or OIDC login — the same JWT cookie is issued, the same RBAC rules apply, and the session expires on the same schedule.

---

## 6) User lifecycle

- **First login:** A new user record is created with `auth_provider=ldap` and `external_id` set to the user's DN returned by the directory search.
- **Subsequent logins:** `last_login_at` is updated and the role is re-evaluated from the user's current group membership.
- **Disabled in HardwareOps:** If an operator disables the user via the admin API (`PATCH /api/v1/users/{userId}` with `{"disabled": true}`), login is rejected even when the LDAP credentials are valid.
- **Directory deprovisioning:** HardwareOps does not continuously sync with the directory. If a user is removed from the directory, they will fail to authenticate because the credential rebind fails. To immediately revoke access, disable the HardwareOps user record:

```bash
curl -s -X PATCH https://control-plane.example.com/api/v1/users/usr_01HZ... \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"disabled": true}'
```

Disabling the record causes all existing JWTs to be rejected at their next validation (on or before expiry).

---

## 7) Security notes

- Credentials are verified by rebinding to LDAP with the user DN and the supplied password. HardwareOps never stores directory passwords.
- Use `ldaps://` (LDAP over TLS, port 636) in production. Plain `ldap://` is acceptable only for local dev or localhost-bound directories.
- Failed login attempts emit a `user.login_failed` audit event including the attempted username and source IP.
- Successful logins emit a `user.login` audit event with `actor=ldap:<userDN>`.
- Rate limiting (identical to local login) applies to the `/api/v1/auth/ldap/login` endpoint.

---

## 8) Verification

After configuring the environment variables and restarting the control-plane, run through these steps to confirm the integration is working.

**1. Confirm LDAP is active**

```bash
curl -s https://control-plane.example.com/api/v1/auth/status | jq .ldapEnabled
# Expected: true
```

**2. Test a login with known-good credentials**

```bash
curl -s -X POST https://control-plane.example.com/api/v1/auth/ldap/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpassword"}' | jq '{token: .token, role: .user.role}'
```

A successful response includes a non-empty `token` and the expected role.

**3. Verify the audit log shows a login event**

```bash
curl -s "https://control-plane.example.com/api/v1/audit?eventType=user.login&limit=5" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '.[].actor'
# Expected: "ldap:cn=testuser,ou=users,dc=example,dc=com"
```

**4. Confirm the user record was created**

```bash
curl -s "https://control-plane.example.com/api/v1/users?authProvider=ldap" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '.[] | {email, role, authProvider}'
```

The user should appear with `"authProvider": "ldap"` and the role derived from group mapping.

---

## Related docs

- `docs/auth-secrets-v1.md` — managing service account credentials and secret rotation
- `docs/icd.md` — full API contract including `/api/v1/auth/*` and `/api/v1/users` endpoints
- `docs/development/phase-c-internals.md` — developer internals for the auth subsystem
