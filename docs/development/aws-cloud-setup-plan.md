# AWS Cloud Setup Plan (Production-Grade Customer Deployment)

This document covers supported deployment shapes and the production-grade AWS path for customer deployments, while preserving the existing on-prem product model.

Execution runbook: `aws-customer-deployment-runbook.md`.
Security hardening tracker: `security-hardening.md`.

---

## Deployment Shapes

### On-Prem (customer-hosted)

**Use when:** customer requires local control, air-gapped, or low-latency environment.

Shape:
- Control-plane + UI on a single server
- Postgres (local or customer-managed)
- MinIO/S3-compatible object store (local)
- Optional CoreDNS for internal DNS

**Why this works:** fits the installer bundle model; easy to secure with TLS + mTLS; minimal external dependencies.

Operational needs: Postgres + MinIO backups, CA rotation plan, host monitoring.

### AWS ECS Fargate (cloud target)

**Use when:** customer wants managed infrastructure and internet-reachable control-plane.

Shape: ECS Fargate (control-plane + gateway) + RDS Postgres + S3 + ALB + ACM + CloudWatch.

**Why ECS Fargate:** lowest ops for a small team, fast iteration, no Kubernetes overhead.

### Alternative cloud shapes

- **EKS:** best for large customers with existing Kubernetes ops.
- **EC2 + Docker Compose:** simplest ops, least scalable; suitable for POCs and sales demos.

### DNS + TLS (both modes)

Use a customer-owned domain and split hostnames:
- Human/UI host: `hardwareops.internal`
- Agent/API host: `agent.hardwareops.internal`

Prefer a browser-trusted server cert on the human host. Keep agent traffic on mTLS device certs. For private CA onboarding, use the bootstrap token flow (`/api/v1/bootstrap/ca`) to fetch the CA cert before full login.

---

## Scope and Goals
- Preserve current product behavior: artifacts, desired state, agent mTLS identity, audit logs, auth, and backup/restore workflows.
- Provide a repeatable AWS deployment pattern for customer-facing production environments.
- Keep a fast demo path, but prioritize production controls and operability.
- Minimize cloud-specific code forks by introducing explicit runtime adapters where needed.

## Ground Rules
- Single-tenant deployment per customer environment (default).
- Environment tiers: `dev`, `staging`, `prod`.
- Human/UI traffic and agent/device traffic are split at DNS and ingress policy.
- No plaintext credentials in task definitions.

---

## Architecture Baseline (Target State)

### 1) Account and Environment Strategy
- Use multi-account landing zone (at least separate prod/non-prod accounts).
- Keep shared security/logging controls outside workload account(s).
- Treat each customer production environment as an isolated workload boundary.

### 2) Networking
- VPC across 3 Availability Zones.
- Public subnets: ALB only.
- Private subnets: ECS services and RDS.
- No direct internet ingress to ECS tasks or database.
- Use VPC endpoints where possible for private service access.

### 3) Ingress and Endpoint Split
- `app.<domain>`: UI + operator APIs.
- `devices.<domain>`: agent endpoints only.
- ALB host-based listener rules enforce this split.
- Apply AWS WAF web ACL on ALB for `app.<domain>`.

### 4) TLS and mTLS
- Human endpoint uses ACM certificate(s) on ALB.
- Device endpoint requires mTLS.
- Preferred cloud pattern:
  - ALB mutual TLS in verify mode, then forward certificate identity in headers to control-plane.
- Application must accept ALB mTLS headers (`X-Amzn-Mtls-*`) as trusted identity source when `TRUST_PROXY=1`.

### 5) Compute
- ECS Fargate services:
  - `control-plane` service.
  - `gateway` service (or move static UI to S3/CloudFront later).
- Minimum 2 tasks per service in production, spread across AZs.
- Use deployment protection:
  - deployment circuit breaker with rollback for rolling deploys.
  - optionally blue/green for higher-risk upgrades.

