# AWS Customer Deployment Runbook (Vendor-Hosted, One Stack per Customer)

This runbook is the execution guide for deploying and operating **one isolated AWS stack per customer**.

Use it with:
- `docs/development/aws-cloud-setup-plan.md` (strategy)
- `docs/development/security-hardening.md` (hardening tasks and exit criteria)
- `deploy/aws/terraform/README.md` (Terraform scaffold)
- `scripts/aws-customer.sh` (CLI wrapper for setup/deploy/status)

## CLI Wrapper

Use `scripts/aws-customer.sh` to run setup and deployment from command line:

```bash
scripts/aws-customer.sh requirements
scripts/aws-customer.sh bootstrap-state --region us-east-1 --state-bucket hwops-tf-state --state-lock-table hwops-tf-locks
scripts/aws-customer.sh init --customer acme --env prod --region us-east-1 --state-bucket hwops-tf-state --state-lock-table hwops-tf-locks \
  --customer-domain acme.example.com --route53-zone-id Z1234 --acm-cert-arn arn:aws:acm:... \
  --device-mtls-bucket acme-security-assets \
  --artifact-pull-credentials-secret-id arn:aws:secretsmanager:us-east-1:111122223333:secret:hardwareops/acme/prod/artifact-pull-credentials-AbCdEf \
  --control-plane-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-control-plane:20260213 \
  --gateway-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-gateway:20260213
scripts/aws-customer.sh plan --customer acme --env prod --region us-east-1
scripts/aws-customer.sh apply --customer acme --env prod --region us-east-1 --auto-approve
scripts/aws-customer.sh status --customer acme --env prod --region us-east-1
```

### Optional Demo Fleet (3 persistent demo agents)

Build/push demo agent image:

```bash
AWS_PROFILE=hwops-admin AWS_REGION=us-east-1 \
ECR_REPOSITORY=hardwareops-demo-agent \
IMAGE_TAG=latest \
./scripts/aws-demo-image.sh
```

Generate customer config with demo fleet enabled:

```bash
scripts/aws-customer.sh init --customer acme --env dev --region us-east-1 \
  --state-bucket hwops-tf-state --state-lock-table hwops-tf-locks \
  --customer-domain acme.tryparcel.dev --route53-zone-id Z1234 --acm-cert-arn arn:aws:acm:... \
  --control-plane-image <cp-image-uri> --gateway-image <gw-image-uri> \
  --enable-demo-fleet --demo-agent-image <demo-agent-image-uri> --demo-agent-count 3
```

Seed demo artifacts for manual switching in UI:

```bash
BASE_URL=https://app.acme.tryparcel.dev \
AUTH_EMAIL=<bootstrap-admin-email> AUTH_PASSWORD=<bootstrap-admin-password> \
./scripts/aws-demo-seed.sh
```

Notes:
- Demo agents persist `/data` on EFS per slot, so device identity survives normal ECS restarts and rolling updates.
- Demo artifact switching remains manual through UI desired-state edits (no auto-switch workflow).
- Initial enrollment still requires `device_mtls_mode = "passthrough"` until certs exist.

## 1) Deployment Model

For each customer, create separate:
- DNS hosts (`app.<customer-domain>`, `agent.<customer-domain>`)
- VPC + ECS + RDS + S3 resources
- mTLS trust store bundle for device traffic
- secret namespace (`hardwareops/<customer>/<env>/...`)

This gives strong isolation and clean lifecycle management (provision, rotate, decommission) per customer.

## 2) Pre-Deployment Checklist

Before first `terraform apply` for a customer:
- AWS account/environment selected (`dev`, `staging`, `prod`)
- Route53 hosted zone available
- ACM certificate issued for app + agent hosts
- ECR images built/pushed (`control-plane`, `gateway`)
- Device CA created (for mTLS trust + device cert signing)
- Terraform state backend ready (S3 + DynamoDB lock table)

### Account/customer information you must provide

At minimum:
- AWS region
- AWS account/profile credentials with infra permissions
- Terraform state S3 bucket + DynamoDB lock table
- Customer slug + environment
- Route53 hosted zone ID
- App host + device host
- ACM certificate ARN
- Control-plane and gateway image URIs
- Device mTLS trust bundle S3 bucket/key

Optional (for pull adapters with `credentialRef`):
- Secrets Manager secret ID/ARN with credential map JSON

## 3) Certificate Model You Need

There are two certificate domains to manage:

1. **Server TLS certificate (human/UI + HTTPS endpoint)**
   - Terminates at ALB using ACM.
   - If ACM/public trust is used, customer operators do **not** need to install a custom CA cert in browsers.

2. **Device mTLS CA (agent identity)**
   - Used by control-plane to sign device certificates.
- Used by ALB trust store to verify device client certs in `verify` mode.

## 3.1) Pull Adapter Credentials in Secrets Manager (Optional)

If you want cloud pull ingest with `source.credentialRef` (for example Artifactory basic auth), create one secret per customer environment:

```bash
aws secretsmanager create-secret \
  --name hardwareops/acme/prod/artifact-pull-credentials \
  --secret-string '{
    "artifactory-demo": {
      "username": "admin",
      "password": "password"
    }
  }'
```

Then set in Terraform:

