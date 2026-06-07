# Playbook: Customer Self-Hosts in Their Own AWS Environment

## When to Use

Use this playbook when the **customer owns and operates their own AWS account** and you are delivering Parcel as software they deploy themselves.

Choose this path when:
- The customer has AWS expertise in-house and wants full infrastructure ownership.
- The customer's compliance or data-sovereignty requirements prohibit third-party-managed infrastructure.
- The customer is deploying into an existing VPC or account structure they control.
- The customer has purchased a self-hosted license rather than a managed engagement.

Do **not** use this playbook when Parcel is managing the infrastructure for the customer — use `managed-aws-hosted.md` instead.

---

## Engineering Hours

Terraform is the preferred IaC for deploying the Parcel `customer_stack` module. However, the customer is free to use any tooling they choose. If the customer wants hands-on assistance with CDK, CloudFormation, manual console deployment, or Ansible-based workflows, they can purchase engineering hours and you will assist in whichever approach they prefer.

All IaC examples in this playbook use Terraform. Translate as needed for the customer's chosen tooling.

---

## Responsibility Split

| Area | Parcel | Customer |
|---|---|---|
| AWS infrastructure provisioning | Advisory / engineering hours | Owner |
| Terraform state and apply | Advisory | Owner |
| TLS certificates and DNS | Advisory | Owner |
| RDS backup policy | Advisory | Owner |
| IAM roles and policies | Advisory | Owner |
| Platform upgrades | Provides release packages | Owner (applies) |
| Incident response | Advisory | Owner |
| User and admin management | — | Owner |
| Artifact and desired-state management | — | Owner |
| CI integration | Advisory | Owner |

---

## Phase 1 — Prerequisites

The customer must have the following before starting.

### AWS account and access
- An AWS account with permissions to create: VPC, ECS, RDS, ALB, S3, IAM, Route53, ACM, Secrets Manager, CloudWatch, WAF.
- An IAM user or role with the above permissions available locally (`aws configure` or profile).

### DNS and TLS
- A domain or subdomain they control (e.g. `parcel.internal.customer.com`).
- An ACM certificate covering both the app host and the devices host (or a wildcard):
  ```bash
  aws acm request-certificate \
    --domain-name "parcel.customer.com" \
    --subject-alternative-names "agent.customer.com" \
    --validation-method DNS
  ```
  Validate the certificate before proceeding. Certificate must be in the same region as the deployment.
- A Route53 hosted zone for the domain (if using Route53 for DNS). If the customer manages DNS externally, set `create_dns_records = false` in `terraform.tfvars` and configure their DNS manually after ALB provisioning.

### Container images
- The Parcel container images (`control-plane` and `gateway`) must be pushed to an ECR repository in the customer's account, or pulled from a registry they control.
  ```bash
  aws ecr create-repository --repository-name parcel-control-plane
  aws ecr create-repository --repository-name parcel-gateway

  aws ecr get-login-password --region us-east-1 | docker login \
    --username AWS \
    --password-stdin ACCOUNT.dkr.ecr.us-east-1.amazonaws.com

  docker tag parcel-control-plane:VERSION \
    ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-control-plane:VERSION
  docker push ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-control-plane:VERSION
  ```

### Terraform (if using Parcel Terraform modules)
- Terraform >= 1.5 installed.
- S3 bucket for Terraform state (customer creates this):
  ```bash
  aws s3api create-bucket \
    --bucket customer-hwops-tf-state \
    --region us-east-1

  aws s3api put-bucket-versioning \
    --bucket customer-hwops-tf-state \
    --versioning-configuration Status=Enabled

  aws dynamodb create-table \
    --table-name customer-hwops-tf-locks \
    --attribute-definitions AttributeName=LockID,AttributeType=S \
    --key-schema AttributeName=LockID,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST
  ```

---

## Phase 2 — Network Architecture Decision

This is the most common failure point for self-hosted deployments. Decide the connectivity model before touching any Terraform.

### Option A — Internet-facing (default)

The ALB is public. App UI (port 443) and device endpoint (port 8443) are reachable from the internet.

- Use this when: operators access the UI from anywhere, and devices may be geographically distributed.
- Harden immediately: set `app_ingress_cidrs` to your operator IP range; do not leave port 443 open to `0.0.0.0/0`.
- WAF is strongly recommended (`enable_waf = true`).

