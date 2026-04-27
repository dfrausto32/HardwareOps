# Parcel — AWS End-to-End Test Plan

This document is a complete, sequential runbook for validating the Parcel platform on a real AWS deployment. It covers infrastructure bring-up, agent enrollment, artifact ingest, desired-state apply, and visual confirmation — from zero to a working managed device.

**Estimated time:** 2–3 hours for a first run (30 min if infrastructure already exists).

**What you will confirm at the end:**
1. A single ECS Fargate control plane is running and healthy.
2. A single EC2 agent enrolled via the approval-mode first-contact flow.
3. Two artifacts (`hwops-banner` v1 and `hwops-config` v1) applied to the EC2 agent — files visible on disk via SSH.
4. An in-place artifact upgrade (`hwops-banner` v2) that changes the file content — confirming the upgrade path.
5. All hardening gates pass (no plaintext secrets, WAF attached, ECS Exec disabled, alarms wired).

---

## Prerequisites

### Tools (install on your workstation before starting)
```bash
aws --version          # >= 2.x
terraform --version    # >= 1.5
gh --version           # GitHub CLI, authenticated to this repo
jq --version           # >= 1.6
python3 --version      # >= 3.9 (for artifact-pack.py)
curl --version
ssh -V
```

### AWS requirements
- AWS account with the ability to create: VPC, ECS, RDS, ALB, ACM, S3, Secrets Manager, CloudWatch, WAFv2, Route53 records, IAM roles.
- A Route53 hosted zone you control (e.g. `test.parcel.internal`).
- An ACM certificate for the zone (or wildcard) in the same region.
- A key pair for EC2 SSH access.

### Repository checkout
```bash
git clone git@github.com:dfrausto32/Parcel.git
cd Parcel
```

### Shell variables used throughout this guide
Set these once at the start of your session. All subsequent commands reference them.
```bash
export AWS_REGION=us-east-1
export AWS_PROFILE=hwops-admin        # AWS CLI profile with admin-level access
export CUSTOMER=e2e-test              # Customer slug — used in resource names
export ENV=dev                        # dev | staging | prod
export DOMAIN=test.parcel.internal
export APP_HOST="app.${DOMAIN}"
export DEVICES_HOST="agent.${DOMAIN}"
export ROUTE53_ZONE_ID=Z0123456789ABCDEF    # Your Route53 zone ID
export ACM_CERT_ARN="arn:aws:acm:${AWS_REGION}:111122223333:certificate/..."
export EC2_KEY_NAME=my-key-pair       # EC2 key pair name
export EC2_KEY_PATH=~/.ssh/my-key-pair.pem
export STATE_BUCKET=hwops-tf-state-${CUSTOMER}
export STATE_LOCK_TABLE=hwops-tf-locks-${CUSTOMER}
export MTLS_BUCKET=${CUSTOMER}-security-assets
export CONTROL_PLANE_IMAGE="111122223333.dkr.ecr.${AWS_REGION}.amazonaws.com/parcel-control-plane:latest"
export GATEWAY_IMAGE="111122223333.dkr.ecr.${AWS_REGION}.amazonaws.com/parcel-gateway:latest"
```

---

## Step 1 — Build the Two Test Artifacts

These artifacts write visible files to the agent host. You will confirm they applied by reading the files over SSH.

### 1.1 — Artifact A: `hwops-banner` v1.0.0

Writes `/etc/hwops-banner.txt` on the agent.

```bash
mkdir -p /tmp/hwops-artifacts/banner-v1/files
cat > /tmp/hwops-artifacts/banner-v1/files/banner.txt <<'EOF'
== Parcel Managed Device ==
Artifact  : hwops-banner
Version   : 1.0.0
Managed by: Parcel Control Plane
EOF

cat > /tmp/hwops-artifacts/banner-v1/plan.yaml <<'EOF'
version: "1"
steps:
  - id: copy-banner
    type: file.copy
    timeoutSec: 10
    params:
      src: files/banner.txt
      dest: /etc/hwops-banner.txt
EOF
```

### 1.2 — Artifact B: `hwops-config` v1.0.0

Writes `/opt/parcel/app-config.json` on the agent.

```bash
mkdir -p /tmp/hwops-artifacts/config-v1/files
cat > /tmp/hwops-artifacts/config-v1/files/app-config.json <<'EOF'
{
  "artifact": "hwops-config",
  "version": "1.0.0",
  "telemetry": {
    "enabled": true,
    "endpoint": "https://metrics.parcel.internal"
  },
  "logLevel": "info"
}
EOF

cat > /tmp/hwops-artifacts/config-v1/plan.yaml <<'EOF'
version: "1"
steps:
  - id: ensure-dir
    type: file.copy
    timeoutSec: 10
    params:
      src: files/app-config.json
      dest: /opt/parcel/app-config.json
EOF
```