```hcl
artifact_pull_credentials_aws_secret_id = "arn:aws:secretsmanager:us-east-1:111122223333:secret:hardwareops/acme/prod/artifact-pull-credentials-AbCdEf"
```

Notes:
- ARN is preferred; name also works.
- Stack wiring now injects resolver env vars into control-plane and grants ECS task role `secretsmanager:GetSecretValue`/`DescribeSecret` scoped to that secret.

## 4) Create Per-Customer Device CA Assets

Run from repo root:

```bash
CUSTOMER=customer-a
ENV=prod
OUT_DIR="$PWD/out/${CUSTOMER}-${ENV}-certs" ./scripts/bootstrap-ca.sh
```

Outputs:
- `ca.crt` (device CA public cert)
- `ca.key` (device CA private key)

Create trust bundle for ALB:

```bash
cat "out/${CUSTOMER}-${ENV}-certs/ca.crt" > "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem"
```

Upload bundle to S3 (example):

```bash
aws s3 cp \
  "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem" \
  "s3://hardwareops-${CUSTOMER}-security-assets/mtls/device-ca-bundle.pem"
```

## 5) Terraform Per Customer

Use `deploy/aws/terraform/envs/prod` (or `staging`/`dev`):

```bash
cd deploy/aws/terraform/envs/prod
cp backend.hcl.example backend.hcl
cp terraform.tfvars.example terraform.tfvars
```

Set at minimum in `terraform.tfvars`:
- `customer_slug`
- `app_host` and `devices_host` (use `agent.<domain>` for devices host with current gateway routing)
- `route53_zone_id`
- `acm_certificate_arn`
- `control_plane_image`, `gateway_image`
- `device_mtls_bucket`, `device_mtls_key`
- `device_mtls_mode`

Deploy:

```bash
terraform init -backend-config=backend.hcl
terraform plan
terraform apply
```

## 6) Enrollment Strategy (Important)

Current practical rollout for new customer fleets:

1. Start with:
   - `device_mtls_mode = "passthrough"`
2. Enroll devices using token-based enrollment.
3. After fleet enrollment is complete, switch:
   - `device_mtls_mode = "verify"`
4. Re-apply Terraform.

Why: strict ALB `verify` mode requires client certificates on the listener. During first enrollment, devices do not have client certs yet.

## 7) What You Give the Customer for Device Onboarding

Per customer, provide a **device onboarding package** containing:

- `CONTROL_PLANE_URL` (agent host URL, ex: `https://agent.customer-a.example.com:8443`)
- Enrollment token(s) with short TTL
- Agent install instructions (`docs/agent-systemd.md`)
- Optional CA file for server trust if using private TLS (not needed with public ACM trust)

### Token creation (operator side)

Create enrollment token from authenticated operator API:

```bash
curl --cacert <server-ca-if-needed> \
  -H "Authorization: Bearer <operator-jwt>" \
  -H "Content-Type: application/json" \
  -X POST "https://app.customer-a.example.com/api/v1/enrollments" \
  -d '{"expiresInSec":3600}'
```

### Device enrollment (device side)

Use the token to call:
- `POST /api/v1/devices/enroll`

The response includes:
- `deviceId`
- `certPem` (device client cert)
- `caCertPem` (device CA cert)

Device stores these into:
- `device.crt`
- `device.key`
- `ca.crt`

Then regular check-in starts with mTLS.

## 8) Certificate Delivery Guidance

If customer asks "which certs do we install on devices?":

- With public ACM/server trust: usually **no browser/server CA install** needed.
- For agent mTLS, enrollment flow returns device cert + CA cert automatically.
- Only send standalone CA files to customer when:
  - they run private PKI TLS, or
  - they require manual offline provisioning.

## 9) Day-2 Operations Per Customer

- Rotate device CA on a controlled schedule.
- Update ALB trust store bundle/object version when CA changes.
- Use cert rotation endpoints/runbook from `docs/certs.md`.
- Keep backups/restore drills customer-specific (`docs/backup-restore.md`).

## 10) Current Gaps to Close Before Broad Production Rollout

Track these as must-do production hardening:
- Full secret/file injection path for CA key material in ECS tasks (avoid image-embedded private keys).
- WAF + alarm policies in Terraform modules.
- Autoscaling policies and SLO-based alert thresholds.
- Automated customer onboarding pipeline (create cert assets, tfvars, secrets, deploy, smoke test).

## 11) Cloud Validation for Pull Adapter

After `terraform apply`:

1. Confirm resolver envs are present on control-plane task:
```bash
aws ecs describe-task-definition \
  --task-definition <control-plane-task-definition-arn> \
  --query 'taskDefinition.containerDefinitions[?name==`control-plane`].environment' \
  --output table
```
Expected keys:
- `ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID`
- `ARTIFACT_PULL_CREDENTIALS_AWS_REGION`

2. Run Artifactory pull smoke test against cloud app endpoint:
```bash
BASE_URL=https://app.<customer-domain> \
INSECURE=1 \
RUN_SETUP=0 \
SETUP_OUTPUT_DIR=/tmp/hardwareops-artifactory-demo \
./scripts/test-artifactory-adapter.sh
```

3. Verify:
- Artifact exists in `/api/v1/artifacts`.
- Audit entry `artifact.pull` contains `sourceKind=artifactory`.
