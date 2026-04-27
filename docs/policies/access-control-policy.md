# Access Control Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All Parcel platform environments (dev, staging, production) and all personnel with access to production systems or customer data.

---

## 1. Purpose

This policy establishes requirements for granting, maintaining, and revoking access to Parcel platform systems, infrastructure, and customer data. It supports the principle of least privilege and provides an auditable trail of all access decisions.

---

## 2. Scope

Covers:
- Parcel control-plane (all environments)
- AWS infrastructure (ECS, RDS, S3, Secrets Manager, CloudWatch)
- Source code repository (GitHub)
- Deployment tooling (Terraform, CI/CD pipelines)
- Third-party SaaS tools used in platform operations

---

## 3. Access Roles

### 3.1 Platform Roles

The Parcel control-plane enforces three built-in roles:

| Role | Capability |
|------|-----------|
| `viewer` | Read-only access to devices, artifacts, audit logs |
| `operator` | All viewer actions plus deploy, enroll, manage desired state |
| `admin` | All operator actions plus user management, system configuration |

Roles are assigned at user creation and managed via `PATCH /api/v1/users/{userId}` (admin only). All role changes are recorded in the audit log (`auth.user.update` event).

### 3.2 Service Token Scopes

CI/CD pipelines and automation use scoped service tokens rather than user credentials. Available scopes: `artifact.publish`, `artifact.read`, `deployment.trigger`, `webhook.manage`, `device.read`. Tokens are created with the minimum scope required for the job, given an explicit TTL, and rotated on a schedule defined in Section 6.

### 3.3 AWS Infrastructure Roles

AWS access follows the principle of least privilege via IAM roles:
- ECS task execution role: scoped to pull images from ECR and read specific Secrets Manager ARNs
- ECS task role: scoped to the S3 bucket and Secrets Manager paths required by the running service
- Terraform deployment role: scoped to the resources managed in the customer stack
- No IAM users with long-lived access keys are used for service access; IRSA / instance profiles are used instead

---

## 4. Access Provisioning

### 4.1 Platform Users

1. New operator accounts are created by an admin via `POST /api/v1/users` with the minimum required role.
2. An invite link is sent via `POST /api/v1/users/{userId}/invite` (72-hour TTL).
3. The new user sets their password on first login via the invite redemption flow.
4. Accounts are enabled with MFA (TOTP) before being granted `operator` or `admin` roles in production.

### 4.2 AWS / Infrastructure Access

1. Access requests are submitted to the Engineering Lead via the internal ticketing system.
2. The Engineering Lead approves and applies the minimum required IAM policy change via Terraform PR.
3. The PR is reviewed by at least one other engineer before merge.
4. Temporary elevated access (e.g. for incident response) is time-bounded and removed within 24 hours of incident closure.

### 4.3 Repository Access

GitHub repository access is granted by the Engineering Lead. External contributors use fork + pull-request workflows with no direct push access to `main`.

---

## 5. Authentication Requirements

| Context | Requirement |
|---------|------------|
| Platform operator/admin accounts | Password (bcrypt, ≥ 12 chars) + TOTP MFA |
| Platform viewer accounts | Password (bcrypt, ≥ 12 chars); MFA strongly recommended |
| CI/CD service tokens | Scoped service token or OIDC workload identity (no password) |
| AWS console access | IAM Identity Center (SSO) with MFA enforced at IdP |
| AWS programmatic access | IAM roles via instance profile or IRSA; no long-lived keys |

Session JWTs have a maximum TTL of 24 hours (hardened profile). All login events — successful, failed, and backoff-blocked — are recorded in the audit log.

---

## 6. Access Review

Access is reviewed on the following schedule:

| Scope | Frequency | Reviewer | Process |
|-------|-----------|----------|---------|
| Platform user roles | Quarterly | Engineering Lead | Export user list via `GET /api/v1/users`; confirm each account is still active and role is appropriate; remove or downgrade stale accounts |
| Service tokens | Quarterly | Engineering Lead | List tokens via `GET /api/v1/auth/service-tokens`; revoke any expired or no-longer-needed tokens |
| AWS IAM roles and policies | Quarterly | Engineering Lead | Review IAM Access Analyzer findings; confirm no privilege escalation paths |
| GitHub repository members | Quarterly | Engineering Lead | Review organization member list; remove departed contributors |
| Third-party tool access | Annual | Engineering Lead | Confirm access lists for Terraform Cloud, monitoring tools, and any SaaS integrations |

Results of each review are documented in `docs/access-reviews/` with the date, reviewer, findings, and any remediation actions taken.

---

## 7. Access Revocation

Access must be revoked within the following timeframes:

| Event | Action | Deadline |
|-------|--------|----------|
| Employee or contractor offboarding | Disable platform account, revoke service tokens, remove GitHub access, remove AWS console access | Same business day |
| Role change (demotion) | Update platform role, rotate any shared service tokens the departing role had access to | Within 24 hours |
| Suspected credential compromise | Immediately revoke affected token/account; initiate IR runbook Section 4 | Immediately |
| Service token scope no longer needed | Revoke token; re-issue with reduced scope if still required | Within 5 business days of scope change |

Platform account disabling is performed via `PATCH /api/v1/users/{userId}` with `{"disabled": true}` and is recorded in the audit log.

---

## 8. Privileged Access

Admin-level platform access and AWS production write access are considered privileged. Additional controls:

- Privileged actions are performed only when operationally required.
- Admin operations in production are performed via the API (audited) rather than direct database access wherever possible.
- Direct database access (e.g. for emergency recovery) requires Engineering Lead approval, is performed via a time-bounded session, and is documented in an incident or change record.
- Break-glass local admin reset (`control-plane auth breakglass`) is available for total lockout scenarios; use is audited and requires a documented reason.

---

## 9. Shared Accounts

Shared accounts and shared credentials are prohibited for human access to production systems. All production actions must be attributable to a named individual or a named service identity.

---

## 10. Exceptions

Exceptions to this policy require written approval from the Engineering Lead and must be time-bounded. All active exceptions are tracked in `docs/access-reviews/exceptions.md`.

---

## 11. Related Documents

- `docs/deployment-hardening.md` — technical enforcement of authentication requirements
- `docs/incidents/ir-runbook.md` — response procedures for access-related incidents
- `docs/policies/change-management-policy.md`
- `docs/policies/risk-assessment-policy.md`
