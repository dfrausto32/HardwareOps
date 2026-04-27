# Playbook: Parcel Manages the AWS Environment (Vendor-Hosted)

## When to Use

Use this playbook when **Parcel provisions and operates the AWS infrastructure** on the customer's behalf. The customer accesses Parcel through a URL you provide; they do not own or manage the underlying AWS resources.

Choose this path when:
- The customer does not want to manage cloud infrastructure.
- The customer has purchased a managed SaaS-style engagement.
- You are deploying a new customer environment from scratch on Parcel-controlled AWS infrastructure.
- The customer needs a fast go-live without AWS expertise on their side.

Do **not** use this playbook when the customer is running Parcel in their own AWS account — use `customer-self-hosted-aws.md` instead.

---

## Responsibility Split

| Area | Parcel | Customer |
|---|---|---|
| AWS infrastructure lifecycle | Owner | —  |
| Terraform apply / destroy | Owner | — |
| TLS certificates and DNS | Owner | Provides domain if desired |
| RDS backups and retention | Owner | Notified of schedule |
| ECS service health and scaling | Owner | — |
| Platform upgrades | Owner | Approves maintenance window |
| Incident response (AWS-side) | Owner | Escalation point |
| User and admin management | Shared | Owner |
| Enrollment profile creation | Shared | Owner for day-to-day |
| Desired-state and artifact management | Shared | Owner |
| Artifact trust policy | Shared | Owner if delegated |
| CI integration | Shared | Owner |

---

## Phase 1 — Information Gathering

Collect what the customer provides. Anything not provided is your decision.

### Customer-provided (accept whatever they give you)

| Item | Required | Notes |
|---|---|---|
| Preferred domain or subdomain | Optional | e.g. `ops.customer.com`. You choose one if not provided. |
| Admin user email | Optional | Bootstrap admin. You generate one if not provided. |
| AWS region preference | Optional | Default to `us-east-1` if not specified. |
| License key / contract ID | Yes | Must be applied before go-live. |
| CI provider (GitHub, GitLab, Jenkins) | Optional | Needed only if workload identity is in scope. |
| Allowed source IP ranges for operator access | Optional | Defaults to `0.0.0.0/0`; restrict if customer requires it. |

### You decide (record your choices)

- Customer slug (e.g. `acme`) — used in all resource names.
- Environment (`prod`, `staging`, `dev`).
- AWS region.
- Domain and subdomain if not provided.
- Admin bootstrap email and password (store in Secrets Manager immediately).
- RDS instance class and storage sizing based on fleet size.
- Backup retention period (recommended: 14 days for prod).

---

## Phase 2 — Pre-Provisioning Setup

### 2.1 Create evidence directory

```bash
CUSTOMER=acme
ENV=prod
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
EVIDENCE_DIR="$PWD/out/aws-hardening-${CUSTOMER}-${ENV}-${STAMP}"
mkdir -p "$EVIDENCE_DIR"
```

### 2.2 Bootstrap Terraform state bucket

```bash
scripts/aws-customer.sh requirements

scripts/aws-customer.sh bootstrap-state \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks
```

### 2.3 Generate the device CA

```bash
CUSTOMER=acme
ENV=prod
OUT_DIR="$PWD/out/${CUSTOMER}-${ENV}-certs" ./scripts/bootstrap-ca.sh
cat "out/${CUSTOMER}-${ENV}-certs/ca.crt" > "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem"
```

Upload the ALB trust bundle to S3:

```bash
aws s3 cp \
  "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem" \
  "s3://parcel-${CUSTOMER}-${ENV}-security-assets/mtls/device-ca-bundle.pem"
```

---

## Phase 3 — Secrets Manager Setup

All sensitive values must be in Secrets Manager before Terraform apply. Never pass them as inline `tfvars`.

### 3.1 Core secrets (required)

```bash
# JWT signing key
JWT_SECRET=$(openssl rand -base64 48)
aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/jwt-secret" \
  --secret-string "$JWT_SECRET"

# Bootstrap admin password (if you are generating it)
BOOTSTRAP_PASSWORD=$(openssl rand -base64 24)
aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/bootstrap-password" \
  --secret-string "$BOOTSTRAP_PASSWORD"

# DATABASE_URL (constructed after RDS apply or pre-created)
# Set database_url_secret_arn in tfvars to eliminate plaintext DATABASE_URL from ECS task defs.
```

