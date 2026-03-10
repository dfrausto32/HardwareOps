# HardwareOps AWS Terraform Scaffold

This directory is a production-oriented Terraform scaffold for **vendor-hosted, per-customer** AWS deployments.

Canonical deployment guide: `../../../docs/deploy.md`  
Canonical operations guide: `../../../docs/operations.md`

Operational runbook: `../../../docs/development/aws-customer-deployment-runbook.md`.
CLI helper: `../../../scripts/aws-customer.sh`.
Acceptance helper: `../../../scripts/aws-hardening-check.sh`.

## What this scaffold includes
- `modules/network`: VPC, public/private subnets, IGW, NAT, route tables.
- `modules/security`: ALB, ECS, and RDS security groups.
- `modules/artifact_store`: S3 artifact bucket with versioning and encryption.
- `modules/database`: RDS PostgreSQL (Multi-AZ enabled by default).
- `modules/alb`: ALB with:
  - app listener on `443`
  - device listener on `8443` with **ALB mTLS**
  - host-based routing
  - AWS WAF managed baseline scoped to `app_host`
- `modules/ecs`: ECS Fargate cluster/services for `control-plane` and `gateway`.
- Optional demo fleet: one ECS service per demo agent slot backed by EFS persistence.
- `modules/customer_stack`: composition module for a single customer environment.
- `envs/{dev,staging,prod}`: per-environment entry points and examples.

## Current defaults aligned to your decisions
- Hosting model: vendor-hosted, one stack per customer environment.
- Device ingress: ALB mutual TLS (`verify` mode).
- Database HA: defaults to Multi-AZ RDS.
- Deploy safety: ECS deployment circuit breaker with rollback enabled.
- App ingress: WAF enabled by default with AWS managed baseline rules.

## Quick start
1. Choose an environment directory (example: `envs/dev`).
2. Copy examples:
   - `cp terraform.tfvars.example terraform.tfvars`
   - `cp backend.hcl.example backend.hcl`
3. Fill required values:
   - DNS (`app_host`, `devices_host`, `route53_zone_id`)  
     (`devices_host` should currently use an `agent.` prefix because of gateway host routing)
   - ACM certificate ARN
   - trust store object location (`device_mtls_bucket`, `device_mtls_key`)
   - choose device mTLS mode (`device_mtls_mode`)
   - optional ingress controls (`ingress_cidrs`, `app_ingress_cidrs`, `device_ingress_cidrs`)
- image URIs for gateway/control-plane
- optional demo fleet image URI (`demo_agent_image`) when `enable_demo_fleet = true`
- optional pull credential secret (`artifact_pull_credentials_aws_secret_id`) for adapter `credentialRef`
- optional customer-managed secret KMS keys (`secret_kms_key_arns`)
- secrets map for sensitive env values (for example `DATABASE_URL`)
4. Run:
   - `terraform init -backend-config=backend.hcl`
   - `terraform plan`
   - `terraform apply`

Before production apply, run:
- `./scripts/aws-hardening-check.sh config --customer <customer> --env prod`

After apply, run:
- `./scripts/aws-hardening-check.sh deployment --customer <customer> --env prod --region <aws-region>`

## Important notes
- This is scaffolding, not final hardened production IaC.
- The app listener is `443`; the devices listener is `8443` to keep human/UI traffic separated from mTLS device traffic on ALB.
- If you need both endpoints on port `443`, use separate ALBs or front-door routing pattern in a later iteration.
- Secret values should come from Secrets Manager (`*_secret_arns` maps), not plaintext tfvars.
- Proxy header trust is locked to trusted proxy CIDRs; by default this is set to the ECS private subnet CIDRs via `TRUST_PROXY_CIDRS`.
- `ingress_cidrs` remains the shared fallback for both listeners; set `app_ingress_cidrs` and `device_ingress_cidrs` only when you need different policies.
- WAF is associated to the shared ALB, but its managed rules and optional rate limit are scope-down matched to `app_host` so device mTLS traffic is not filtered by app rules.
- `artifact_pull_credentials_aws_secret_id` can be set to a secret **name or ARN**, but ARN is recommended for least-privilege IAM policy generation.
- Set `secret_kms_key_arns` only when referenced Secrets Manager secrets use customer-managed KMS keys; AWS-managed Secrets Manager keys do not need extra input here.
- For local auth mode, ensure `AUTH_JWT_SECRET` and bootstrap credentials are set in `control_plane_env`.
- For token-based first-time enrollment, start with `device_mtls_mode = "passthrough"`, enroll devices, then switch to `device_mtls_mode = "verify"` and re-apply.
- Demo fleet is designed to persist device identity/state across normal ECS restarts and rolling updates via EFS; full environment destroy still deletes demo state.
- Set `database_url_secret_arn` to inject `DATABASE_URL` via ECS `valueFrom` (Secrets Manager) instead of plaintext. When not set, the plaintext path remains active for backward compatibility. See "Database URL Secret" section below.
- CloudWatch alarms (ALB, ECS, RDS) are created automatically. Set `alarm_sns_email` to receive email notifications. See "CloudWatch Alarms" section below.

## Cloud pull-adapter smoke test (Artifactory + Secrets Manager)
1. Create/update a Secrets Manager secret with credential JSON:
   ```json
   {
     "artifactory-demo": {
       "username": "admin",
       "password": "password"
     }
   }
   ```
