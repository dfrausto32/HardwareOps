#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

TF_DIR=${TF_DIR:-"$BASE_DIR/deploy/aws/terraform"}
WORK_DIR=${WORK_DIR:-"$BASE_DIR/.aws-customers"}

COMMAND=${1:-help}
if [ $# -gt 0 ]; then
  shift
fi

CUSTOMER=""
ENVIRONMENT="prod"
AWS_REGION=""
AWS_PROFILE=""
TFVARS_PATH=""
ALARM_PREFIX=""
SNS_TOPIC_ARN=""

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

usage() {
  cat <<'EOF'
Usage:
  scripts/aws-hardening-check.sh <command> [options]

Commands:
  config        Validate repo + tfvars hardening readiness before apply.
  deployment    Validate live AWS deployment after apply.
  help          Show this message.

Common options:
  --customer <slug>        Customer ID (for example acme)
  --env <dev|staging|prod> Environment (default: prod)
  --region <aws-region>    AWS region (required for deployment)
  --profile <aws-profile>  AWS CLI profile
  --work-dir <path>        Customer config directory (default: ./.aws-customers)

Config options:
  --tfvars <path>          Explicit tfvars file to validate

Deployment options:
  --alarm-prefix <prefix>  Optional CloudWatch alarm name prefix to validate
  --sns-topic-arn <arn>    Optional SNS topic ARN required on all matching alarms

Examples:
  scripts/aws-hardening-check.sh config --customer acme --env prod
  scripts/aws-hardening-check.sh config --tfvars .aws-customers/acme/prod/terraform.tfvars
  scripts/aws-hardening-check.sh deployment --customer acme --env prod --region us-east-1
  scripts/aws-hardening-check.sh deployment --customer acme --env prod --region us-east-1 \
    --alarm-prefix hardwareops-acme-prod --sns-topic-arn arn:aws:sns:us-east-1:111122223333:hardwareops-acme-prod-security
EOF
}

log() {
  printf '%s\n' "$*"
}

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '[PASS] %s\n' "$*"
}

warn() {
  WARN_COUNT=$((WARN_COUNT + 1))
  printf '[WARN] %s\n' "$*"
}

fail_check() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '[FAIL] %s\n' "$*"
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

ensure_customer_context() {
  require_value "--customer" "$CUSTOMER"
  case "$ENVIRONMENT" in
    dev|staging|prod) ;;
    *) die "--env must be one of: dev, staging, prod" ;;
  esac
}

ensure_generated_files() {
  local bfile tfile
  bfile=$(backend_file)
  tfile=$(tfvars_file)
  [ -f "$bfile" ] || die "Missing backend file: $bfile"
  [ -f "$tfile" ] || die "Missing tfvars file: $tfile"
}

ensure_aws_tooling() {
  command -v aws >/dev/null 2>&1 || die "aws CLI not found."
}

ensure_terraform_tooling() {
  command -v terraform >/dev/null 2>&1 || die "terraform not found."
}

ensure_python_tooling() {
  command -v python3 >/dev/null 2>&1 || die "python3 not found."
}

