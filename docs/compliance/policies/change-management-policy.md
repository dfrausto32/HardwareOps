# Change Management Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All changes to production Parcel platform code, infrastructure, and configuration.

---

## 1. Purpose

This policy ensures that changes to the Parcel platform are reviewed, tested, and deployed in a controlled manner that minimizes risk to availability, security, and data integrity.

---

## 2. Scope

Covers:
- Application code (control-plane, agent, UI)
- Infrastructure-as-code (Terraform modules and environment configurations)
- Database schema migrations
- Platform configuration (environment variables, secrets rotation)
- Third-party dependency updates

---

## 3. Change Categories

| Category | Examples | Review Required | Testing Required | Approval |
|----------|----------|-----------------|------------------|----------|
| **Standard** | Bug fixes, minor feature additions, dependency patches | Peer code review | Unit + integration tests pass | 1 engineer |
| **Significant** | New authentication flows, schema migrations, new external integrations | Peer code review + Engineering Lead review | Full test suite + staging validation | Engineering Lead |
| **Emergency** | Active incident hotfixes, security patches for actively-exploited CVEs | Async review (post-deploy review within 24h) | Smoke tests at minimum | Engineering Lead (verbal ok acceptable) |
| **Infrastructure** | Terraform changes to production environment | Peer review of `terraform plan` output | `terraform validate`; staging apply first | Engineering Lead |

---

## 4. Standard Change Process

### 4.1 Development

1. Work is performed on a feature branch named `feature/<description>` or `fix/<description>`.
2. Commits are descriptive and reference any relevant issue or compliance ticket.
3. Database migrations use sequential numbered files (`migrations/NNNN_<name>.sql`) and are additive where possible (no destructive migrations without Engineering Lead sign-off).

### 4.2 Code Review

1. A pull request (PR) is opened against `main`.
2. The PR description must include: what changed, why, testing performed, and any rollback plan.
3. At least one engineer other than the author reviews and approves the PR before merge.
4. Significant changes require Engineering Lead review.
5. The CI pipeline must pass (build, lint, unit tests, RBAC tests) before merge is permitted.

### 4.3 Staging Validation

Before deploying to production:
1. The change is deployed to the staging environment.
2. Relevant integration scripts are run (`scripts/artifact-e2e.sh`, `scripts/test-ci-feedback-loop.sh`, etc.) against staging.
3. For schema migrations: migration is verified to run cleanly and rollback procedure is confirmed.

### 4.4 Production Deployment

1. Deployments to production occur during business hours unless addressing an active incident.
2. The deploying engineer monitors logs and health endpoints (`/healthz`, CloudWatch alarms) for at least 15 minutes post-deploy.
3. If anomalies are observed, the engineer initiates rollback per Section 5.

---

## 5. Rollback Procedure

### Application (ECS)

Force a new ECS deployment of the previous task definition revision:

```bash
aws ecs update-service \
  --cluster <cluster-name> \
  --service control-plane \
  --task-definition <previous-task-def-arn> \
  --force-new-deployment
```

### Database Migration

Additive migrations (ADD COLUMN, CREATE TABLE) do not require rollback — the prior code version runs against the new schema safely. Destructive migrations must include a documented reverse migration script reviewed before deployment.

### Infrastructure (Terraform)

Revert the offending commit and apply `terraform plan` / `terraform apply` from the reverted state. Emergency rollback of individual resources uses `terraform state` commands with Engineering Lead approval.

---

## 6. Emergency Change Process

When a change must be deployed immediately (active incident or active exploit):

1. Engineering Lead or on-call engineer verbally approves deployment.
2. Minimum smoke test run (`/healthz`, core enrollment + checkin path).
3. Change is deployed.
4. A formal post-deploy review (code review, test coverage, documentation) is completed within 24 hours.
5. The emergency deployment and post-deploy review are documented in the incident record.

---

## 7. Dependency Management

- Third-party Go dependencies are updated via `go get` with a PR following the standard review process.
- `go.sum` is committed and reviewed for unexpected additions.
- Vulnerability scanning (Trivy/Grype) runs on all artifacts post-ingest; critical CVEs in direct dependencies are patched within 14 days per the Vulnerability Management Policy.
- UI dependencies are updated via `npm update` with a PR; `npm audit` must report no high/critical findings before merge.

---

## 8. Secret and Configuration Changes

- Secrets (JWT keys, encryption keys, API tokens) are rotated via AWS Secrets Manager; the new version is deployed as part of a standard or significant change.
- Environment variable changes to production ECS task definitions are applied via Terraform and follow the standard change process.
- `HARDENED_PROFILE=1` is enforced in production; any change that would violate the hardened profile startup checks will fail at boot, providing a natural gate.

---

## 9. Change Records

All production changes are traceable via:
- GitHub PR history (code and infrastructure changes)
- Terraform state history (infrastructure drift)
- Audit log (`auth.user.*`, `artifact.*`, `device.*` events for platform-level changes)
- ECS deployment events in CloudWatch

There is no separate manual change log — the combination of GitHub and audit log is the authoritative record.

---

## 10. Related Documents

- `docs/development/roadmap.md` — feature and phase planning
- `docs/incidents/ir-runbook.md` — emergency change and rollback procedures under incident conditions
- `docs/compliance/policies/access-control-policy.md` — who is authorized to deploy
- `docs/compliance/policies/vulnerability-management-policy.md` — patch timelines driven by scan findings
