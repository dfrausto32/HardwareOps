#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

TF_DIR=${TF_DIR:-"$BASE_DIR/deploy/aws/terraform"}
WORK_DIR=${WORK_DIR:-"$BASE_DIR/.aws-customers"}
STATE_KEY_PREFIX=${STATE_KEY_PREFIX:-customers}

COMMAND=${1:-help}
if [ $# -gt 0 ]; then
  shift
fi

CUSTOMER=""
ENVIRONMENT="prod"
AWS_REGION=""
AWS_PROFILE=""
OWNER="platform"

STATE_BUCKET=""
STATE_LOCK_TABLE=""
STATE_KMS_KEY_ID=""

CUSTOMER_DOMAIN=""
APP_HOST=""
DEVICES_HOST=""
ROUTE53_ZONE_ID=""
ACM_CERT_ARN=""

DEVICE_MTLS_BUCKET=""
DEVICE_MTLS_KEY="mtls/device-ca-bundle.pem"
DEVICE_MTLS_OBJECT_VERSION=""
DEVICE_MTLS_MODE="passthrough"

CONTROL_PLANE_IMAGE=""
GATEWAY_IMAGE=""
ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID=""
DB_MASTER_PASSWORD=""
DB_MANAGE_MASTER_USER_PASSWORD=0
AUTH_BOOTSTRAP_EMAIL=""
AUTH_BOOTSTRAP_PASSWORD=""

ENABLE_DEMO_FLEET=0
DEMO_AGENT_COUNT=3
DEMO_AGENT_IMAGE=""
DEMO_AGENT_CPU=256
DEMO_AGENT_MEMORY=512
DEMO_AGENT_CHECKIN_INTERVAL_SEC=5
DEMO_BOOTSTRAP_EMAIL=""
DEMO_BOOTSTRAP_PASSWORD=""

VPC_OCTET="40"
TF_AUTO_APPROVE=0
FORCE=0
CONFIRM_DESTROY=0
SERVICE="all"
EXTRA_ARGS=()

usage() {
  cat <<'EOF'
Usage:
  scripts/aws-customer.sh <command> [options] [-- <terraform args>]

Commands:
  requirements     Print required account/customer info.
  bootstrap-state  Create/verify Terraform state S3 bucket + DynamoDB lock table.
  init             Generate per-customer backend.hcl + terraform.tfvars.
  plan             Terraform plan for a customer stack.
  apply            Terraform apply for a customer stack.
  destroy          Terraform destroy for a customer stack (requires --confirm-destroy).
  output           Show Terraform outputs for a customer stack.
  status           Show ECS service status for a customer stack.
  deploy           Force ECS redeploy (control-plane, gateway, or all).
  help             Show this message.

Common options:
  --customer <slug>          Customer ID (for example acme)
  --env <dev|staging|prod>   Environment (default: prod)
  --region <aws-region>      AWS region (for example us-east-1)
  --profile <aws-profile>    AWS CLI profile
  --work-dir <path>          Customer config output directory (default: ./.aws-customers)
  --auto-approve             For apply command
  --force                    Overwrite generated files for init command

State options (bootstrap-state/init):
  --state-bucket <name>      S3 bucket for Terraform state
  --state-lock-table <name>  DynamoDB table for Terraform state locking
  --state-kms-key-id <arn>   Optional KMS key for S3 default encryption
  --state-key-prefix <path>  State key prefix (default: customers)

Customer setup options (init):
  --owner <name>             Owner tag (default: platform)
  --customer-domain <fqdn>   Base domain, derives hosts if not set
  --app-host <fqdn>          App host (default from customer-domain: app.<domain>)
  --devices-host <fqdn>      Device host (default from customer-domain: agent.<domain>)
  --route53-zone-id <id>     Route53 hosted zone ID
  --acm-cert-arn <arn>       ACM certificate ARN for ALB listeners
  --device-mtls-bucket <s3>  S3 bucket with device trust bundle
  --device-mtls-key <key>    S3 key for trust bundle (default: mtls/device-ca-bundle.pem)
  --device-mtls-version <v>  Optional object version for trust bundle
  --device-mtls-mode <mode>  passthrough|verify (default: passthrough)
  --control-plane-image <uri>  Control-plane image URI
  --gateway-image <uri>        Gateway image URI
  --artifact-pull-credentials-secret-id <id-or-arn>
                              Optional Secrets Manager secret ID/ARN for pull adapter credentials
  --auth-bootstrap-email <email>    Local auth bootstrap admin email (default generated)
  --auth-bootstrap-password <pwd>   Local auth bootstrap admin password (default random)
  --enable-demo-fleet               Enable ECS demo agent fleet
  --demo-agent-count <n>            Demo agent service count (default: 3)
  --demo-agent-image <uri>          Demo agent image URI (required when demo fleet enabled)
  --demo-agent-cpu <units>          Demo agent task CPU (default: 256)
  --demo-agent-memory <MiB>         Demo agent task memory (default: 512)
  --demo-agent-checkin-interval-sec <sec>  Demo agent check-in interval (default: 5)
  --demo-bootstrap-email <email>    Demo agent enrollment email override
  --demo-bootstrap-password <pwd>   Demo agent enrollment password override
  --vpc-octet <0-255>          VPC 2nd octet seed (default: 40)
  --db-master-password <pwd>   DB password for non-managed mode (auto-generated if omitted)
  --db-managed-password        Use RDS managed DB password mode

Deploy options (deploy):
  --service <all|control-plane|gateway>  Service to redeploy (default: all)

Examples:
  scripts/aws-customer.sh requirements
  scripts/aws-customer.sh bootstrap-state --region us-east-1 --state-bucket hwops-tf-state --state-lock-table hwops-tf-locks
  scripts/aws-customer.sh init --customer acme --env prod --region us-east-1 --state-bucket hwops-tf-state \
    --state-lock-table hwops-tf-locks --customer-domain acme.example.com --route53-zone-id Z12345 \
    --acm-cert-arn arn:aws:acm:... --device-mtls-bucket acme-security-assets \
    --control-plane-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-control-plane:20260213 \
    --gateway-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-gateway:20260213 \
    --artifact-pull-credentials-secret-id arn:aws:secretsmanager:us-east-1:111122223333:secret:hardwareops/acme/prod/artifact-pull-credentials-AbCdEf
  scripts/aws-demo-image.sh
  scripts/aws-customer.sh init --customer acme --env dev --region us-east-1 --state-bucket hwops-tf-state \
    --state-lock-table hwops-tf-locks --customer-domain acme.example.com --route53-zone-id Z12345 \
    --acm-cert-arn arn:aws:acm:... --control-plane-image ... --gateway-image ... \
    --enable-demo-fleet --demo-agent-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-demo-agent:latest
  scripts/aws-customer.sh plan --customer acme --env prod --region us-east-1
  scripts/aws-customer.sh apply --customer acme --env prod --region us-east-1 --auto-approve
EOF
}

requirements() {
  cat <<'EOF'
Required inputs to deploy a customer stack:

AWS account/platform:
  1) AWS account ID and region
  2) AWS CLI credentials/profile with permissions for:
     - VPC, EC2 networking, ALB, ECS, IAM, RDS, S3, Route53, CloudWatch, DynamoDB
  3) Terraform remote state resources:
     - S3 bucket name for state
     - DynamoDB table name for lock
     - Optional KMS key ARN for state bucket encryption