### 6) Data Plane
- Database: Amazon RDS PostgreSQL.
  - Prefer Multi-AZ in production.
  - Evaluate Multi-AZ DB cluster vs Multi-AZ DB instance by engine/version and feature constraints.
- Artifact store: Amazon S3 bucket.
  - Versioning enabled.
  - Lifecycle policy for retention/cost.
  - Explicit encryption policy (SSE-KMS with customer managed key for stricter controls).

### 7) Secrets and Identity
- Secrets Manager for app secrets and DB credentials.
- ECS task role + execution role separation (least privilege).
- Avoid static long-lived S3 keys in env:
  - use IAM task role and AWS credential chain.
- Rotate database credentials via managed rotation or Lambda-based rotation schedule.

### 8) Observability and Ops
- CloudWatch Logs for all services.
- CloudWatch metrics and alarms:
  - ALB 5xx, target health, latency.
  - ECS task restart/crash loops, deployment failures.
  - RDS CPU/storage/connections/replication lag.
- Add synthetic checks for `/healthz` and login path.

### 9) Backup, Restore, and DR
- RDS automated backups with PITR.
- Additional scheduled snapshots for retention policy.
- S3 versioning + (optional) cross-region replication for DR objectives.
- Restore drill cadence and evidence capture.

---

## Deployment Modes

## Mode A (Short-Term): EC2 Demo Stack
- Existing compose stack on a hardened EC2 host.
- Useful for sales/demo and smoke validation.
- Not the target production model.

## Mode B (Target): Managed Customer Cloud
- ECS Fargate + RDS + S3 + ALB + WAF + ACM + Secrets Manager.
- This is the production model to standardize for customers.
- Optional demo overlay in `dev`: 3 ECS demo agents with EFS-backed persistence for customer demos and manual artifact switching.

---

## Production Readiness Controls (Must-Have)

### Security
- IAM least-privilege task roles and scoped secrets access.
- WAF on ALB and security group minimization.
- Private subnets for ECS/RDS, no direct database ingress from internet.
- TLS 1.2+ policies on ALB listeners.
- mTLS verify on `devices.<domain>`.

### Reliability
- Multi-AZ service distribution.
- RDS Multi-AZ.
- Health checks tuned for faster but safe deployments.
- Automatic rollback policy on failed deployments.

### Operations
- Runbooks for deploy, rollback, cert rotation, backup restore, incident response.
- Alarm routing (PagerDuty/Slack/SNS).
- Change windows and maintenance workflow.

### Compliance/Audit
- Audit log retention policy documented and enforced.
- Access logging and change history retained.
- Secret rotation policy documented and auditable.

---

## Repo Work Required (Cloud Blockers)

1. Ingress identity adapter
- Add support for ALB mTLS headers (`X-Amzn-Mtls-Clientcert-Leaf` or verify-mode fields).
- Preserve existing on-prem `X-Client-Cert` behavior behind trusted proxy toggle.

2. AWS credentials chain for object store
- Replace mandatory `S3_ACCESS_KEY`/`S3_SECRET_KEY` assumption.
- Support IAM task role credentials by default.

3. Cloud-native maintenance adapters
- Backup/restore runners currently assume host Docker access.
- Introduce AWS-mode handlers for:
  - backup orchestration (RDS/S3 aware),
  - restore orchestration (documented controls),
  - upgrade workflow without Docker-in-Docker assumptions.

4. IaC baseline
- Add `deploy/aws/terraform` modules:
  - VPC, ALB, ECS services, RDS, S3, KMS, Secrets, IAM, Route53, WAF, alarms.

5. Deployment pipeline
- Build/push images to ECR.
- Promote immutable images across environments.
- Apply infrastructure and service deploy with approvals.

## Current Scaffold in Repo
- Terraform scaffold now lives under `deploy/aws/terraform`.
- It includes:
  - reusable modules (`network`, `security`, `artifact_store`, `database`, `alb`, `ecs`, `customer_stack`)
  - environment entry points in `deploy/aws/terraform/envs/dev`, `deploy/aws/terraform/envs/staging`, and `deploy/aws/terraform/envs/prod`
