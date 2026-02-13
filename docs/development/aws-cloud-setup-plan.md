# AWS Cloud Setup Plan (Demo + Customer Option)

This document defines a practical AWS path that gives HardwareOps:
- A cloud demo environment quickly.
- A production-capable cloud option customers can choose instead of on-prem.

It is intentionally phased so we can deliver value now without blocking on larger infrastructure changes.

## Goals
- Preserve current platform behavior (artifacts, check-ins, mTLS device identity, desired state, audit logs).
- Keep on-prem as a supported option.
- Add an AWS deployment path that is repeatable and supportable.
- Stand up a demo environment that is easy to run for sales and product walkthroughs.

## Current Constraints (from repo behavior)
- Device check-in/apply endpoints require client cert identity (mTLS cert must be available directly or forwarded in a trusted header).
- Object store client currently expects explicit `S3_ACCESS_KEY` and `S3_SECRET_KEY`.
- Backup/upgrade runners are Docker-host oriented (`docker` runner mode); this does not map cleanly to Fargate.
- On-prem compose defaults are optimized for private environments, not internet exposure.

## Deployment Modes
1. `Mode A (Now)`: AWS EC2 single-host demo stack (fastest path, minimal code change).
2. `Mode B (Target)`: AWS managed deployment (ECS Fargate + RDS + S3 + LB + Secrets).

## Mode A: EC2 Demo Stack (Immediate)
Use this first so demos can run quickly while Mode B is implemented.

### Target Shape
- One Ubuntu EC2 instance.
- Existing `deploy/compose/docker-compose.onprem.yml` stack:
  - `gateway` (UI + API reverse proxy)
  - `control-plane`
  - `postgres`
  - `minio`
- Route53 DNS record to the instance.
- Security group allowlist for known demo operator IPs.

### Step-by-Step
1. Create AWS baseline.
   - Region: pick one primary region.
   - EC2 IAM role: CloudWatch agent/log shipping only (no wide admin role).
   - Route53 hosted zone for demo domain.
2. Launch instance.
   - Ubuntu 22.04 LTS, `t3.large` minimum.
   - 100+ GB gp3 EBS.
   - Attach Elastic IP.
   - Security group:
     - `22/tcp` from admin IPs only.
     - `443/tcp` from known demo user IPs (or VPN CIDR).
3. Install runtime and repo.
   - Install Docker: `./scripts/install-docker-ubuntu.sh`
   - Clone repo and `cd` into it.
4. Configure on-prem compose env for cloud host.
   - `cp deploy/compose/.env.onprem.example deploy/compose/.env.onprem`
   - Set:
     - `DOMAIN=<demo-domain>`
     - `PUBLIC_BASE_URL=https://<demo-domain>`
     - Strong values for `POSTGRES_PASSWORD`, `MINIO_ROOT_PASSWORD`, `MAINTENANCE_TOKEN`.
   - For demo-only deployments where licensing is not enforced:
     - `LICENSE_ENFORCE=0`
5. Create cert material for gateway/control-plane.
   - `sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=<demo-domain> ./scripts/setup-control-plane.sh`
   - Note: this creates a private CA chain. Demo browsers must trust the CA cert.
6. Start stack.
   - `docker compose -f deploy/compose/docker-compose.onprem.yml --env-file deploy/compose/.env.onprem up -d --build`
7. Validate health.
   - `curl --cacert /opt/hardwareops/certs/ca.crt https://<demo-domain>/healthz`
   - Open `https://<demo-domain>/` and verify UI loads.
8. Enroll demo agent(s).
   - Set `CONTROL_PLANE_URL=https://<demo-domain>`
   - Use `scripts/agent-enroll.sh` and `docs/agent-systemd.md`.
9. Operational checks before each demo.
   - `docker compose ... ps`
   - Disk free space (`df -h`)
   - Recent logs for `control-plane` and `gateway`
   - Last backup timestamp

### Mode A Risks and Guardrails
- Do not expose this stack broadly to the internet without auth hardening.
- Keep ingress IP-restricted for demos unless auth and WAF are in place.
- Use scheduled EBS snapshots + periodic `scripts/backup-stack.sh`.

## Mode B: Managed AWS Deployment (Customer Cloud Option)
This is the recommended long-term cloud option.

### Target Shape
- ECS Fargate services for application containers.
- RDS PostgreSQL for control-plane data.
- S3 for artifacts.
- AWS Secrets Manager for runtime secrets.
- CloudWatch Logs/metrics and alarms.
- Route53 + ACM + Load Balancer endpoints.

### Proposed Endpoint Model
- `app.<domain>` for UI/operator API.
- `devices.<domain>` for agent traffic (mTLS-required path).

This split allows tighter security controls for devices and avoids mixing browser/operator traffic with strict mTLS policy.

### Phase Plan
1. Phase B0: Architecture decision and spike.
   - Confirm mTLS ingress pattern with a working proof:
     - LB/client-cert forwarding header compatibility with `CLIENT_CERT_HEADER`, or
     - NLB pass-through to in-task TLS proxy preserving current `X-Client-Cert` behavior.
2. Phase B1: Infrastructure as code.
   - Terraform modules for VPC, ECS, RDS, S3, Secrets, IAM, DNS, LB.
   - One stack per environment (`dev`, `demo`, `prod`).
3. Phase B2: Runtime configuration.
   - Move control-plane env and secrets to ECS task definitions + Secrets Manager.
   - Set `TRUST_PROXY=1` when certs are forwarded by an ingress layer.
   - Disable host-bound runners:
     - `UPGRADE_RUNNER_MODE=disabled`
     - `BACKUP_RUNNER_MODE=disabled`
4. Phase B3: AWS-native operations.
   - Backups: RDS snapshots + S3 versioning/lifecycle + optional AWS Backup.
   - Deployments: rolling or blue/green ECS service deploys.
   - Monitoring: CloudWatch alarms + dashboards.
5. Phase B4: Production readiness.
   - Disaster recovery test.
   - Security review (IAM least privilege, SG rules, secret rotation).
   - Runbook validation with an end-to-end agent apply scenario.

## Required Repo Work Items for Mode B
1. Add cloud deployment manifests/IaC folder (`deploy/aws/terraform`).
2. Add env passthrough for auth settings in deployment templates used for cloud.
3. Add object-store credential strategy improvement:
   - Current: static keys.
   - Target: IAM role/default AWS credential chain support.
4. Add cloud-native backup/restore adapters (RDS/S3 aware) for maintenance APIs.
5. Add cloud-native upgrade workflow (replace Docker-in-Docker runner assumptions).

## Recommended Environment Profiles
- `demo`:
  - Small ECS task sizes, single-AZ acceptable, lower retention.
  - Tight access controls for invited demo users.
- `customer-prod`:
  - Multi-AZ RDS, private subnets, stricter alarms, documented RTO/RPO.

## Acceptance Criteria
- A repeatable AWS deployment exists and is documented.
- Agents can enroll and perform mTLS check-ins against cloud endpoint.
- Artifact upload/download and apply flow works end-to-end.
- Recovery drill passes (restore DB/object data and resume operations).
- Demo environment can be stood up and validated within one working day.

## Open Decisions / Questions
1. Should we support one cloud environment per customer account, or a vendor-hosted single-tenant environment per customer?
2. Is the immediate priority:
   - `A)` demo environment fast (Mode A first), or
   - `B)` managed customer cloud first (Mode B first)?
3. For initial cloud launch, is IP allowlisting acceptable, or do you require internet-open access with full auth hardening from day one?
4. Do we want to keep license enforcement in cloud deployments, or scope it to on-prem only?