Customer-specific:
  4) Customer slug (for names/tags), and environment (dev/staging/prod)
  5) DNS hosted zone ID in Route53
  6) FQDNs:
     - app host (for operators)
     - devices host (should use agent.* prefix with current routing)
  7) ACM certificate ARN for these hosts
  8) ECR image URIs:
     - control-plane image
     - gateway image
  9) Device mTLS trust store S3 location:
     - bucket + key (+ optional object version)
 10) Device CA/key operational plan:
     - where device CA private key is stored
     - how trust bundle updates are promoted
 11) Optional artifact pull credentials secret:
     - Secrets Manager secret ID/ARN containing JSON credential map for pull adapters
     - Example secret payload:
       {
         "artifactory-demo": {
           "username": "admin",
           "password": "password"
         }
       }
 12) Optional demo fleet setup:
     - demo agent image URI in ECR
     - bootstrap credentials for demo enrollment token creation

Networking defaults this script generates:
  - VPC: 10.<vpc-octet>.0.0/16
  - 3 public subnets and 3 private subnets across <region>a/<region>b/<region>c
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

require_value() {
  local name=$1
  local value=$2
  if [ -z "$value" ]; then
    die "$name is required."
  fi
}

customer_dir() {
  printf '%s/%s/%s' "$WORK_DIR" "$CUSTOMER" "$ENVIRONMENT"
}