- This scaffold is intentionally opinionated for:
  - vendor-hosted, per-customer stacks
  - ALB mTLS for device ingress
  - ECS deployment circuit-breaker rollback

---

## Implementation Sequence (Recommended)

### Phase 0: Cloud Foundation (1-2 weeks)
- Landing zone/account model and environment naming.
- Terraform skeleton and state strategy.
- DNS and certificate strategy finalized.

### Phase 1: Runtime Parity in AWS (2-3 weeks)
- ECS + ALB + RDS + S3 up in `dev`.
- Basic auth/login and artifact flows work.
- mTLS ingress proof on `devices.<domain>`.

### Phase 2: Security and Operability (2-3 weeks)
- Secrets Manager integration.
- IAM least privilege pass.
- CloudWatch dashboards/alarms + runbooks.
- WAF and ingress hardening.

### Phase 3: Production Controls (2-3 weeks)
- Backup/PITR restore test in staging.
- Deployment rollback validation.
- Load and failure tests.
- Go-live checklist and sign-off.

---

## Customer Go-Live Checklist (Condensed)
- DNS delegated and certificates issued.
- Auth bootstrap and admin controls validated.
- Device enrollment + check-in + apply successful in staging.
- Backup + restore drill completed and timed.
- Alerting and on-call routing active.
- Security review complete (IAM, SGs, WAF, secret rotation).
- Rollback procedure executed at least once in staging.

---

## Open Decisions (Need to Lock)
1. mTLS ingress implementation:
- ALB verify headers at app layer, or NLB passthrough + sidecar proxy termination.

2. Customer hosting model:
- Customer AWS account (preferred for isolation), or vendor-hosted per-customer tenancy.

3. Database HA mode:
- RDS Multi-AZ instance vs Multi-AZ DB cluster based on required engine features and constraints.

4. Upgrade model in cloud:
- rolling with circuit breaker only, or blue/green for all production releases.

---

## AWS Source Notes (Primary References)
- ALB mTLS modes and headers:
  - https://docs.aws.amazon.com/elasticloadbalancing/latest/application/mutual-authentication.html
  - https://docs.aws.amazon.com/elasticloadbalancing/latest/application/configuring-mtls-with-elb.html
- ALB HTTPS listener/certificates:
  - https://docs.aws.amazon.com/elasticloadbalancing/latest/application/create-https-listener.html
  - https://docs.aws.amazon.com/elasticloadbalancing/latest/application/https-listener-certificates.html
- ECS IAM roles and autoscaling:
  - https://docs.aws.amazon.com/AmazonECS/latest/developerguide/security-iam-roles.html
  - https://docs.aws.amazon.com/AmazonECS/latest/developerguide/service-auto-scaling.html
  - https://docs.aws.amazon.com/AmazonECS/latest/developerguide/service-autoscaling-targettracking.html
- ECS deployment safety:
  - https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ecs-service-deploymentcircuitbreaker.html
  - https://docs.aws.amazon.com/AmazonECS/latest/developerguide/deployment-type-bluegreen.html
- ECS secrets handling:
  - https://docs.aws.amazon.com/AmazonECS/latest/developerguide/specifying-sensitive-data.html
  - https://docs.aws.amazon.com/AmazonECS/latest/userguide/secrets-envvar-secrets-manager.html
- RDS HA and backup/PITR:
  - https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/create-multi-az-db-cluster.html
  - https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html
  - https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_WorkingWithAutomatedBackups.BackupRetention.html
  - https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_PIT.html
- S3 encryption defaults:
  - https://docs.aws.amazon.com/AmazonS3/latest/userguide/default-bucket-encryption.html
- WAF association:
  - https://docs.aws.amazon.com/waf/latest/developerguide/web-acl-associating-aws-resource.html
- Multi-account landing zone:
  - https://docs.aws.amazon.com/controltower/latest/userguide/aws-multi-account-landing-zone.html