```hcl
ingress_cidrs        = ["0.0.0.0/0"]          # devices listener fallback
app_ingress_cidrs    = ["203.0.113.0/24"]     # restrict to operator IPs
device_ingress_cidrs = ["0.0.0.0/0"]          # devices may come from anywhere
enable_waf           = true
```

### Option B — VPN / private only

The ALB has no public IP. Operators and devices reach it through a VPN, AWS Direct Connect, or VPC peering.

- Use this when: all operators and devices are behind a corporate network or VPN.
- Set `ingress_cidrs` to the VPN or Direct Connect CIDR ranges.
- The ALB can remain in private subnets; set `internal = true` in the ALB module if customizing.

```hcl
app_ingress_cidrs    = ["10.0.0.0/8"]     # corporate VPN range
device_ingress_cidrs = ["10.0.0.0/8"]
enable_waf           = false              # WAF is optional for private-only
```

### Option C — Air-gapped / offline

Devices operate without internet access. The control plane may also be offline.

- Use this when: devices are in isolated facilities with no outbound connectivity.
- The control plane is deployed in a private VPC with no internet gateway.
- Agents must be pre-enrolled before going offline or enrolled through an in-network path.
- S3 must be accessed via VPC endpoint (add `aws_vpc_endpoint` for S3 to the customer's VPC).
- Secrets Manager must also be accessed via VPC endpoint if ECS tasks need it at runtime.

```hcl
# Add to VPC configuration
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = module.network.vpc_id
  service_name      = "com.amazonaws.us-east-1.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [module.network.private_route_table_id]
}

resource "aws_vpc_endpoint" "secretsmanager" {
  vpc_id              = module.network.vpc_id
  service_name        = "com.amazonaws.us-east-1.secretsmanager"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = module.network.private_subnet_ids
  security_group_ids  = [module.security.ecs_security_group_id]
  private_dns_enabled = true
}
```

---

## Phase 3 — Secrets Manager Setup

All sensitive runtime values must be stored in Secrets Manager before Terraform apply. Do not pass credentials as inline `terraform.tfvars` values for production.

### 3.1 JWT secret

```bash
JWT_SECRET=$(openssl rand -base64 48)
aws secretsmanager create-secret \
  --name "parcel/prod/jwt-secret" \
  --secret-string "$JWT_SECRET"
```

### 3.2 Bootstrap admin password

```bash
BOOTSTRAP_PASSWORD=$(openssl rand -base64 24)
aws secretsmanager create-secret \
  --name "parcel/prod/bootstrap-password" \
  --secret-string "$BOOTSTRAP_PASSWORD"
```

### 3.3 DATABASE_URL (created after RDS apply or pre-declared)

```bash
aws secretsmanager create-secret \
  --name "parcel/prod/database-url" \
  --secret-string "postgres://parcel:PASSWORD@RDS-ENDPOINT:5432/parcel?sslmode=require"
```

Reference all three in `terraform.tfvars`:
```hcl
control_plane_secret_arns = {
  AUTH_JWT_SECRET         = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/prod/jwt-secret-SUFFIX"
  AUTH_BOOTSTRAP_PASSWORD = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/prod/bootstrap-password-SUFFIX"
}
database_url_secret_arn = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/prod/database-url-SUFFIX"
```

### 3.4 Trusted signing keys (if using `require_verified`)

```bash
# Generate keys if the customer doesn't already have an Ed25519 keypair
openssl genpkey -algorithm ed25519 -out ed25519.key
openssl pkey -in ed25519.key -pubout -out ed25519.pub

./scripts/build-trusted-signing-keys.sh /tmp/trusted-signing-keys.json ./ed25519.pub

aws secretsmanager create-secret \
  --name "parcel/prod/trusted-signing-keys" \
  --secret-string file:///tmp/trusted-signing-keys.json
```

Reference in `terraform.tfvars`:
```hcl
trusted_signing_keys_aws_secret_id = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/prod/trusted-signing-keys-SUFFIX"
```

---

## Phase 4 — Terraform Configuration

### 4.1 Directory structure

The customer can copy one of the environment templates from this repo:

```
deploy/aws/terraform/envs/prod/
  main.tf
  variables.tf
  outputs.tf
  versions.tf
  terraform.tfvars       # customer fills this in
  backend.hcl            # customer fills this in
```

### 4.2 Backend configuration (`backend.hcl`)

```hcl
bucket         = "customer-hwops-tf-state"
key            = "parcel/prod/terraform.tfstate"
region         = "us-east-1"
dynamodb_table = "customer-hwops-tf-locks"
encrypt        = true
```

### 4.3 Key `terraform.tfvars` values

```hcl
name_prefix = "parcel-prod"

vpc_cidr = "10.0.0.0/16"
public_subnet_cidrs = {
  "us-east-1a" = "10.0.1.0/24"
  "us-east-1b" = "10.0.2.0/24"
}
private_subnet_cidrs = {
  "us-east-1a" = "10.0.11.0/24"
  "us-east-1b" = "10.0.12.0/24"
}

app_host     = "parcel.customer.com"
devices_host = "agent.customer.com"
acm_certificate_arn = "arn:aws:acm:us-east-1:ACCOUNT:certificate/CERT-ID"
route53_zone_id     = "Z1234567890"

control_plane_image = "ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-control-plane:VERSION"
gateway_image       = "ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-gateway:VERSION"

device_mtls_bucket = "customer-hwops-security-assets"
device_mtls_key    = "mtls/device-ca-bundle.pem"
device_mtls_mode   = "passthrough"   # switch to "verify" after fleet enrollment

enable_waf = true

# See Phase 3 for all secret ARN values
database_url_secret_arn = "..."
control_plane_secret_arns = { ... }
```

### 4.4 Initialize and apply

```bash
cd deploy/aws/terraform/envs/prod

terraform init -backend-config=backend.hcl

terraform plan -out=tfplan | tee /tmp/plan.txt

terraform apply tfplan | tee /tmp/apply.txt
```

---

## Phase 5 — ALB and Networking Validation (Most Common Failure Point)

ALB routing issues are the leading cause of failed self-hosted deployments. Work through these in order before moving on.

### 5.1 Confirm ALB health checks pass

```bash
ALB_ARN=$(terraform output -raw alb_arn)

aws elbv2 describe-target-health \
  --target-group-arn $(terraform output -raw app_target_group_arn) \
  --query 'TargetHealthDescriptions[*].{target:Target.Id,port:Target.Port,state:TargetHealth.State,reason:TargetHealth.Reason}' \
  --output table
```

Expected: all targets show `healthy`. Common failures:

| State | Reason | Fix |
|---|---|---|
| `unhealthy` | `Target.FailedHealthChecks` | ECS task is not running or crashing. Check ECS logs. |
| `unhealthy` | `Elb.InternalError` | Security group on ECS tasks blocks ALB. Check `ecs_security_group_id` allows traffic from `alb_security_group_id`. |
| `unused` | `Target.DeregistrationInProgress` | Wait 30 seconds; if persists, task is cycling. |
| `initial` | `Elb.RegistrationInProgress` | Normal on first deploy; wait up to 3 minutes. |

### 5.2 Verify ECS tasks are running

```bash
CLUSTER=$(terraform output -raw ecs_cluster_name)
CP_SERVICE=$(terraform output -raw ecs_control_plane_service_name)
GW_SERVICE=$(terraform output -raw ecs_gateway_service_name)

aws ecs describe-services \
  --cluster "$CLUSTER" \
  --services "$CP_SERVICE" "$GW_SERVICE" \
  --query 'services[*].{name:serviceName,desired:desiredCount,running:runningCount,pending:pendingCount,status:status}' \
  --output table
```

If `runningCount` is 0, check ECS task failure logs:
```bash
aws ecs list-tasks --cluster "$CLUSTER" --service-name "$CP_SERVICE" --desired-status STOPPED \
  --query 'taskArns' --output text \
| xargs -I{} aws ecs describe-tasks --cluster "$CLUSTER" --tasks {} \
  --query 'tasks[*].stoppedReason'
```

### 5.3 Check security group rules

The most common network misconfiguration: the ECS security group does not allow inbound traffic from the ALB security group.

```bash
# Get both security group IDs
ALB_SG=$(terraform output -raw alb_security_group_id 2>/dev/null || \
  aws elbv2 describe-load-balancers \
    --query "LoadBalancers[?contains(LoadBalancerName,'parcel')].SecurityGroups[0]" \
    --output text)

ECS_SG=$(terraform output -raw ecs_security_group_id 2>/dev/null)

# Verify ECS SG allows inbound from ALB SG
aws ec2 describe-security-groups \
  --group-ids "$ECS_SG" \
  --query 'SecurityGroups[0].IpPermissions[*].{port:ToPort,source:UserIdGroupPairs[*].GroupId}' \
  --output json
```

Expected: the ECS security group has an inbound rule allowing traffic from the ALB SG on the container ports (default: 8080 for control-plane, 8443 for gateway).

### 5.4 Check WebSocket connectivity

Parcel uses WebSockets for live device event streaming. ALBs require specific settings for WebSocket support.

Verify the ALB listener has `idle_timeout` set high enough (recommended: 3600 seconds):
```bash
ALB_ARN=$(terraform output -raw alb_arn 2>/dev/null || \
  aws elbv2 describe-load-balancers \
    --query "LoadBalancers[?contains(LoadBalancerName,'parcel')].LoadBalancerArn | [0]" \
    --output text)

aws elbv2 describe-load-balancer-attributes \
  --load-balancer-arn "$ALB_ARN" \
  --query 'Attributes[?Key==`idle_timeout.timeout_seconds`]'
```

If the value is below 3600, add to your Terraform ALB module:
```hcl
# In the ALB module or directly:
resource "aws_lb" "this" {
  # ...
  idle_timeout = 3600
}
```

### 5.5 Verify device mTLS listener

```bash
aws elbv2 describe-listeners \
  --load-balancer-arn "$ALB_ARN" \
  --query 'Listeners[?Port==`8443`].{port:Port,mode:MutualAuthentication.Mode,trustStore:MutualAuthentication.TrustStoreArn}' \
  --output table
```

- During initial enrollment: `mode` should be `passthrough`.
- After fleet is enrolled: change `device_mtls_mode = "verify"` in `terraform.tfvars` and re-apply.
- If `TrustStoreArn` is empty after switching to `verify`, the device CA bundle was not uploaded to the correct S3 path.

---

## Phase 6 — Post-Apply Validation

### Health check
```bash
APP_URL=$(terraform output -raw app_url)
curl -f "$APP_URL/healthz"
```

### First login
- Open `APP_URL` in a browser.
- Log in with `AUTH_BOOTSTRAP_EMAIL` and `AUTH_BOOTSTRAP_PASSWORD` (from Secrets Manager).
- Immediately rotate the bootstrap password, generate recovery codes, and create a second admin account.

### Auto-migration confirmation
The `AUTO_MIGRATE=1` env var (set by default in `customer_stack`) runs all SQL migrations on startup. Confirm in ECS logs:
```bash
aws logs tail "/ecs/parcel-prod-control-plane" --since 10m | grep -i migrat
```

Expected: `migrations applied` or `schema up to date`.

### Validate secrets are not plaintext in ECS task definition
```bash
CP_TASK_DEF=$(aws ecs describe-services \
  --cluster "$CLUSTER" --services "$CP_SERVICE" \
  --query 'services[0].taskDefinition' --output text)

aws ecs describe-task-definition --task-definition "$CP_TASK_DEF" \
  --query 'taskDefinition.containerDefinitions[?name==`control-plane`] | [0].{env:environment[].name,secrets:secrets[].name}' \
  --output json
```

`AUTH_JWT_SECRET`, `AUTH_BOOTSTRAP_PASSWORD`, and `DATABASE_URL` must appear under `secrets`, not `env`.

---

## Phase 7 — Ongoing Customer Operations

Once deployed, the customer manages:
- User accounts and admin rotation.
- Enrollment profile creation and device approval.
- Artifact uploads and CI integration.
- Desired-state and group management.
- Platform upgrades (see below).

### Applying upgrades
When a new Parcel version is released:
1. Push the new container images to their ECR repository.
2. Update `control_plane_image` and `gateway_image` in `terraform.tfvars`.
3. Run `terraform apply` — ECS performs a rolling update with no downtime.

For database migration changes, `AUTO_MIGRATE=1` runs them automatically on startup. Check logs after the new task starts to confirm.

---

## Phase 8 — Backup and Disaster Recovery

### Backup strategy (customer-owned)

**Database (RDS PostgreSQL):**

Enable and verify RDS automated backups:
```bash
aws rds describe-db-instances \
  --query 'DBInstances[?contains(DBInstanceIdentifier, `parcel`)].{id:DBInstanceIdentifier,retention:BackupRetentionPeriod,multiAZ:MultiAZ,status:DBInstanceStatus}' \
  --output table
```

Recommended settings in `terraform.tfvars`:
```hcl
db_backup_retention_days = 14
db_multi_az              = true    # required for prod
db_deletion_protection   = true
```

For manual on-demand snapshots (before a major upgrade):
```bash
aws rds create-db-snapshot \
  --db-instance-identifier parcel-prod \
  --db-snapshot-identifier parcel-prod-pre-upgrade-$(date +%Y%m%d)
```

**Artifact store (S3):**
- S3 versioning is enabled by default in the `artifact_store` Terraform module.
- Add a lifecycle rule to expire old object versions after 90 days to control costs.
- For critical prod environments, enable S3 Cross-Region Replication (CRR):
  ```bash
  # Configure via Terraform aws_s3_bucket_replication_configuration
  # or add to the artifact_store module inputs if available
  ```

**RTO / RPO targets (recommended baselines for customer planning):**
- RPO: < 24 hours (daily RDS snapshot) — tighten with transaction logs if RDS Multi-AZ is active.
- RTO: < 4 hours for full stack restore from snapshot.

### Restore procedure

**Restore database from RDS snapshot:**
```bash
# List available snapshots
aws rds describe-db-snapshots \
  --db-instance-identifier parcel-prod \
  --query 'DBSnapshots[*].{id:DBSnapshotIdentifier,time:SnapshotCreateTime,status:Status}' \
  --output table

# Restore to a new instance (keeps original running until validation)
aws rds restore-db-instance-from-db-snapshot \
  --db-instance-identifier parcel-prod-restored \
  --db-snapshot-identifier <snapshot-id> \
  --db-subnet-group-name parcel-prod \
  --db-instance-class db.t3.medium \
  --no-publicly-accessible

aws rds wait db-instance-available \
  --db-instance-identifier parcel-prod-restored
```

After validation, update `DATABASE_URL` in Secrets Manager and force a new ECS deployment:
```bash
NEW_ENDPOINT=$(aws rds describe-db-instances \
  --db-instance-identifier parcel-prod-restored \
  --query 'DBInstances[0].Endpoint.Address' --output text)

aws secretsmanager put-secret-value \
  --secret-id "parcel/prod/database-url" \
  --secret-string "postgres://parcel:PASSWORD@${NEW_ENDPOINT}:5432/parcel?sslmode=require"

aws ecs update-service \
  --cluster parcel-prod \
  --service parcel-prod-control-plane \
  --force-new-deployment
```

**Restore artifacts from S3:**
- Individual artifacts: restore from S3 version history via console or `aws s3api get-object --version-id`.
- Bulk restore: `aws s3 sync s3://backup-bucket s3://parcel-prod-artifacts`.

**Full stack loss (catastrophic):**
1. Restore RDS snapshot to the target region/account.
2. Sync S3 artifact bucket to a new bucket.
3. Update all Secrets Manager secrets with new endpoints.
4. Re-run `terraform apply` with updated `terraform.tfvars` (new region, new resource ARNs).
5. Update DNS records to point to the new ALB.
6. Validate health endpoint and agent connectivity.

### DR drill (recommended quarterly)
1. Restore the most recent RDS snapshot to a test instance.
2. Point a test stack at it.
3. Verify `GET /healthz` returns `ok`.
4. Confirm one artifact can be downloaded and one agent can check in.
5. Destroy the test stack.
6. Record results and any gaps.

---

## Common Issues Reference

| Symptom | Likely Cause | Fix |
|---|---|---|
| ALB targets unhealthy | ECS task crashing | Check ECS logs; usually a missing env var or failed migration |
| `curl /healthz` → connection refused | Security group blocks ALB→ECS | Add inbound rule on ECS SG from ALB SG |
| WebSocket drops after 60s | ALB idle timeout too low | Set `idle_timeout = 3600` on ALB |
| Devices can't connect on port 8443 | mTLS trust store not uploaded | Upload `device-ca-bundle.pem` to S3 and re-apply |
| `AUTO_MIGRATE` not running | Env var missing | Confirm `AUTO_MIGRATE = "1"` in `customer_stack` locals |
| Secrets in plaintext in task def | `control_plane_secret_arns` not set | Move sensitive vars to `control_plane_secret_arns` |
| 403 on artifact upload | Artifact trust verification rejecting | Set `ARTIFACT_TRUST_VERIFICATION_MODE = "log_only"` temporarily; add signing key |

---

## Reference

- Terraform modules: `deploy/aws/terraform/modules/customer_stack/`
- Terraform env examples: `deploy/aws/terraform/envs/prod/`
- Full deployment guide: `docs/guides/deploy.md`
- Hardening reference: `docs/guides/deployment-hardening.md`
- Backup commands: `docs/guides/backup-restore.md`
- API contract: `docs/reference/icd.md`
- Customer first-agent guide: `docs/customer/first-agent-onboarding.md`
- CI integration: `docs/customer/ci-workflows.md`