backend_file() {
  printf '%s/backend.hcl' "$(customer_dir)"
}

tfvars_file() {
  printf '%s/terraform.tfvars' "$(customer_dir)"
}

env_tf_dir() {
  printf '%s/envs/%s' "$TF_DIR" "$ENVIRONMENT"
}

aws_cli() {
  local cmd=(aws)
  if [ -n "$AWS_PROFILE" ]; then
    cmd+=(--profile "$AWS_PROFILE")
  fi
  if [ -n "$AWS_REGION" ]; then
    cmd+=(--region "$AWS_REGION")
  fi
  cmd+=("$@")
  "${cmd[@]}"
}

tf_cmd() {
  local env_args=()
  if [ -n "$AWS_PROFILE" ]; then
    env_args+=("AWS_PROFILE=$AWS_PROFILE")
  fi
  if [ -n "$AWS_REGION" ]; then
    env_args+=("AWS_REGION=$AWS_REGION" "AWS_DEFAULT_REGION=$AWS_REGION")
  fi
  env "${env_args[@]}" terraform -chdir="$(env_tf_dir)" "$@"
}

ensure_customer_context() {
  require_value "--customer" "$CUSTOMER"
  require_value "--region" "$AWS_REGION"
  case "$ENVIRONMENT" in
    dev|staging|prod) ;;
    *) die "--env must be one of: dev, staging, prod" ;;
  esac
}

ensure_generated_files() {
  local bfile tfile
  bfile=$(backend_file)
  tfile=$(tfvars_file)
  [ -f "$bfile" ] || die "Missing backend file: $bfile (run init command first)."
  [ -f "$tfile" ] || die "Missing tfvars file: $tfile (run init command first)."
}

ensure_aws_tooling() {
  command -v aws >/dev/null 2>&1 || die "aws CLI not found."
}

ensure_terraform_tooling() {
  command -v terraform >/dev/null 2>&1 || die "terraform not found."
}

bootstrap_state() {
  ensure_aws_tooling
  require_value "--region" "$AWS_REGION"
  require_value "--state-bucket" "$STATE_BUCKET"
  require_value "--state-lock-table" "$STATE_LOCK_TABLE"

  log "Ensuring Terraform state bucket exists: $STATE_BUCKET"
  if ! aws_cli s3api head-bucket --bucket "$STATE_BUCKET" >/dev/null 2>&1; then
    if [ "$AWS_REGION" = "us-east-1" ]; then
      aws_cli s3api create-bucket --bucket "$STATE_BUCKET" >/dev/null
    else
      aws_cli s3api create-bucket \
        --bucket "$STATE_BUCKET" \
        --create-bucket-configuration "LocationConstraint=$AWS_REGION" >/dev/null
    fi
  fi

  aws_cli s3api put-public-access-block \
    --bucket "$STATE_BUCKET" \
    --public-access-block-configuration \
    "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true" >/dev/null

  aws_cli s3api put-bucket-versioning \
    --bucket "$STATE_BUCKET" \
    --versioning-configuration Status=Enabled >/dev/null

  if [ -n "$STATE_KMS_KEY_ID" ]; then
    aws_cli s3api put-bucket-encryption \
      --bucket "$STATE_BUCKET" \
      --server-side-encryption-configuration \
      "{\"Rules\":[{\"ApplyServerSideEncryptionByDefault\":{\"SSEAlgorithm\":\"aws:kms\",\"KMSMasterKeyID\":\"$STATE_KMS_KEY_ID\"}}]}" >/dev/null
  else
    aws_cli s3api put-bucket-encryption \
      --bucket "$STATE_BUCKET" \
      --server-side-encryption-configuration \
      '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}' >/dev/null
  fi

  log "Ensuring Terraform lock table exists: $STATE_LOCK_TABLE"
  if ! aws_cli dynamodb describe-table --table-name "$STATE_LOCK_TABLE" >/dev/null 2>&1; then
    aws_cli dynamodb create-table \
      --table-name "$STATE_LOCK_TABLE" \
      --attribute-definitions AttributeName=LockID,AttributeType=S \
      --key-schema AttributeName=LockID,KeyType=HASH \
      --billing-mode PAY_PER_REQUEST >/dev/null
    aws_cli dynamodb wait table-exists --table-name "$STATE_LOCK_TABLE"
  fi

  log "State bootstrap complete."
}