ensure_rg_tooling() {
  command -v rg >/dev/null 2>&1 || die "rg not found."
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
  AWS_PAGER="" "${cmd[@]}"
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

tf_init_with_backend() {
  ensure_terraform_tooling
  ensure_customer_context
  ensure_generated_files
  [ -d "$(env_tf_dir)" ] || die "Terraform env dir not found: $(env_tf_dir)"
  tf_cmd init -backend-config "$(backend_file)" -input=false >/tmp/tf-hardening-init-"$CUSTOMER"-"$ENVIRONMENT".log
}

run_repo_readiness_checks() {
  ensure_rg_tooling

  if rg -q 'resource "aws_wafv2_web_acl"' "$TF_DIR"; then
    pass "Terraform defines WAF resources."
  else
    fail_check "Terraform does not define any aws_wafv2_web_acl resources under $TF_DIR."
  fi

  if rg -q 'resource "aws_cloudwatch_(metric|composite)_alarm"' "$TF_DIR"; then
    pass "Terraform defines CloudWatch alarm resources."
  else
    fail_check "Terraform does not define CloudWatch alarm resources under $TF_DIR."
  fi

  if rg -q 'variable "(app|device|devices)_ingress_cidrs"' "$TF_DIR"; then
    pass "Terraform exposes separate ingress CIDR inputs."
  else
    fail_check "Terraform still exposes only shared ingress CIDRs; prod hardening requires separate app and device ingress policy inputs."
  fi

  if rg -q 'DATABASE_URL[[:space:]]*=[[:space:]]*local\.database_url' "$TF_DIR/modules/customer_stack/main.tf"; then
    fail_check "customer_stack still injects DATABASE_URL through default_control_plane_env; prod tasks will expose a plaintext DB URL."
  else
    pass "customer_stack does not inject DATABASE_URL through default_control_plane_env."
  fi

  if rg -q 'enable_execute_command' "$TF_DIR/envs/$ENVIRONMENT/main.tf" "$TF_DIR/modules/customer_stack/main.tf" 2>/dev/null; then
    pass "Terraform exposes enable_execute_command above the ECS module."
  else
    fail_check "Terraform does not expose enable_execute_command above modules/ecs; ECS services inherit the module default."
  fi

  if python3 - "$TF_DIR/modules/ecs/variables.tf" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()
match = re.search(r'variable\s+"enable_execute_command"\s*\{([^}]*)\}', text, re.S)
if not match:
    raise SystemExit(1)
block = match.group(1)
raise SystemExit(0 if re.search(r'default\s*=\s*true\b', block) else 1)
PY
  then
    fail_check "modules/ecs defaults enable_execute_command to true."
  else
    pass "modules/ecs does not default enable_execute_command to true."
  fi
}

run_tfvars_checks() {
  ensure_python_tooling

  local tfvars_path=$1
  [ -f "$tfvars_path" ] || die "tfvars file not found: $tfvars_path"

  while IFS=$'\t' read -r level message; do
    [ -n "$level" ] || continue
    case "$level" in
      PASS) pass "$message" ;;
      WARN) warn "$message" ;;
      FAIL) fail_check "$message" ;;
      *) die "Unexpected check level from parser: $level" ;;
    esac
  done < <(python3 - "$tfvars_path" "$ENVIRONMENT" <<'PY'
import re
import sys

path = sys.argv[1]
environment = sys.argv[2]
text = open(path, encoding="utf-8").read()


def emit(level, message):
    print(f"{level}\t{message}")


def find_block(name):
    pattern = re.compile(rf'(^|\n)\s*{re.escape(name)}\s*=\s*\{{', re.M)
    match = pattern.search(text)
    if not match:
        return None
    start = match.end() - 1
    depth = 0
    i = start
    while i < len(text):
        ch = text[i]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return text[start + 1:i]
        i += 1
    return None


def block_keys(name):
    block = find_block(name)
    if block is None:
        return set()
    keys = set()
    for line in block.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line or "=" not in line:
            continue
        key = line.split("=", 1)[0].strip().strip('"')
        if key:
            keys.add(key)
    return keys


def find_scalar(name):
    match = re.search(rf'(^|\n)\s*{re.escape(name)}\s*=\s*(".*?"|\S+)', text, re.M)
    if not match:
        return None
    raw = match.group(2).strip()
    if raw.startswith('"') and raw.endswith('"'):
        return raw[1:-1]
    return raw


def find_list(name):
    pattern = re.compile(rf'(^|\n)\s*{re.escape(name)}\s*=\s*\[', re.M)
    match = pattern.search(text)
    if not match:
        return None
    start = match.end() - 1
    depth = 0
    i = start
    while i < len(text):
        ch = text[i]
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
            if depth == 0:
                inner = text[start + 1:i]
                return re.findall(r'"([^"]+)"', inner)
        i += 1
    return None


sensitive_keys = {"AUTH_JWT_SECRET", "AUTH_BOOTSTRAP_PASSWORD", "DATABASE_URL", "MAINTENANCE_TOKEN"}
required_secret_keys = {"AUTH_JWT_SECRET", "AUTH_BOOTSTRAP_PASSWORD", "DATABASE_URL"}

control_plane_env_keys = block_keys("control_plane_env")
gateway_env_keys = block_keys("gateway_env")
control_plane_secret_keys = block_keys("control_plane_secret_arns")
device_mtls_mode = find_scalar("device_mtls_mode")
ingress_cidrs = find_list("ingress_cidrs")

if environment != "prod":
    emit("WARN", f"tfvars validation is advisory for {environment}; prod is the hardening release gate.")

if device_mtls_mode == "verify":
    emit("PASS", "device_mtls_mode is set to verify.")
elif device_mtls_mode is None:
    emit("FAIL", "device_mtls_mode is not set; the scaffold default is not sufficient for final prod acceptance.")
else:
    emit("FAIL", f"device_mtls_mode is {device_mtls_mode}; final prod acceptance requires verify.")

plaintext_control_plane = sorted(control_plane_env_keys & sensitive_keys)
if plaintext_control_plane:
    emit("FAIL", "control_plane_env still carries plaintext sensitive keys: " + ", ".join(plaintext_control_plane))
else:
    emit("PASS", "control_plane_env does not declare plaintext sensitive keys.")

plaintext_gateway = sorted(gateway_env_keys & sensitive_keys)
if plaintext_gateway:
    emit("FAIL", "gateway_env still carries plaintext sensitive keys: " + ", ".join(plaintext_gateway))
else:
    emit("PASS", "gateway_env does not declare plaintext sensitive keys.")

missing_secret_arns = sorted(required_secret_keys - control_plane_secret_keys)
if missing_secret_arns:
    emit("FAIL", "control_plane_secret_arns is missing required prod secret keys: " + ", ".join(missing_secret_arns))
else:
    emit("PASS", "control_plane_secret_arns includes required prod secret keys.")

if ingress_cidrs is None:
    emit("WARN", "ingress_cidrs is not set; the env default remains 0.0.0.0/0 until Terraform ingress split lands.")
elif "0.0.0.0/0" in ingress_cidrs:
    emit("FAIL", "ingress_cidrs still includes 0.0.0.0/0; final prod acceptance requires a narrower operator policy on app ingress.")
else:
    emit("PASS", "ingress_cidrs does not include 0.0.0.0/0.")
PY
)
}