2. Set `artifact_pull_credentials_aws_secret_id` in your environment `terraform.tfvars` and apply.
3. Run pull ingest against cloud control-plane:
   ```bash
   BASE_URL=https://app.<customer-domain> \
   INSECURE=1 \
   RUN_SETUP=0 \
   SETUP_OUTPUT_DIR=/tmp/hardwareops-artifactory-demo \
   ./scripts/test-artifactory-adapter.sh
   ```
4. Expected: `/api/v1/artifacts/pull` returns `200`, artifact appears in UI, and audit includes `artifact.pull` with `sourceKind=artifactory`.

## Add AWS Secrets Manager for pull credentials (step-by-step)
Use this when you want `source.credentialRef` to resolve in cloud deployments.

1. Create or update the secret:
   ```bash
   AWS_PROFILE=hwops-admin
   AWS_REGION=us-east-1
   CUSTOMER=parcel
   ENV=dev
   SECRET_NAME="hardwareops/${CUSTOMER}/${ENV}/artifact-pull-credentials"

   aws --profile "$AWS_PROFILE" --region "$AWS_REGION" secretsmanager create-secret \
     --name "$SECRET_NAME" \
     --secret-string file:///tmp/hardwareops-artifactory-demo/pull-credentials.json \
   || aws --profile "$AWS_PROFILE" --region "$AWS_REGION" secretsmanager put-secret-value \
     --secret-id "$SECRET_NAME" \
     --secret-string file:///tmp/hardwareops-artifactory-demo/pull-credentials.json
   ```

2. Get the secret ARN (recommended form for Terraform input):
   ```bash
   SECRET_ARN=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" secretsmanager describe-secret \
     --secret-id "$SECRET_NAME" --query ARN --output text)
   echo "$SECRET_ARN"
   ```

3. Set it in your customer tfvars:
   ```hcl
   artifact_pull_credentials_aws_secret_id = "<SECRET_ARN>"
   ```

4. Apply Terraform:
   ```bash
   ./scripts/aws-customer.sh apply \
     --customer "$CUSTOMER" \
     --env "$ENV" \
     --region "$AWS_REGION" \
     --profile "$AWS_PROFILE" \
     --auto-approve
   ```

5. Validate with adapter test:
   ```bash
   BASE_URL=https://app.<customer-domain> \
   INSECURE=1 \
   RUN_SETUP=0 \
   SETUP_OUTPUT_DIR=/tmp/hardwareops-artifactory-demo \
   AUTH_EMAIL=<bootstrap-admin-email> \
   AUTH_PASSWORD=<bootstrap-admin-password> \
   ./scripts/test-artifactory-adapter.sh
   ```

Credential JSON format must be a ref map, for example:
```json
{
  "artifactory-demo": {
    "username": "admin",
    "password": "password"
  }
}
```

## Database URL Secret (Secrets Manager)

By default `DATABASE_URL` is injected as plaintext in ECS task environment variables. To eliminate the plaintext credential:

1. Create a Secrets Manager secret with the full connection string as its value:
   ```bash
   aws secretsmanager create-secret \
     --name "hardwareops/customer-a/prod/database-url" \
     --secret-string "postgres://hardwareops:<password>@<rds-endpoint>:5432/hardwareops?sslmode=require"
   ```

2. Get the ARN:
   ```bash
   aws secretsmanager describe-secret \
     --secret-id "hardwareops/customer-a/prod/database-url" \
     --query ARN --output text
   ```

3. Set in your environment `terraform.tfvars`:
   ```hcl
   database_url_secret_arn = "arn:aws:secretsmanager:us-east-1:111122223333:secret:hardwareops/customer-a/prod/database-url-AbCdEf"
   ```

4. Apply Terraform — `DATABASE_URL` will now be in the ECS task definition `secrets` block (injected via `valueFrom`) rather than the plaintext `environment` block.

When `db_manage_master_user_password = true`, use the `database_master_secret_arn` Terraform output to locate the RDS-managed secret. However, that secret stores JSON (username/password fields), not a connection URL — you still need to create a separate URL secret as shown above.

## CloudWatch Alarms

An SNS topic (`${name_prefix}-alerts`) is created automatically. Set `alarm_sns_email` to subscribe an email address:

```hcl
alarm_sns_email = "ops-team@example.com"
```

Alarms created:

| Alarm | Metric | Default threshold |
|---|---|---|
| ALB 5xx errors | `HTTPCode_ELB_5XX_Count` | ≥ 10 per 5 min |
| ALB unhealthy hosts | `UnHealthyHostCount` | ≥ 1 over 2 periods |
| ECS CP CPU | `CPUUtilization` | ≥ 80% over 3 periods |
| ECS CP Memory | `MemoryUtilization` | ≥ 80% over 3 periods |
| ECS CP Tasks | `RunningTaskCount` | < 1 (service down) |
| RDS CPU | `CPUUtilization` | ≥ 80% over 3 periods |
| RDS Free Storage | `FreeStorageSpace` | ≤ 5 GB |
| RDS Connections | `DatabaseConnections` | ≥ 100 over 2 periods |

All thresholds are configurable via `alarm_*` variables.

**Note:** The ECS `RunningTaskCount` alarm uses the `ECS/ContainerInsights` namespace. Container Insights must be enabled on the cluster for this metric to populate. The other ECS alarms use the standard `AWS/ECS` namespace and work without Container Insights.

The `alerts_sns_topic_arn` output exposes the topic ARN for additional subscriptions (PagerDuty, Lambda, etc.).

## Next implementation pass recommended
1. Add autoscaling policies for ECS services.
2. Add Route53 health checks and failover strategy (optional).