write_backend_file() {
  local path=$1
  cat > "$path" <<EOF
bucket         = "$STATE_BUCKET"
key            = "$STATE_KEY_PREFIX/$CUSTOMER/$ENVIRONMENT/terraform.tfstate"
region         = "$AWS_REGION"
dynamodb_table = "$STATE_LOCK_TABLE"
encrypt        = true
EOF
  if [ -n "$STATE_KMS_KEY_ID" ]; then
    printf 'kms_key_id     = "%s"\n' "$STATE_KMS_KEY_ID" >> "$path"
  fi
}

write_tfvars_file() {
  local path=$1
  local az_a="${AWS_REGION}a"
  local az_b="${AWS_REGION}b"
  local az_c="${AWS_REGION}c"
  local cp_count db_multi_az db_class
  local db_manage db_password auth_jwt_secret
  local auth_bootstrap_email auth_bootstrap_password
  local demo_fleet demo_bootstrap_email demo_bootstrap_password

  case "$ENVIRONMENT" in
    dev)
      cp_count=1
      db_multi_az=false
      db_class="db.t4g.medium"
      ;;
    staging)
      cp_count=2
      db_multi_az=true
      db_class="db.t4g.medium"
      ;;
    prod)
      cp_count=3
      db_multi_az=true
      db_class="db.r6g.large"
      ;;
    *)
      die "Invalid environment: $ENVIRONMENT"
      ;;
  esac

  db_manage=false
  if [ "$DB_MANAGE_MASTER_USER_PASSWORD" = "1" ]; then
    db_manage=true
  fi
  db_password="$DB_MASTER_PASSWORD"
  if [ -z "$db_password" ] && [ "$db_manage" = false ]; then
    db_password=$(openssl rand -hex 16)
  fi
  auth_jwt_secret=$(openssl rand -hex 32)
  auth_bootstrap_email="$AUTH_BOOTSTRAP_EMAIL"
  if [ -z "$auth_bootstrap_email" ]; then
    auth_bootstrap_email="admin+${CUSTOMER}-${ENVIRONMENT}@hardwareops.local"
  fi
  auth_bootstrap_password="$AUTH_BOOTSTRAP_PASSWORD"
  if [ -z "$auth_bootstrap_password" ]; then
    auth_bootstrap_password=$(openssl rand -hex 16)
  fi

  demo_fleet=false
  if [ "$ENABLE_DEMO_FLEET" = "1" ]; then
    demo_fleet=true
  fi
  demo_bootstrap_email="$DEMO_BOOTSTRAP_EMAIL"
  if [ -z "$demo_bootstrap_email" ]; then
    demo_bootstrap_email="$auth_bootstrap_email"
  fi
  demo_bootstrap_password="$DEMO_BOOTSTRAP_PASSWORD"
  if [ -z "$demo_bootstrap_password" ]; then
    demo_bootstrap_password="$auth_bootstrap_password"
  fi

  cat > "$path" <<EOF