normalize_text_output() {
  local value=$1
  if [ "$value" = "None" ] || [ "$value" = "null" ]; then
    printf ''
  else
    printf '%s' "$value"
  fi
}

normalize_word_list() {
  printf '%s' "$1" | tr '\t\r\n' '   '
}

contains_word() {
  local haystack=$1
  local needle=$2
  case " $haystack " in
    *" $needle "*) return 0 ;;
    *) return 1 ;;
  esac
}

run_deployment_checks() {
  ensure_python_tooling
  ensure_aws_tooling
  require_value "--region" "$AWS_REGION"
  ensure_customer_context
  tf_init_with_backend

  local cluster cp_service gw_service alb_dns_name app_url devices_url
  cluster=$(tf_cmd output -raw ecs_cluster_name)
  cp_service=$(tf_cmd output -raw ecs_control_plane_service_name)
  gw_service=$(tf_cmd output -raw ecs_gateway_service_name)
  alb_dns_name=$(tf_cmd output -raw alb_dns_name)
  app_url=$(tf_cmd output -raw app_url)
  devices_url=$(tf_cmd output -raw devices_url)

  log "Cluster: $cluster"
  log "App URL: $app_url"
  log "Devices URL: $devices_url"

  local service task_def exec_enabled running desired
  for service in "$cp_service" "$gw_service"; do
    task_def=$(aws_cli ecs describe-services \
      --cluster "$cluster" \
      --services "$service" \
      --query 'services[0].taskDefinition' \
      --output text 2>/dev/null || true)
    exec_enabled=$(aws_cli ecs describe-services \
      --cluster "$cluster" \
      --services "$service" \
      --query 'services[0].enableExecuteCommand' \
      --output text 2>/dev/null || true)
    running=$(aws_cli ecs describe-services \
      --cluster "$cluster" \
      --services "$service" \
      --query 'services[0].runningCount' \
      --output text 2>/dev/null || true)
    desired=$(aws_cli ecs describe-services \
      --cluster "$cluster" \
      --services "$service" \
      --query 'services[0].desiredCount' \
      --output text 2>/dev/null || true)

    task_def=$(normalize_text_output "$task_def")
    exec_enabled=$(normalize_text_output "$exec_enabled")
    running=$(normalize_text_output "$running")
    desired=$(normalize_text_output "$desired")

    if [ -n "$task_def" ]; then
      pass "ECS service $service exists with task definition $task_def."
    else
      fail_check "ECS service $service could not be resolved from cluster $cluster."
      continue
    fi

    if [ "$exec_enabled" = "False" ]; then
      pass "ECS Exec is disabled on service $service."
    else
      fail_check "ECS Exec is enabled on service $service."
    fi

    if [ -n "$running" ] && [ -n "$desired" ] && [ "$running" -ge 1 ] && [ "$running" -le "$desired" ]; then
      pass "Service $service has running tasks ($running/$desired)."
    else
      warn "Service $service has an unexpected running/desired count ($running/$desired)."
    fi
  done

  local cp_task_def cp_env_names cp_secret_names gw_task_def gw_env_names
  cp_task_def=$(aws_cli ecs describe-services \
    --cluster "$cluster" \
    --services "$cp_service" \
    --query 'services[0].taskDefinition' \
    --output text)
  gw_task_def=$(aws_cli ecs describe-services \
    --cluster "$cluster" \
    --services "$gw_service" \
    --query 'services[0].taskDefinition' \
    --output text)

  cp_env_names=$(aws_cli ecs describe-task-definition \
    --task-definition "$cp_task_def" \
    --query 'taskDefinition.containerDefinitions[?name==`control-plane`] | [0].environment[].name' \
    --output text 2>/dev/null || true)
  cp_secret_names=$(aws_cli ecs describe-task-definition \
    --task-definition "$cp_task_def" \
    --query 'taskDefinition.containerDefinitions[?name==`control-plane`] | [0].secrets[].name' \
    --output text 2>/dev/null || true)
  gw_env_names=$(aws_cli ecs describe-task-definition \
    --task-definition "$gw_task_def" \
    --query 'taskDefinition.containerDefinitions[?name==`gateway`] | [0].environment[].name' \
    --output text 2>/dev/null || true)
  cp_env_names=$(normalize_word_list "$cp_env_names")
  cp_secret_names=$(normalize_word_list "$cp_secret_names")
  gw_env_names=$(normalize_word_list "$gw_env_names")

  local sensitive_name
  for sensitive_name in DATABASE_URL AUTH_JWT_SECRET AUTH_BOOTSTRAP_PASSWORD MAINTENANCE_TOKEN; do
    if contains_word "$cp_env_names" "$sensitive_name"; then
      fail_check "Control-plane task definition exposes plaintext environment key $sensitive_name."
    else
      pass "Control-plane task definition does not expose plaintext environment key $sensitive_name."
    fi
  done

  for sensitive_name in DATABASE_URL AUTH_JWT_SECRET AUTH_BOOTSTRAP_PASSWORD MAINTENANCE_TOKEN; do
    if contains_word "$gw_env_names" "$sensitive_name"; then
      fail_check "Gateway task definition exposes plaintext environment key $sensitive_name."
    fi
  done

  if contains_word "$cp_secret_names" "AUTH_JWT_SECRET" && contains_word "$cp_secret_names" "AUTH_BOOTSTRAP_PASSWORD"; then
    pass "Control-plane task definition uses Secrets Manager wiring for auth secrets."
  else
    fail_check "Control-plane task definition is missing Secrets Manager wiring for AUTH_JWT_SECRET and/or AUTH_BOOTSTRAP_PASSWORD."
  fi

  if contains_word "$cp_env_names" "ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID"; then
    pass "Control-plane resolver env exposes ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID."
  else
    warn "Control-plane resolver env does not expose ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID."
  fi

  local task_role_arn task_role_name attached_policies_json inline_policies_json
  task_role_arn=$(aws_cli ecs describe-task-definition \
    --task-definition "$cp_task_def" \
    --query 'taskDefinition.taskRoleArn' \
    --output text)
  task_role_name=${task_role_arn##*/}

  if [ -n "$task_role_name" ] && [ "$task_role_name" != "$task_role_arn" ]; then
    attached_policies_json=$(aws_cli iam list-attached-role-policies --role-name "$task_role_name" --output json)
    inline_policies_json=$(aws_cli iam list-role-policies --role-name "$task_role_name" --output json)

    while IFS=$'\t' read -r level message; do
      [ -n "$level" ] || continue
      case "$level" in
        PASS) pass "$message" ;;
        WARN) warn "$message" ;;
        FAIL) fail_check "$message" ;;
        *) die "Unexpected IAM check level: $level" ;;
      esac
    done < <(python3 - "$task_role_name" "$attached_policies_json" "$inline_policies_json" <<'PY'
import json
import sys

role_name = sys.argv[1]
attached = json.loads(sys.argv[2]).get("AttachedPolicies", [])
inline = json.loads(sys.argv[3]).get("PolicyNames", [])

aws_managed = [policy for policy in attached if policy.get("PolicyArn", "").startswith("arn:aws:iam::aws:policy/")]

def emit(level, message):
    print(f"{level}\t{message}")

if aws_managed:
    names = ", ".join(sorted(policy["PolicyName"] for policy in aws_managed))
    emit("FAIL", f"Task role {role_name} still carries AWS-managed policies: {names}")
else:
    emit("PASS", f"Task role {role_name} does not carry AWS-managed policies.")

if inline:
    emit("PASS", f"Task role {role_name} has inline policies: {', '.join(sorted(inline))}")
else:
    emit("WARN", f"Task role {role_name} has no inline policies; verify least-privilege is provided by customer-managed policies.")
PY
)
  else
    fail_check "Could not determine the control-plane task role ARN."
  fi

  local alb_arn alb_sg_id
  alb_arn=$(aws_cli elbv2 describe-load-balancers \
    --query "LoadBalancers[?DNSName=='$alb_dns_name'].LoadBalancerArn | [0]" \
    --output text)
  alb_arn=$(normalize_text_output "$alb_arn")

  if [ -z "$alb_arn" ]; then
    fail_check "Could not resolve ALB ARN from DNS name $alb_dns_name."
  else
    pass "Resolved ALB ARN $alb_arn."
  fi

  local device_listener_mode trust_store_arn
  device_listener_mode=$(aws_cli elbv2 describe-listeners \
    --load-balancer-arn "$alb_arn" \
    --query 'Listeners[?Port==`8443`].MutualAuthentication.Mode | [0]' \
    --output text 2>/dev/null || true)
  trust_store_arn=$(aws_cli elbv2 describe-listeners \
    --load-balancer-arn "$alb_arn" \
    --query 'Listeners[?Port==`8443`].MutualAuthentication.TrustStoreArn | [0]' \
    --output text 2>/dev/null || true)
  device_listener_mode=$(normalize_text_output "$device_listener_mode")
  trust_store_arn=$(normalize_text_output "$trust_store_arn")

  if [ "$device_listener_mode" = "verify" ] && [ -n "$trust_store_arn" ]; then
    pass "Device listener is in verify mode with trust store $trust_store_arn."
  else
    fail_check "Device listener is not in verify mode with an attached trust store."
  fi

  local waf_name
  waf_name=$(aws_cli wafv2 get-web-acl-for-resource \
    --resource-arn "$alb_arn" \
    --query 'WebACL.Name' \
    --output text 2>/tmp/aws-hardening-waf.err || true)
  waf_name=$(normalize_text_output "$waf_name")
  if [ -n "$waf_name" ]; then
    pass "ALB is associated with WAF web ACL $waf_name."
  else
    fail_check "ALB has no WAF web ACL association."
  fi

  alb_sg_id=$(aws_cli elbv2 describe-load-balancers \
    --load-balancer-arns "$alb_arn" \
    --query 'LoadBalancers[0].SecurityGroups[0]' \
    --output text)
  alb_sg_id=$(normalize_text_output "$alb_sg_id")
  if [ -n "$alb_sg_id" ]; then
    while IFS=$'\t' read -r level message; do
      [ -n "$level" ] || continue
      case "$level" in
        PASS) pass "$message" ;;
        WARN) warn "$message" ;;
        FAIL) fail_check "$message" ;;
        *) die "Unexpected ingress check level: $level" ;;
      esac
    done < <(python3 - "$alb_sg_id" "$(aws_cli ec2 describe-security-groups --group-ids "$alb_sg_id" --output json)" <<'PY'
import json
import sys

sg_id = sys.argv[1]
data = json.loads(sys.argv[2])
permissions = data["SecurityGroups"][0].get("IpPermissions", [])

cidrs_by_port = {}
for perm in permissions:
    port = perm.get("ToPort")
    if port not in (443, 8443):
        continue
    cidrs = []
    for item in perm.get("IpRanges", []):
        cidr = item.get("CidrIp")
        if cidr:
            cidrs.append(cidr)
    cidrs_by_port[port] = sorted(set(cidrs))


def emit(level, message):
    print(f"{level}\t{message}")


app = cidrs_by_port.get(443, [])
devices = cidrs_by_port.get(8443, [])

if not app:
    emit("FAIL", f"ALB security group {sg_id} has no port 443 ingress rule.")
else:
    emit("PASS", f"ALB security group {sg_id} exposes app ingress CIDRs on 443: {', '.join(app)}")

if not devices:
    emit("FAIL", f"ALB security group {sg_id} has no port 8443 ingress rule.")
else:
    emit("PASS", f"ALB security group {sg_id} exposes device ingress CIDRs on 8443: {', '.join(devices)}")

if "0.0.0.0/0" in app:
    emit("FAIL", "App ingress on port 443 still allows 0.0.0.0/0.")

if app and devices and app == devices:
    emit("FAIL", "App and device ingress CIDRs are identical; hardened deployment requires operator and device policy separation.")
PY
)
  else
    fail_check "Could not determine the ALB security group."
  fi

  if [ -n "$ALARM_PREFIX" ]; then
    local alarms_json
    alarms_json=$(aws_cli cloudwatch describe-alarms --alarm-name-prefix "$ALARM_PREFIX" --output json)
    while IFS=$'\t' read -r level message; do
      [ -n "$level" ] || continue
      case "$level" in
        PASS) pass "$message" ;;
        WARN) warn "$message" ;;
        FAIL) fail_check "$message" ;;
        *) die "Unexpected alarm check level: $level" ;;
      esac
    done < <(python3 - "$ALARM_PREFIX" "$SNS_TOPIC_ARN" "$alarms_json" <<'PY'
import json
import sys

prefix = sys.argv[1]
sns_topic_arn = sys.argv[2]
data = json.loads(sys.argv[3])
metric = data.get("MetricAlarms", [])
composite = data.get("CompositeAlarms", [])
alarms = metric + composite


def emit(level, message):
    print(f"{level}\t{message}")


if not alarms:
    emit("FAIL", f"No CloudWatch alarms found with prefix {prefix}.")
    raise SystemExit(0)

emit("PASS", f"Found {len(alarms)} CloudWatch alarms with prefix {prefix}.")

if sns_topic_arn:
    missing = []
    for alarm in alarms:
        actions = alarm.get("AlarmActions", [])
        if sns_topic_arn not in actions:
            missing.append(alarm.get("AlarmName", "<unknown>"))
    if missing:
        emit("FAIL", "Alarms missing the required SNS action: " + ", ".join(sorted(missing)))
    else:
        emit("PASS", f"All alarms with prefix {prefix} publish to {sns_topic_arn}.")
PY
)
  else
    warn "Alarm validation skipped; pass --alarm-prefix to validate CloudWatch coverage and --sns-topic-arn to validate SNS wiring."
  fi
}