Required in `control_plane_secret_arns` in `terraform.tfvars`:
```hcl
control_plane_secret_arns = {
  AUTH_JWT_SECRET           = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/acme/prod/jwt-secret-SUFFIX"
  AUTH_BOOTSTRAP_PASSWORD   = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/acme/prod/bootstrap-password-SUFFIX"
}
database_url_secret_arn = "arn:aws:secretsmanager:us-east-1:ACCOUNT:secret:parcel/acme/prod/database-url-SUFFIX"
```

### 3.2 Artifact signing keys (if using `require_verified`)

```bash
./scripts/build-trusted-signing-keys.sh /tmp/trusted-signing-keys.json /path/to/ed25519.pub

aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/trusted-signing-keys" \
  --secret-string file:///tmp/trusted-signing-keys.json
```

### 3.3 CI workload identity (if applicable)

See `docs/development/aws-customer-deployment-runbook.md` section 6.4 for the provider config payload and secret creation commands.

---

## Phase 4 — Terraform Init and Apply

### 4.1 Initialize the customer stack

```bash
scripts/aws-customer.sh init \
  --customer acme \
  --env prod \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks \
  --customer-domain acme.example.com \
  --route53-zone-id Z1234567890 \
  --acm-cert-arn arn:aws:acm:us-east-1:ACCOUNT:certificate/CERT-ID \
  --device-mtls-bucket parcel-acme-prod-security-assets \
  --control-plane-image ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-control-plane:VERSION \
  --gateway-image ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/parcel-gateway:VERSION
```

### 4.2 Run the pre-apply hardening gate

```bash
./scripts/aws-hardening-check.sh config \
  --customer acme \
  --env prod | tee "$EVIDENCE_DIR/00-config-gate.txt"
```

Do not proceed if any check fails.

### 4.3 Plan and apply

```bash
scripts/aws-customer.sh plan \
  --customer acme --env prod --region us-east-1 \
  | tee "$EVIDENCE_DIR/01-plan.txt"

scripts/aws-customer.sh apply \
  --customer acme --env prod --region us-east-1 --auto-approve \
  | tee "$EVIDENCE_DIR/02-apply.txt"

scripts/aws-customer.sh status \
  --customer acme --env prod --region us-east-1 \
  | tee "$EVIDENCE_DIR/03-status.txt"
```

### 4.4 First-time device enrollment mode

Set `device_mtls_mode = "passthrough"` during initial fleet enrollment, then switch to `"verify"` and re-apply before go-live sign-off.

---

## Phase 5 — Post-Apply Validation

```bash
./scripts/aws-hardening-check.sh deployment \
  --customer acme --env prod --region us-east-1 --profile hwops-admin \
  | tee "$EVIDENCE_DIR/04-deployment-gate.txt"
```

Verify:
- Health endpoint: `curl https://app.acme.example.com/healthz`
- ECS services are stable and running.
- Secrets are injected via Secrets Manager (not plaintext env vars).
- WAF is attached to the ALB.
- `enableExecuteCommand` is `False`.
- SNS on-call subscription is confirmed and test publish succeeds.

Full acceptance command reference: `docs/development/aws-customer-deployment-runbook.md` sections 9 and 10.

---

## Phase 6 — Customer Handoff

Deliver to the customer (via secure channel):

| Item | Notes |
|---|---|
| App URL | e.g. `https://app.acme.example.com` |
| Agent URL | e.g. `https://agent.acme.example.com:8443` |
| Bootstrap admin email + password | One-time; customer must rotate on first login |
| Device CA public cert (`ca.crt`) | For agent trust configuration |
| Enrollment profile procedure | Link to `docs/customer/first-agent-onboarding.md` |
| Escalation contact | Your on-call email or ticket queue |

First login checklist to give the customer (from `docs/customer/setup-vendor-hosted-aws.md`):
1. Rotate the bootstrap password immediately.
2. Generate recovery codes.
3. Create a second admin account.
4. Review trust and auth settings.