aws_region    = "$AWS_REGION"
customer_slug = "$CUSTOMER"
environment   = "$ENVIRONMENT"
owner         = "$OWNER"

vpc_cidr = "10.$VPC_OCTET.0.0/16"
public_subnet_cidrs = {
  "$az_a" = "10.$VPC_OCTET.0.0/20"
  "$az_b" = "10.$VPC_OCTET.16.0/20"
  "$az_c" = "10.$VPC_OCTET.32.0/20"
}
private_subnet_cidrs = {
  "$az_a" = "10.$VPC_OCTET.64.0/20"
  "$az_b" = "10.$VPC_OCTET.80.0/20"
  "$az_c" = "10.$VPC_OCTET.96.0/20"
}

route53_zone_id = "$ROUTE53_ZONE_ID"
app_host        = "$APP_HOST"
devices_host    = "$DEVICES_HOST"
acm_certificate_arn = "$ACM_CERT_ARN"

device_mtls_bucket = "$DEVICE_MTLS_BUCKET"
device_mtls_key    = "$DEVICE_MTLS_KEY"
device_mtls_mode   = "$DEVICE_MTLS_MODE"
EOF
  if [ -n "$DEVICE_MTLS_OBJECT_VERSION" ]; then
    printf 'device_mtls_object_version = "%s"\n' "$DEVICE_MTLS_OBJECT_VERSION" >> "$path"
  fi
  cat >> "$path" <<EOF

control_plane_image = "$CONTROL_PLANE_IMAGE"
gateway_image       = "$GATEWAY_IMAGE"

db_instance_class           = "$db_class"
db_multi_az                 = $db_multi_az
db_manage_master_user_password = $db_manage
db_master_password          = "$db_password"
control_plane_desired_count = $cp_count
gateway_desired_count       = $cp_count

control_plane_env = {
  AUTH_MODE               = "local"
  AUTH_JWT_SECRET         = "$auth_jwt_secret"
  AUTH_BOOTSTRAP_EMAIL    = "$auth_bootstrap_email"
  AUTH_BOOTSTRAP_PASSWORD = "$auth_bootstrap_password"
}

gateway_env = {}
enable_demo_fleet = $demo_fleet
demo_agent_count  = $DEMO_AGENT_COUNT
demo_agent_image  = "$DEMO_AGENT_IMAGE"
demo_agent_cpu    = $DEMO_AGENT_CPU
demo_agent_memory = $DEMO_AGENT_MEMORY
demo_agent_checkin_interval_sec = $DEMO_AGENT_CHECKIN_INTERVAL_SEC
demo_bootstrap_email = "$demo_bootstrap_email"
demo_bootstrap_password = "$demo_bootstrap_password"

# Set secret ARNs when ready (recommended for production):
# control_plane_secret_arns = {
#   DATABASE_URL       = "arn:aws:secretsmanager:REGION:ACCOUNT:secret:..."
#   MAINTENANCE_TOKEN  = "arn:aws:secretsmanager:REGION:ACCOUNT:secret:..."
#   AUTH_JWT_SECRET    = "arn:aws:secretsmanager:REGION:ACCOUNT:secret:..."
# }
control_plane_secret_arns = {}
gateway_secret_arns = {}
EOF
  if [ -n "$ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID" ]; then
    printf 'artifact_pull_credentials_aws_secret_id = "%s"\n' "$ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID" >> "$path"
  fi
}

