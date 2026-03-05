# Security Hardening Pass (Core + AWS)

This document captures the current security posture, gaps found in a focused hardening pass, and the execution order to close them.

Date: 2026-02-16  
Scope reviewed: control-plane security model, auth/RBAC posture, certificate model, AWS Terraform scaffold, AWS deployment runbook.

## Current Security Baseline (Already in Place)
- Artifact signing + agent-side verification (hard-fail on invalid signatures).
- Local auth with bootstrap admin, JWT sessions, voucher onboarding, and audit logs.
- Audit query/export + retention controls.
- Device identity via mTLS and certificate rotation workflows.
- Backup/restore runbooks + upgrade rollback contract.
- AWS split endpoint model (`app.*` and `agent.*`) with ALB listener separation and mTLS-capable device ingress.

## Findings From This Pass

### High priority
1) Secret handling in AWS deployments is not yet strict-by-default.
- Plaintext values can still be provided through tfvars/env for DB URL, JWT secret, and bootstrap credentials.
- `control_plane_secret_arns` and `gateway_secret_arns` exist, but are not yet enforced as the default production path.

2) AWS edge protection is incomplete.
- WAF association is planned but not yet implemented in the Terraform stack.
- Ingress policy is still broad by default (`0.0.0.0/0`) and app/device CIDR controls are not split.

3) IAM permissions still need least-privilege tightening.
- ECS task role currently uses broad managed policies that should be replaced with scoped S3/CloudWatch/Secrets/KMS permissions.

### Medium priority
4) ECS Exec defaults are too permissive for production.
- Current default enables exec access for tasks; production should require explicit opt-in with audit controls.

5) Cloud security operations controls are partial.
- Core metrics exist, but CloudWatch/SNS security-oriented alarms and response wiring are not yet complete.

6) Role-aware UI enforcement is partial.
- API role checks are present; some UI surfaces still need strict action gating by role.

## Hardening Plan (Execution Order)

1) Secrets Manager first-class production path
- Require secret ARNs for `DATABASE_URL`, `AUTH_JWT_SECRET`, `MAINTENANCE_TOKEN`, and bootstrap credentials in `prod`.
- Keep plaintext only for local/dev convenience.
- Acceptance: no sensitive values in ECS task definition environment blocks for production.

2) IAM least-privilege role set
- Replace broad managed policies with scoped inline policies:
  - S3 artifact bucket (exact bucket ARN + prefix),
  - Secrets Manager read for required secret ARNs only,
  - KMS decrypt for required keys only,
  - CloudWatch log write only for service log groups.
- Acceptance: IAM policy review passes with no wildcard resource grants except where technically required.

3) WAF + ingress policy split
- Add WAF web ACL and attach to app listener/ALB.
- Split app/device ingress CIDRs so operator access policy can differ from device ingress policy.
- Acceptance: Terraform-managed WAF with baseline managed rules and explicit allow/deny behavior.

4) ECS Exec and break-glass guardrails
- Default `enable_execute_command=false` for production stacks.
- Add explicit break-glass runbook + audit event requirements when exec is enabled temporarily.
- Acceptance: production deployment is non-exec by default and exceptions are auditable.

5) Security alarms and response hooks
- Add alarms for ALB 4xx/5xx anomalies, ECS task churn, auth failures, and mTLS verification failure spikes.
- Route to SNS (and PagerDuty/Slack integration point).
- Acceptance: documented alarm thresholds and tested notification path.

6) Complete role-aware UI enforcement
- Enforce role-based hiding/disable across all mutating actions.
- Backend route coverage now includes explicit viewer/operator/admin matrix tests and `artifact.publish` service-token checks; remaining work is UI parity.
- Acceptance: viewer role cannot trigger writes from UI; operator/admin split is consistent.

## Production Exit Criteria (Security)
- No plaintext secrets in production task definitions or committed config.
- WAF enabled on app ingress.
- IAM policies least-privilege and reviewed.
- ECS exec disabled by default in production.
- Security alarms active with tested paging path.
- RBAC enforced consistently in API and UI.