### 1.3 — Artifact C: `hwops-banner` v2.0.0 (for upgrade test)

Same artifact, new version with different content. Build now; upload later.

```bash
mkdir -p /tmp/hwops-artifacts/banner-v2/files
cat > /tmp/hwops-artifacts/banner-v2/files/banner.txt <<'EOF'
== Parcel Managed Device ==
Artifact  : hwops-banner
Version   : 2.0.0  *** UPGRADED ***
Managed by: Parcel Control Plane
EOF

cp /tmp/hwops-artifacts/banner-v1/plan.yaml /tmp/hwops-artifacts/banner-v2/plan.yaml
```

---

## Step 2 — Prepare AWS Prerequisites

### 2.1 — Terraform state backend
```bash
./scripts/aws-customer.sh bootstrap-state \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE" \
  --state-bucket "$STATE_BUCKET" \
  --state-lock-table "$STATE_LOCK_TABLE"
```
Expected: "State backend ready" with bucket and table names printed.

### 2.2 — Device CA bucket
```bash
aws s3 mb "s3://${MTLS_BUCKET}" \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE"
```

### 2.3 — Bootstrap device CA and upload trust store
```bash
./scripts/bootstrap-ca.sh \
  --out-dir /tmp/hwops-ca \
  --cn "Parcel Device CA (e2e-test)"

# Upload trust bundle to S3
aws s3 cp /tmp/hwops-ca/ca-bundle.pem \
  "s3://${MTLS_BUCKET}/mtls/device-ca-bundle.pem" \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE"

echo "CA bundle uploaded."
```
Save the CA key path — you will need `/tmp/hwops-ca/ca.key` and `/tmp/hwops-ca/ca.crt`.

---

## Step 3 — Create Secrets in Secrets Manager

Move all sensitive values to Secrets Manager before Terraform apply.

```bash
# Generates strong random values
JWT_SECRET=$(openssl rand -base64 48)
BOOTSTRAP_PASSWORD=$(openssl rand -base64 16 | tr -dc 'a-zA-Z0-9' | head -c 20)

# Create secrets
aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/auth-jwt-secret" \
  --secret-string "$JWT_SECRET" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/auth-bootstrap-password" \
  --secret-string "$BOOTSTRAP_PASSWORD" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

# Database URL — set after Terraform creates RDS (update this ARN after Step 6)
# We create the secret now with a placeholder; update after apply.
aws secretsmanager create-secret \
  --name "parcel/${CUSTOMER}/${ENV}/database-url" \
  --secret-string "postgres://PLACEHOLDER" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

# Save ARNs
export JWT_SECRET_ARN=$(aws secretsmanager describe-secret \
  --secret-id "parcel/${CUSTOMER}/${ENV}/auth-jwt-secret" \
  --query ARN --output text --region "$AWS_REGION" --profile "$AWS_PROFILE")

export BOOTSTRAP_PASSWORD_ARN=$(aws secretsmanager describe-secret \
  --secret-id "parcel/${CUSTOMER}/${ENV}/auth-bootstrap-password" \
  --query ARN --output text --region "$AWS_REGION" --profile "$AWS_PROFILE")

export DATABASE_URL_ARN=$(aws secretsmanager describe-secret \
  --secret-id "parcel/${CUSTOMER}/${ENV}/database-url" \
  --query ARN --output text --region "$AWS_REGION" --profile "$AWS_PROFILE")

echo "JWT secret ARN:         $JWT_SECRET_ARN"
echo "Bootstrap password ARN: $BOOTSTRAP_PASSWORD_ARN"
echo "Database URL ARN:       $DATABASE_URL_ARN"

# Save bootstrap email for later use
export BOOTSTRAP_EMAIL="admin@${DOMAIN}"
echo "Bootstrap credentials: $BOOTSTRAP_EMAIL / $BOOTSTRAP_PASSWORD"
echo "Save these — you need them to log in to the UI."
```

---

## Step 4 — Configure Terraform

### 4.1 — Initialize workspace
```bash
./scripts/aws-customer.sh init \
  --customer "$CUSTOMER" \
  --env "$ENV" \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE" \
  --state-bucket "$STATE_BUCKET" \
  --state-lock-table "$STATE_LOCK_TABLE" \
  --customer-domain "$DOMAIN" \
  --route53-zone-id "$ROUTE53_ZONE_ID" \
  --acm-cert-arn "$ACM_CERT_ARN" \
  --device-mtls-bucket "$MTLS_BUCKET" \
  --device-mtls-mode passthrough \
  --control-plane-image "$CONTROL_PLANE_IMAGE" \
  --gateway-image "$GATEWAY_IMAGE"
```
This creates `.aws-customers/${CUSTOMER}/${ENV}/terraform.tfvars`.