init_customer() {
  ensure_customer_context

  require_value "--state-bucket" "$STATE_BUCKET"
  require_value "--state-lock-table" "$STATE_LOCK_TABLE"
  require_value "--route53-zone-id" "$ROUTE53_ZONE_ID"
  require_value "--acm-cert-arn" "$ACM_CERT_ARN"
  require_value "--control-plane-image" "$CONTROL_PLANE_IMAGE"
  require_value "--gateway-image" "$GATEWAY_IMAGE"

  if [ -z "$APP_HOST" ] || [ -z "$DEVICES_HOST" ]; then
    require_value "--customer-domain (or explicit --app-host/--devices-host)" "$CUSTOMER_DOMAIN"
    if [ -z "$APP_HOST" ]; then
      APP_HOST="app.$CUSTOMER_DOMAIN"
    fi
    if [ -z "$DEVICES_HOST" ]; then
      DEVICES_HOST="agent.$CUSTOMER_DOMAIN"
    fi
  fi

  if [ -z "$DEVICE_MTLS_BUCKET" ]; then
    DEVICE_MTLS_BUCKET="hardwareops-$CUSTOMER-$ENVIRONMENT-security-assets"
  fi

  if ! [[ "$VPC_OCTET" =~ ^[0-9]+$ ]] || [ "$VPC_OCTET" -lt 0 ] || [ "$VPC_OCTET" -gt 255 ]; then
    die "--vpc-octet must be an integer between 0 and 255"
  fi

  case "$DEVICE_MTLS_MODE" in
    passthrough|verify) ;;
    *) die "--device-mtls-mode must be passthrough or verify" ;;
  esac

  if ! [[ "$DEMO_AGENT_COUNT" =~ ^[0-9]+$ ]] || [ "$DEMO_AGENT_COUNT" -lt 1 ]; then
    die "--demo-agent-count must be a positive integer"
  fi
  if ! [[ "$DEMO_AGENT_CPU" =~ ^[0-9]+$ ]] || [ "$DEMO_AGENT_CPU" -lt 128 ]; then
    die "--demo-agent-cpu must be a numeric value (>= 128)"
  fi
  if ! [[ "$DEMO_AGENT_MEMORY" =~ ^[0-9]+$ ]] || [ "$DEMO_AGENT_MEMORY" -lt 128 ]; then
    die "--demo-agent-memory must be a numeric value (>= 128)"
  fi
  if ! [[ "$DEMO_AGENT_CHECKIN_INTERVAL_SEC" =~ ^[0-9]+$ ]] || [ "$DEMO_AGENT_CHECKIN_INTERVAL_SEC" -lt 1 ]; then
    die "--demo-agent-checkin-interval-sec must be a positive integer"
  fi
  if [ "$ENABLE_DEMO_FLEET" = "1" ]; then
    require_value "--demo-agent-image" "$DEMO_AGENT_IMAGE"
  fi

  command -v openssl >/dev/null 2>&1 || die "openssl is required to generate runtime secrets."

  local cdir bfile tfile
  cdir=$(customer_dir)
  bfile=$(backend_file)
  tfile=$(tfvars_file)
  mkdir -p "$cdir"

  if [ -f "$bfile" ] && [ "$FORCE" != "1" ]; then
    die "$bfile already exists. Use --force to overwrite."
  fi
  if [ -f "$tfile" ] && [ "$FORCE" != "1" ]; then
    die "$tfile already exists. Use --force to overwrite."
  fi

  write_backend_file "$bfile"
  write_tfvars_file "$tfile"

  cat > "$cdir/README.txt" <<EOF
Customer: $CUSTOMER
Environment: $ENVIRONMENT
Region: $AWS_REGION

Terraform files:
  $bfile
  $tfile

Bootstrap login:
  email: ${AUTH_BOOTSTRAP_EMAIL:-admin+${CUSTOMER}-${ENVIRONMENT}@hardwareops.local}
  password: ${AUTH_BOOTSTRAP_PASSWORD:-generated in terraform.tfvars}

Demo fleet:
  enabled: $([ "$ENABLE_DEMO_FLEET" = "1" ] && echo "yes" || echo "no")
  demo image: ${DEMO_AGENT_IMAGE:-"(not set)"}

Run:
  scripts/aws-customer.sh plan --customer $CUSTOMER --env $ENVIRONMENT --region $AWS_REGION
  scripts/aws-customer.sh apply --customer $CUSTOMER --env $ENVIRONMENT --region $AWS_REGION --auto-approve
EOF

  log "Generated:"
  log "  $bfile"
  log "  $tfile"
  log "  $cdir/README.txt"
}