print_summary_and_exit() {
  log
  log "Summary: $PASS_COUNT pass, $FAIL_COUNT fail, $WARN_COUNT warn"
  if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
  fi
}

while [ $# -gt 0 ]; do
  case "$1" in
    --customer) CUSTOMER=$2; shift 2 ;;
    --env) ENVIRONMENT=$2; shift 2 ;;
    --region) AWS_REGION=$2; shift 2 ;;
    --profile) AWS_PROFILE=$2; shift 2 ;;
    --work-dir) WORK_DIR=$2; shift 2 ;;
    --tfvars) TFVARS_PATH=$2; shift 2 ;;
    --alarm-prefix) ALARM_PREFIX=$2; shift 2 ;;
    --sns-topic-arn) SNS_TOPIC_ARN=$2; shift 2 ;;
    --help|-h) COMMAND="help"; shift ;;
    *) die "Unknown option: $1" ;;
  esac
done

case "$COMMAND" in
  config)
    run_repo_readiness_checks
    if [ -z "$TFVARS_PATH" ]; then
      ensure_customer_context
      TFVARS_PATH=$(tfvars_file)
    fi
    run_tfvars_checks "$TFVARS_PATH"
    print_summary_and_exit
    ;;
  deployment)
    run_deployment_checks
    print_summary_and_exit
    ;;
  help|*)
    usage
    ;;
esac