### 4.2 — Wire secrets and hardening settings

Edit `.aws-customers/${CUSTOMER}/${ENV}/terraform.tfvars` and set the following. The `init` command creates a stub; replace the placeholder values:

```hcl
# Secrets Manager injection — required for hardening gate
control_plane_secret_arns = {
  AUTH_JWT_SECRET          = "<JWT_SECRET_ARN>"
  AUTH_BOOTSTRAP_PASSWORD  = "<BOOTSTRAP_PASSWORD_ARN>"
  DATABASE_URL             = "<DATABASE_URL_ARN>"
}

# Move secrets OUT of plaintext env block
control_plane_env = {
  AUTH_MODE                = "local"
  AUTH_BOOTSTRAP_EMAIL     = "admin@<DOMAIN>"
  AUTO_MIGRATE             = "1"
  MIGRATIONS_DIR           = "/app/migrations"
  TRUST_PROXY              = "1"
  ARTIFACT_SIGNATURE_REQUIRE_DEFAULT = "0"
}

# Alarm routing
alarm_sns_email = "<YOUR_EMAIL>"

# ECS exec off (already default false; confirm it is not set to true)

# database_url_secret_arn routes DATABASE_URL via Secrets Manager
database_url_secret_arn = "<DATABASE_URL_ARN>"
```

Use `envsubst` to fill in the ARNs automatically:
```bash
TFVARS=".aws-customers/${CUSTOMER}/${ENV}/terraform.tfvars"

# Replace ARN placeholders
sed -i "s|<JWT_SECRET_ARN>|${JWT_SECRET_ARN}|g" "$TFVARS"
sed -i "s|<BOOTSTRAP_PASSWORD_ARN>|${BOOTSTRAP_PASSWORD_ARN}|g" "$TFVARS"
sed -i "s|<DATABASE_URL_ARN>|${DATABASE_URL_ARN}|g" "$TFVARS"
sed -i "s|<DOMAIN>|${DOMAIN}|g" "$TFVARS"
sed -i "s|<YOUR_EMAIL>|${YOUR_EMAIL:-ops@example.com}|g" "$TFVARS"
```

---

## Step 5 — Pre-Apply Hardening Gate

```bash
./scripts/aws-hardening-check.sh config \
  --customer "$CUSTOMER" \
  --env "$ENV" 2>&1 | tee /tmp/hwops-evidence/00-config-gate.txt

echo "Exit code: $?"
```

**Expected:** All checks pass. Failures to look for:
- `FAIL: DATABASE_URL found as plaintext` — move `DATABASE_URL` to `control_plane_secret_arns`
- `FAIL: AUTH_JWT_SECRET found as plaintext` — move to `control_plane_secret_arns`
- `FAIL: No aws_cloudwatch_metric_alarm resources found` — alarms.tf is present; check TF_DIR path

Resolve any failures before continuing. Do not proceed to apply with a failing config gate.

---

## Step 6 — Terraform Plan and Apply

```bash
mkdir -p /tmp/hwops-evidence

./scripts/aws-customer.sh plan \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  2>&1 | tee /tmp/hwops-evidence/01-plan.txt

# Review the plan. Key resources to confirm:
#   + aws_ecs_cluster
#   + aws_ecs_service (control-plane, gateway)
#   + aws_db_instance
#   + aws_lb
#   + aws_wafv2_web_acl
#   + aws_cloudwatch_metric_alarm (8 alarms)
#   + aws_sns_topic

./scripts/aws-customer.sh apply \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  --auto-approve \
  2>&1 | tee /tmp/hwops-evidence/02-apply.txt

echo "Apply exit code: $?"
```

Apply takes 10–15 minutes (RDS provisioning dominates).

### 6.1 — Capture outputs and update DATABASE_URL secret
```bash
./scripts/aws-customer.sh output \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  2>&1 | tee /tmp/hwops-evidence/03-outputs.txt

# Extract RDS endpoint
RDS_ENDPOINT=$(terraform -chdir="deploy/aws/terraform/envs/${ENV}" output -raw rds_endpoint)
DB_PASSWORD="$BOOTSTRAP_PASSWORD"   # reuse for simplicity in test; use distinct password in real prod

DATABASE_URL="postgres://parcel:${DB_PASSWORD}@${RDS_ENDPOINT}:5432/parcel?sslmode=require"

# Update the DATABASE_URL secret with the real connection string
aws secretsmanager put-secret-value \
  --secret-id "$DATABASE_URL_ARN" \
  --secret-string "$DATABASE_URL" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

echo "DATABASE_URL secret updated."

# Force a new ECS deployment to pick up the updated secret
./scripts/aws-customer.sh deploy \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  --service control-plane

echo "Waiting for ECS deployment to stabilize (2 min)..."
sleep 120
```