tf_init_with_backend() {
  ensure_terraform_tooling
  ensure_customer_context
  ensure_generated_files
  [ -d "$(env_tf_dir)" ] || die "Terraform env dir not found: $(env_tf_dir)"
  tf_cmd init -backend-config "$(backend_file)" -input=false >/tmp/tf-init-"$CUSTOMER"-"$ENVIRONMENT".log
}

run_plan() {
  tf_init_with_backend
  tf_cmd plan -var-file "$(tfvars_file)" "${EXTRA_ARGS[@]}"
}

run_apply() {
  tf_init_with_backend
  local args=(apply -var-file "$(tfvars_file)")
  if [ "$TF_AUTO_APPROVE" = "1" ]; then
    args+=(--auto-approve)
  fi
  args+=("${EXTRA_ARGS[@]}")
  tf_cmd "${args[@]}"
}

run_destroy() {
  [ "$CONFIRM_DESTROY" = "1" ] || die "Destroy requires --confirm-destroy."
  tf_init_with_backend
  local args=(destroy -var-file "$(tfvars_file)")
  if [ "$TF_AUTO_APPROVE" = "1" ]; then
    args+=(--auto-approve)
  fi
  args+=("${EXTRA_ARGS[@]}")
  tf_cmd "${args[@]}"
}

run_output() {
  tf_init_with_backend
  if [ "${#EXTRA_ARGS[@]}" -gt 0 ]; then
    tf_cmd output "${EXTRA_ARGS[@]}"
  else
    tf_cmd output
  fi
}

run_status() {
  ensure_aws_tooling
  tf_init_with_backend
  local cluster cp_service gw_service app_url devices_url demo_json
  local -a services
  cluster=$(tf_cmd output -raw ecs_cluster_name)
  cp_service=$(tf_cmd output -raw ecs_control_plane_service_name)
  gw_service=$(tf_cmd output -raw ecs_gateway_service_name)
  app_url=$(tf_cmd output -raw app_url)
  devices_url=$(tf_cmd output -raw devices_url)
  services=("$cp_service" "$gw_service")
  demo_json=$(tf_cmd output -json ecs_demo_agent_service_names 2>/dev/null || echo "[]")
  while IFS= read -r service; do
    [ -n "$service" ] || continue
    services+=("$service")
  done < <(python3 - <<'PY' "$demo_json"
import json
import sys
for name in json.loads(sys.argv[1]):
    print(name)
PY
)

  log "Cluster: $cluster"
  log "App URL: $app_url"
  log "Devices URL: $devices_url"
  log
  aws_cli ecs describe-services \
    --cluster "$cluster" \
    --services "${services[@]}" \
    --query "services[].{service:serviceName,status:status,desired:desiredCount,running:runningCount,pending:pendingCount}" \
    --output table
}

run_deploy() {
  ensure_aws_tooling
  tf_init_with_backend
  local cluster cp_service gw_service
  cluster=$(tf_cmd output -raw ecs_cluster_name)
  cp_service=$(tf_cmd output -raw ecs_control_plane_service_name)
  gw_service=$(tf_cmd output -raw ecs_gateway_service_name)

  case "$SERVICE" in
    all)
      log "Forcing new deployment for $cp_service and $gw_service"
      aws_cli ecs update-service --cluster "$cluster" --service "$cp_service" --force-new-deployment >/dev/null
      aws_cli ecs update-service --cluster "$cluster" --service "$gw_service" --force-new-deployment >/dev/null
      ;;
    control-plane)
      log "Forcing new deployment for $cp_service"
      aws_cli ecs update-service --cluster "$cluster" --service "$cp_service" --force-new-deployment >/dev/null
      ;;
    gateway)
      log "Forcing new deployment for $gw_service"
      aws_cli ecs update-service --cluster "$cluster" --service "$gw_service" --force-new-deployment >/dev/null
      ;;
    *)
      die "--service must be all, control-plane, or gateway"
      ;;
  esac
  log "Deploy request submitted."
}

