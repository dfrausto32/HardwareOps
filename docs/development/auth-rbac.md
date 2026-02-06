# Auth + RBAC Roadmap

This doc outlines **user management**, **RBAC**, and a **break‑glass** access model.

## Recommendation (default)
- **Auth**: OIDC SSO (Google/Microsoft/Okta) + local break‑glass admin
- **RBAC**: fixed roles (admin / operator / viewer)

## Auth Modes

### A) Local Users (simple)
**Use when:** air‑gapped or no IdP.
- Local user store
- Password policy + rotation
- Manual user creation

### B) OIDC SSO (recommended)
**Use when:** most customers.
- OIDC provider (Okta, Azure AD, Google)
- Map IdP groups → roles
- Local break‑glass admin for emergencies

### C) LDAP/AD
**Use when:** existing enterprise directory.
- LDAP sync to local users/roles
- Local break‑glass admin for outages

## RBAC (Fixed Roles)
- **Admin**: users, auth config, secrets, system settings.
- **Operator**: groups, devices, artifacts, desired state.
- **Viewer**: read‑only dashboards/logs.

## Break‑Glass Access (Backdoor)
**Recommended approach (hybrid):**
1) **OIDC Break‑glass group** (primary)
   - Limited to a small on‑call team.
   - Audited actions.
2) **Local bootstrap admin** (fallback)
   - One‑time password printed during install.
   - Must be rotated on first use.
   - Can be disabled after bootstrap if desired.

This satisfies customers who want strict control but still require emergency access.

## Secrets and Passwords
- Use a **secrets manager** (Vault / AWS Secrets Manager) when available.
- Hash local passwords with **bcrypt/argon2**.
- Store artifact repo credentials in a sealed secret store.

## Auditability (Roadmap)
- Log user actions: login, role changes, deploy, delete.
- Export audit logs to SIEM.