### 6.2 — Verify control plane is reachable
```bash
APP_URL="https://${APP_HOST}"

curl -sf "${APP_URL}/api/v1/health" | jq .
# Expected: {"status":"ok","db":"ok"}

curl -sf "${APP_URL}/api/v1/auth/status" | jq .
# Expected: {"enabled":true,"mode":"local","oidcEnabled":false}
```

---

## Step 7 — Post-Apply Hardening Gate

```bash
ALB_DNS=$(terraform -chdir="deploy/aws/terraform/envs/${ENV}" output -raw alb_dns_name)
SNS_ARN=$(terraform -chdir="deploy/aws/terraform/envs/${ENV}" output -raw alerts_sns_topic_arn)

./scripts/aws-hardening-check.sh deployment \
  --customer "$CUSTOMER" \
  --env "$ENV" \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE" \
  --alb-dns "$ALB_DNS" \
  --sns-topic-arn "$SNS_ARN" \
  --alarm-prefix "${CUSTOMER}-${ENV}" \
  2>&1 | tee /tmp/hwops-evidence/04-deployment-gate.txt

echo "Deployment gate exit code: $?"
```

**Expected output (all passing):**
```
[PASS] ECS Exec disabled (control-plane)
[PASS] ECS Exec disabled (gateway)
[PASS] DATABASE_URL not in plaintext env (control-plane)
[PASS] AUTH_JWT_SECRET not in plaintext env (control-plane)
[PASS] WAF associated with ALB
[PASS] Device listener (8443) in verify/passthrough mode
[PASS] Port 443 ingress not open to 0.0.0.0/0
[PASS] CloudWatch alarms (8) all have SNS action
```

---

## Step 8 — Log In to the Control Plane UI

Open `https://${APP_HOST}` in a browser. Log in with:
- **Email:** `admin@${DOMAIN}` (the `BOOTSTRAP_EMAIL` set above)
- **Password:** `$BOOTSTRAP_PASSWORD` (printed in Step 3)

**Confirm:**
- [ ] Login succeeds and dashboard loads
- [ ] No "failed to fetch" errors in console
- [ ] Devices page shows zero devices
- [ ] Artifacts page shows zero artifacts

---

## Step 9 — Launch EC2 Agent Host

```bash
# Find latest Amazon Linux 2023 AMI
AMI_ID=$(aws ec2 describe-images \
  --owners amazon \
  --filters "Name=name,Values=al2023-ami-2023*-x86_64" \
            "Name=state,Values=available" \
  --query 'sort_by(Images, &CreationDate)[-1].ImageId' \
  --output text \
  --region "$AWS_REGION" --profile "$AWS_PROFILE")

echo "Using AMI: $AMI_ID"

# Get the VPC and public subnet from Terraform outputs
VPC_ID=$(terraform -chdir="deploy/aws/terraform/envs/${ENV}" output -raw vpc_id)
PUBLIC_SUBNET=$(terraform -chdir="deploy/aws/terraform/envs/${ENV}" \
  output -json public_subnet_ids | jq -r '.[0]')

# Create a security group for the agent EC2
SG_ID=$(aws ec2 create-security-group \
  --group-name "hwops-agent-test-${CUSTOMER}" \
  --description "Parcel test agent - outbound only" \
  --vpc-id "$VPC_ID" \
  --query GroupId --output text \
  --region "$AWS_REGION" --profile "$AWS_PROFILE")

# Outbound: allow all (agent calls out to control plane)
aws ec2 authorize-security-group-egress \
  --group-id "$SG_ID" \
  --protocol all --cidr 0.0.0.0/0 \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

# Inbound: SSH from your IP only
MY_IP=$(curl -s https://checkip.amazonaws.com)
aws ec2 authorize-security-group-ingress \
  --group-id "$SG_ID" \
  --protocol tcp --port 22 --cidr "${MY_IP}/32" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

# Launch the instance
INSTANCE_ID=$(aws ec2 run-instances \
  --image-id "$AMI_ID" \
  --instance-type t3.small \
  --key-name "$EC2_KEY_NAME" \
  --subnet-id "$PUBLIC_SUBNET" \
  --security-group-ids "$SG_ID" \
  --associate-public-ip-address \
  --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=hwops-agent-${CUSTOMER}}]" \
  --query 'Instances[0].InstanceId' --output text \
  --region "$AWS_REGION" --profile "$AWS_PROFILE")

echo "Instance: $INSTANCE_ID — waiting for running state..."

aws ec2 wait instance-running \
  --instance-ids "$INSTANCE_ID" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

AGENT_HOST=$(aws ec2 describe-instances \
  --instance-ids "$INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' \
  --output text \
  --region "$AWS_REGION" --profile "$AWS_PROFILE")

echo "Agent host: $AGENT_HOST"
echo "SSH: ssh -i $EC2_KEY_PATH ec2-user@$AGENT_HOST"

# Wait for SSH to be ready
sleep 30
ssh -i "$EC2_KEY_PATH" -o StrictHostKeyChecking=no ec2-user@"$AGENT_HOST" "echo SSH OK"
```