---

## Phase 7 — Ongoing Shared Operations

### Your responsibilities (infrastructure layer)
- AWS resource health monitoring and alarm response.
- Platform upgrades — coordinate maintenance windows with the customer.
- RDS backup monitoring and retention enforcement.
- TLS certificate renewal (ACM handles this automatically; verify annually).
- Security incident response for AWS-layer events.
- ECS Exec access through break-glass only (see runbook section 12).

### Customer responsibilities (application layer)
- User and admin account management.
- Enrollment profile creation and device approval.
- Artifact upload and CI integration.
- Desired-state and group management.
- Password and recovery code management.

### Escalation: what the customer escalates to you
- App URL or agent URL unavailable.
- TLS or connectivity failures affecting multiple devices.
- Backup or restore requests.
- Certificate rotation issues.
- AWS-side security incidents.

---

## Phase 8 — Backup and Disaster Recovery

### Backup strategy (managed by Parcel)

**Database (RDS PostgreSQL):**
- Automated daily snapshots via RDS backup retention (14 days recommended for prod).
- Enable multi-AZ (`db_multi_az = true`) for prod environments.
- Final snapshot is retained on stack deletion (`skip_final_snapshot = false` in Terraform).

Verify backup is enabled:
```bash
aws rds describe-db-instances \
  --query 'DBInstances[?contains(DBInstanceIdentifier, `acme`)].{id:DBInstanceIdentifier,retention:BackupRetentionPeriod,multiAZ:MultiAZ}'
```

**Artifact store (S3):**
- S3 versioning is enabled by the `artifact_store` module.
- Enable S3 replication to a second region for prod if RTO/RPO require it.

**RTO/RPO targets (recommended baselines):**
- RTO: < 4 hours for full stack restore.
- RPO: < 24 hours (aligned to daily RDS snapshot cadence).

### Restore procedure

**Restore RDS from snapshot:**
```bash
# List available snapshots
aws rds describe-db-snapshots \
  --db-instance-identifier parcel-acme-prod \
  --query 'DBSnapshots[*].{id:DBSnapshotIdentifier,time:SnapshotCreateTime,status:Status}' \
  --output table

# Restore to a new instance
aws rds restore-db-instance-from-db-snapshot \
  --db-instance-identifier parcel-acme-prod-restore \
  --db-snapshot-identifier <snapshot-id> \
  --db-instance-class db.t3.medium \
  --no-publicly-accessible

# Wait for availability
aws rds wait db-instance-available \
  --db-instance-identifier parcel-acme-prod-restore
```

Then update the `DATABASE_URL` secret in Secrets Manager to point to the restored endpoint and force a new ECS deployment:
```bash
aws secretsmanager put-secret-value \
  --secret-id "parcel/acme/prod/database-url" \
  --secret-string "postgres://user:pass@RESTORED-ENDPOINT:5432/parcel?sslmode=require"

aws ecs update-service \
  --cluster parcel-acme-prod \
  --service parcel-acme-prod-control-plane \
  --force-new-deployment
```

**Restore S3 artifacts:**
- S3 versioning allows per-object recovery. For bulk restore, use `aws s3 sync` from a replica or versioned copy.

**Full stack disaster (region loss):**
1. Restore RDS snapshot to the target region.
2. Sync S3 bucket contents to a new bucket in the target region.
3. Re-run `scripts/aws-customer.sh apply` with updated region and new resource ARNs.
4. Update DNS records to the new ALB endpoint.

### DR verification (quarterly)
- Restore the most recent RDS snapshot to a test instance.
- Verify `GET /healthz` succeeds against the restored instance.
- Confirm artifact downloads and agent check-ins function.
- Document results in the evidence directory.

---

## Reference

- Terraform modules: `deploy/aws/terraform/modules/customer_stack/`
- Deployment runbook: `docs/development/aws-customer-deployment-runbook.md`
- Customer operator guide: `docs/customer/setup-vendor-hosted-aws.md`
- Hardening reference: `docs/deployment-hardening.md`
- Incident response: `docs/incidents/ir-runbook.md`
- Backup commands: `docs/backup-restore.md`