while [ $# -gt 0 ]; do
  case "$1" in
    --customer) CUSTOMER=$2; shift 2 ;;
    --env) ENVIRONMENT=$2; shift 2 ;;
    --region) AWS_REGION=$2; shift 2 ;;
    --profile) AWS_PROFILE=$2; shift 2 ;;
    --owner) OWNER=$2; shift 2 ;;
    --work-dir) WORK_DIR=$2; shift 2 ;;
    --state-bucket) STATE_BUCKET=$2; shift 2 ;;
    --state-lock-table) STATE_LOCK_TABLE=$2; shift 2 ;;
    --state-kms-key-id) STATE_KMS_KEY_ID=$2; shift 2 ;;
    --state-key-prefix) STATE_KEY_PREFIX=$2; shift 2 ;;
    --customer-domain) CUSTOMER_DOMAIN=$2; shift 2 ;;
    --app-host) APP_HOST=$2; shift 2 ;;
    --devices-host) DEVICES_HOST=$2; shift 2 ;;
    --route53-zone-id) ROUTE53_ZONE_ID=$2; shift 2 ;;
    --acm-cert-arn) ACM_CERT_ARN=$2; shift 2 ;;
    --device-mtls-bucket) DEVICE_MTLS_BUCKET=$2; shift 2 ;;
    --device-mtls-key) DEVICE_MTLS_KEY=$2; shift 2 ;;
    --device-mtls-version) DEVICE_MTLS_OBJECT_VERSION=$2; shift 2 ;;
    --device-mtls-mode) DEVICE_MTLS_MODE=$2; shift 2 ;;
    --control-plane-image) CONTROL_PLANE_IMAGE=$2; shift 2 ;;
    --gateway-image) GATEWAY_IMAGE=$2; shift 2 ;;
    --artifact-pull-credentials-secret-id) ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID=$2; shift 2 ;;
    --auth-bootstrap-email) AUTH_BOOTSTRAP_EMAIL=$2; shift 2 ;;
    --auth-bootstrap-password) AUTH_BOOTSTRAP_PASSWORD=$2; shift 2 ;;
    --enable-demo-fleet) ENABLE_DEMO_FLEET=1; shift ;;
    --demo-agent-count) DEMO_AGENT_COUNT=$2; shift 2 ;;
    --demo-agent-image) DEMO_AGENT_IMAGE=$2; shift 2 ;;
    --demo-agent-cpu) DEMO_AGENT_CPU=$2; shift 2 ;;
    --demo-agent-memory) DEMO_AGENT_MEMORY=$2; shift 2 ;;
    --demo-agent-checkin-interval-sec) DEMO_AGENT_CHECKIN_INTERVAL_SEC=$2; shift 2 ;;
    --demo-bootstrap-email) DEMO_BOOTSTRAP_EMAIL=$2; shift 2 ;;
    --demo-bootstrap-password) DEMO_BOOTSTRAP_PASSWORD=$2; shift 2 ;;
    --db-master-password) DB_MASTER_PASSWORD=$2; shift 2 ;;
    --db-managed-password) DB_MANAGE_MASTER_USER_PASSWORD=1; shift ;;
    --vpc-octet) VPC_OCTET=$2; shift 2 ;;
    --service) SERVICE=$2; shift 2 ;;
    --auto-approve) TF_AUTO_APPROVE=1; shift ;;
    --confirm-destroy) CONFIRM_DESTROY=1; shift ;;
    --force) FORCE=1; shift ;;
    --help|-h) COMMAND="help"; shift ;;
    --) shift; EXTRA_ARGS=("$@"); break ;;
    *) die "Unknown option: $1 (use help command for usage)." ;;
  esac
done

case "$COMMAND" in
  requirements) requirements ;;
  bootstrap-state) bootstrap_state ;;
  init) init_customer ;;
  plan) run_plan ;;
  apply) run_apply ;;
  destroy) run_destroy ;;
  output) run_output ;;
  status) run_status ;;
  deploy) run_deploy ;;
  help|*) usage ;;
esac