---

## Step 10 — Create Enrollment Profile

```bash
# Authenticate to the control plane
AUTH_RESPONSE=$(curl -sf -X POST "${APP_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${BOOTSTRAP_EMAIL}\",\"password\":\"${BOOTSTRAP_PASSWORD}\"}")

AUTH_TOKEN=$(echo "$AUTH_RESPONSE" | jq -r '.token')
echo "Authenticated. Token: ${AUTH_TOKEN:0:20}..."

# Create enrollment profile (approval-required, 1 use)
PROFILE_RESPONSE=$(curl -sf -X POST "${APP_URL}/api/v1/enrollment-profiles" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "e2e-test-agent",
    "expiresInSec": 86400,
    "maxUses": 1,
    "requireApproval": true,
    "defaultLabels": {"env": "e2e-test", "role": "test-agent"}
  }')

PROFILE_TOKEN=$(echo "$PROFILE_RESPONSE" | jq -r '.bootstrapToken')
PROFILE_ID=$(echo "$PROFILE_RESPONSE" | jq -r '.profileId')

echo "Profile ID:    $PROFILE_ID"
echo "Profile token: $PROFILE_TOKEN"
```

---

## Step 11 — Install Agent on EC2

### 11.1 — Copy agent binary and CA cert to EC2

```bash
# Replace with your actual agent binary path
AGENT_BINARY=./dist/parcel-agent-linux-amd64

scp -i "$EC2_KEY_PATH" \
  "$AGENT_BINARY" \
  /tmp/hwops-ca/ca.crt \
  "ec2-user@${AGENT_HOST}:/tmp/"

echo "Files copied."
```

### 11.2 — Run installer on EC2

The `DEVICES_HOST` is the agent-facing endpoint (`agent.${DOMAIN}`).

```bash
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" "
  sudo ./scripts/agent-install.sh \
    AGENT_SRC=/tmp/parcel-agent-linux-amd64 \
    CONTROL_PLANE_URL=https://${DEVICES_HOST} \
    CONTROL_PLANE_CA_CERT_SRC=/tmp/ca.crt \
    AGENT_ENROLL_MODE=approval \
    ENROLLMENT_PROFILE_TOKEN=${PROFILE_TOKEN} \
    START_SERVICE=1
"
```

Expected output:
```
[agent-install] Created user: parcel
[agent-install] Agent binary installed: /usr/local/bin/parcel-agent
[agent-install] Enrollment mode: approval
[agent-install] Service started: parcel-agent
[agent-install] Next step: approve pending enrollment request in the control plane UI.
```

### 11.3 — Verify agent is running and requesting enrollment

```bash
# Check service is running
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" \
  "sudo systemctl status parcel-agent --no-pager"

# Check logs
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" \
  "sudo journalctl -u parcel-agent -n 30 --no-pager"
```

Expected in logs:
```
submitting enrollment request... waiting for approval
```

---

## Step 12 — Approve Enrollment Request

### Via API (automated):
```bash
# Wait up to 2 min for the request to appear
for i in $(seq 1 24); do
  REQUEST_ID=$(curl -sf "${APP_URL}/api/v1/pending-enrollments?status=pending" \
    -H "Authorization: Bearer $AUTH_TOKEN" | \
    jq -r '.items[0].requestId // empty')
  [ -n "$REQUEST_ID" ] && break
  echo "Waiting for enrollment request... ($i/24)"
  sleep 5
done

[ -z "$REQUEST_ID" ] && { echo "ERROR: No pending enrollment request found after 120s"; exit 1; }
echo "Found request: $REQUEST_ID"

# Approve it
curl -sf -X POST "${APP_URL}/api/v1/pending-enrollments/${REQUEST_ID}/approve" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' | jq .

echo "Enrollment approved."
```

### Via UI (manual confirmation):
Navigate to **Security → Pending Enrollments**. You should see one entry with `status=pending`. Click **Approve**.

---

## Step 13 — Verify Active Check-In

```bash
# Wait up to 3 min for the agent to reach active status
for i in $(seq 1 36); do
  DEVICE_STATUS=$(curl -sf "${APP_URL}/api/v1/devices?limit=10" \
    -H "Authorization: Bearer $AUTH_TOKEN" | \
    jq -r '.items[0].status // empty')
  [ "$DEVICE_STATUS" = "active" ] && break
  echo "Device status: ${DEVICE_STATUS:-not found} ($i/36)"
  sleep 5
done

[ "$DEVICE_STATUS" != "active" ] && { echo "ERROR: Device did not reach active status"; exit 1; }

DEVICE_ID=$(curl -sf "${APP_URL}/api/v1/devices?limit=10" \
  -H "Authorization: Bearer $AUTH_TOKEN" | \
  jq -r '.items[0].deviceId')

echo "Device active: $DEVICE_ID"
```

**UI confirmation:**
- Devices page shows 1 device with status **active** and a green indicator.
- Device labels include `env=e2e-test`, `role=test-agent`.

---

## Step 14 — Ingest Artifacts

### 14.1 — Create a service token for CI upload

```bash
CI_TOKEN_RESPONSE=$(curl -sf -X POST "${APP_URL}/api/v1/auth/service-tokens" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "e2e-ci",
    "scope": "artifact.publish",
    "expiresInSec": 3600
  }')

CI_SERVICE_TOKEN=$(echo "$CI_TOKEN_RESPONSE" | jq -r '.token')
echo "CI service token: ${CI_SERVICE_TOKEN:0:20}..."
```

### 14.2 — Upload `hwops-banner` v1.0.0

```bash
BASE_URL="$APP_URL" \
ARTIFACT_NAME=hwops-banner \
ARTIFACT_VERSION=1.0.0 \
ARTIFACT_TYPE=app_bundle \
INPUT_DIR=/tmp/hwops-artifacts/banner-v1 \
CI_SERVICE_TOKEN="$CI_SERVICE_TOKEN" \
./scripts/ci-upload-artifact.sh

echo "hwops-banner v1.0.0 uploaded."
```

### 14.3 — Upload `hwops-config` v1.0.0

```bash
BASE_URL="$APP_URL" \
ARTIFACT_NAME=hwops-config \
ARTIFACT_VERSION=1.0.0 \
ARTIFACT_TYPE=config_bundle \
INPUT_DIR=/tmp/hwops-artifacts/config-v1 \
CI_SERVICE_TOKEN="$CI_SERVICE_TOKEN" \
./scripts/ci-upload-artifact.sh

echo "hwops-config v1.0.0 uploaded."
```

### 14.4 — Verify artifacts in UI

Navigate to **Artifacts**. Confirm:
- [ ] `hwops-banner` v1.0.0 — status: active
- [ ] `hwops-config` v1.0.0 — status: active

---

## Step 15 — Assign Desired State

Assign both artifacts to the device's desired state. The agent will apply them on its next check-in (default 30s).

```bash
# Get artifact IDs
BANNER_ID=$(curl -sf "${APP_URL}/api/v1/artifacts?name=hwops-banner&version=1.0.0" \
  -H "Authorization: Bearer $AUTH_TOKEN" | jq -r '.items[0].artifactId')

CONFIG_ID=$(curl -sf "${APP_URL}/api/v1/artifacts?name=hwops-config&version=1.0.0" \
  -H "Authorization: Bearer $AUTH_TOKEN" | jq -r '.items[0].artifactId')

echo "Banner artifact ID: $BANNER_ID"
echo "Config artifact ID: $CONFIG_ID"

# Set desired state on the device
curl -sf -X PUT "${APP_URL}/api/v1/devices/${DEVICE_ID}/desired" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"components\": [
      {
        \"name\": \"banner\",
        \"artifactId\": \"${BANNER_ID}\"
      },
      {
        \"name\": \"app-config\",
        \"artifactId\": \"${CONFIG_ID}\"
      }
    ]
  }" | jq .

echo "Desired state set."
```

---

## Step 16 — Verify Artifact Apply

### 16.1 — Wait for convergence via API

```bash
# Wait up to 3 min for both components to report applied
for i in $(seq 1 36); do
  DEVICE_STATE=$(curl -sf "${APP_URL}/api/v1/devices/${DEVICE_ID}" \
    -H "Authorization: Bearer $AUTH_TOKEN")

  BANNER_STATUS=$(echo "$DEVICE_STATE" | jq -r \
    '.currentState.components[] | select(.name=="banner") | .applyStatus // empty')
  CONFIG_STATUS=$(echo "$DEVICE_STATE" | jq -r \
    '.currentState.components[] | select(.name=="app-config") | .applyStatus // empty')

  echo "[$i/36] banner=$BANNER_STATUS  config=$CONFIG_STATUS"

  [ "$BANNER_STATUS" = "applied" ] && [ "$CONFIG_STATUS" = "applied" ] && break
  sleep 5
done

[ "$BANNER_STATUS" != "applied" ] && echo "ERROR: banner did not apply"
[ "$CONFIG_STATUS" != "applied" ] && echo "ERROR: app-config did not apply"
```

### 16.2 — Confirm files exist on EC2 (visual confirmation)

```bash
echo "--- /etc/hwops-banner.txt ---"
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" "cat /etc/hwops-banner.txt"

echo ""
echo "--- /opt/parcel/app-config.json ---"
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" "cat /opt/parcel/app-config.json"
```

**Expected output:**
```
--- /etc/hwops-banner.txt ---
== Parcel Managed Device ==
Artifact  : hwops-banner
Version   : 1.0.0
Managed by: Parcel Control Plane

--- /opt/parcel/app-config.json ---
{
  "artifact": "hwops-config",
  "version": "1.0.0",
  "telemetry": {
    "enabled": true,
    "endpoint": "https://metrics.parcel.internal"
  },
  "logLevel": "info"
}
```

### 16.3 — UI confirmation

Navigate to **Devices → [your device] → State**:
- [ ] Desired: `banner v1.0.0`, `app-config v1.0.0`
- [ ] Current: `banner v1.0.0`, `app-config v1.0.0`
- [ ] Apply status: `applied` for both components (green)
- [ ] Last apply timestamp is recent (within the last few minutes)

---

## Step 17 — Artifact Upgrade Test

Upload `hwops-banner` v2.0.0 and update the desired state. Confirm the file content changes on disk.

### 17.1 — Upload v2

```bash
BASE_URL="$APP_URL" \
ARTIFACT_NAME=hwops-banner \
ARTIFACT_VERSION=2.0.0 \
ARTIFACT_TYPE=app_bundle \
INPUT_DIR=/tmp/hwops-artifacts/banner-v2 \
CI_SERVICE_TOKEN="$CI_SERVICE_TOKEN" \
./scripts/ci-upload-artifact.sh

BANNER_V2_ID=$(curl -sf "${APP_URL}/api/v1/artifacts?name=hwops-banner&version=2.0.0" \
  -H "Authorization: Bearer $AUTH_TOKEN" | jq -r '.items[0].artifactId')

echo "hwops-banner v2.0.0 ID: $BANNER_V2_ID"
```

### 17.2 — Update desired state

```bash
curl -sf -X PUT "${APP_URL}/api/v1/devices/${DEVICE_ID}/desired" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"components\": [
      {
        \"name\": \"banner\",
        \"artifactId\": \"${BANNER_V2_ID}\"
      },
      {
        \"name\": \"app-config\",
        \"artifactId\": \"${CONFIG_ID}\"
      }
    ]
  }" | jq .
```

### 17.3 — Wait and confirm

```bash
sleep 60  # allow agent check-in cycle

echo "--- /etc/hwops-banner.txt after upgrade ---"
ssh -i "$EC2_KEY_PATH" "ec2-user@${AGENT_HOST}" "cat /etc/hwops-banner.txt"
```

**Expected:**
```
== Parcel Managed Device ==
Artifact  : hwops-banner
Version   : 2.0.0  *** UPGRADED ***
Managed by: Parcel Control Plane
```

**UI:** Device state shows `banner v2.0.0` as both desired and current.

---

## Step 18 — OIDC SSO Smoke Test (optional)

If you have an OIDC provider available (Okta trial, Google Workspace, etc.):

1. Set `AUTH_OIDC_ISSUER`, `AUTH_OIDC_CLIENT_ID`, `AUTH_OIDC_CLIENT_SECRET`, `AUTH_OIDC_REDIRECT_URL`, and `AUTH_OIDC_ROLE_MAP` in the ECS task definition (via `control_plane_secret_arns` or `control_plane_env`).
2. Redeploy: `./scripts/aws-customer.sh deploy --service control-plane ...`
3. Visit `https://${APP_HOST}` — confirm "Sign in with SSO" button appears.
4. Click the SSO button — confirm IdP redirect, login, and return to the dashboard.
5. Check audit events: `GET /api/v1/audit?action=auth.oidc.login` should show one entry.

---

## Step 19 — Switch Device mTLS to Verify Mode

Once the agent is enrolled and actively checking in, harden the device listener.

```bash
# Update terraform.tfvars
sed -i 's/device_mtls_mode = "passthrough"/device_mtls_mode = "verify"/' \
  ".aws-customers/${CUSTOMER}/${ENV}/terraform.tfvars"

# Re-apply (only the ALB listener changes)
./scripts/aws-customer.sh apply \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  --auto-approve

# Verify device stays active (agent should already have mTLS cert from enrollment)
sleep 60
DEVICE_STATUS=$(curl -sf "${APP_URL}/api/v1/devices/${DEVICE_ID}" \
  -H "Authorization: Bearer $AUTH_TOKEN" | jq -r '.status')
echo "Device status after mTLS enforce: $DEVICE_STATUS"
# Expected: active
```

Re-run the deployment gate after this change:
```bash
./scripts/aws-hardening-check.sh deployment \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  --alb-dns "$ALB_DNS" \
  --alarm-prefix "${CUSTOMER}-${ENV}" \
  2>&1 | tee /tmp/hwops-evidence/05-deployment-gate-verify-mode.txt
```

**Expected:** All checks pass, including `[PASS] Device listener (8443) in verify mode`.

---

## Step 20 — Collect Evidence

All artifacts for the release record:

```bash
ls -la /tmp/hwops-evidence/
# 00-config-gate.txt
# 01-plan.txt
# 02-apply.txt
# 03-outputs.txt
# 04-deployment-gate.txt
# 05-deployment-gate-verify-mode.txt
```

Additional manual captures:
- [ ] Screenshot: Control plane login page and dashboard
- [ ] Screenshot: Devices page showing 1 active device
- [ ] Screenshot: Device state page showing both components applied
- [ ] Screenshot: Artifacts page showing all 3 uploaded artifacts
- [ ] Screenshot: Audit log showing `auth.login`, `device.enroll`, `artifact.apply` events
- [ ] Screenshot: CloudWatch alarms dashboard (all in OK state)
- [ ] Terminal output: `cat /etc/hwops-banner.txt` after v2 upgrade

---

## Teardown

When testing is complete, destroy all AWS resources to avoid charges.

```bash
# Remove EC2 agent host
aws ec2 terminate-instances \
  --instance-ids "$INSTANCE_ID" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

# Destroy Terraform stack
./scripts/aws-customer.sh destroy \
  --customer "$CUSTOMER" --env "$ENV" \
  --region "$AWS_REGION" --profile "$AWS_PROFILE" \
  --confirm-destroy

# Delete Secrets Manager secrets
for SECRET in auth-jwt-secret auth-bootstrap-password database-url; do
  aws secretsmanager delete-secret \
    --secret-id "parcel/${CUSTOMER}/${ENV}/${SECRET}" \
    --force-delete-without-recovery \
    --region "$AWS_REGION" --profile "$AWS_PROFILE"
done

# Delete S3 bucket contents and bucket
aws s3 rb "s3://${MTLS_BUCKET}" --force \
  --region "$AWS_REGION" --profile "$AWS_PROFILE"

echo "Teardown complete."
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Control plane health check fails after apply | DATABASE_URL secret has wrong connection string | Update secret in Secrets Manager → force ECS redeploy |
| Agent never submits enrollment request | `CONTROL_PLANE_URL` wrong or TLS error | Check agent logs: `journalctl -u parcel-agent -n 50`; verify CA cert path |
| Enrollment approved but no device.crt | Control plane CA key not configured | Verify `CA_CERT_PATH` and `CA_KEY_PATH` env vars in ECS task definition |
| Artifact apply status stuck at `pending` | Agent not checking in, or desired state not set correctly | Check device last-seen timestamp; re-check desired state API response |
| `cat /etc/hwops-banner.txt` — file not found | `plan.yaml` step failed | Check agent logs for apply error; verify file.copy paths in plan.yaml |
| Config gate fails on DATABASE_URL | `database_url_secret_arn` not set or DATABASE_URL still in `control_plane_env` | Set `database_url_secret_arn` in tfvars; remove `DATABASE_URL` from `control_plane_env` |
| Deployment gate fails on ECS Exec | `enable_execute_command` set to true somewhere | Check `modules/ecs` call in `customer_stack/main.tf` and tfvars |
| Deployment gate fails on alarms | Alarm prefix mismatch | Check `--alarm-prefix` matches `name_prefix` used in Terraform |
