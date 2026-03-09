# AWS Customer Deployment Runbook (Vendor-Hosted, One Stack per Customer)

For the consolidated deployment flow, start with `../deploy.md`. This document is the AWS-specific execution and security acceptance runbook for one customer stack.

Use it with:
- `aws-cloud-setup-plan.md`
- `security-hardening.md`
- `../../deploy/aws/terraform/README.md`
- `../deployment-hardening.md`
- `../../scripts/aws-customer.sh`
- `../../scripts/aws-hardening-check.sh`

## 1) What this runbook must prove

Final production acceptance for one customer environment requires operator evidence for all of the following:
- sensitive runtime values are not exposed as plaintext ECS task-definition environment entries
- task-role access is scoped and reviewable
- device ingress is mTLS-protected and app ingress is not left on the same broad CIDR policy
- WAF is attached to the customer ALB
- ECS Exec is disabled by default and any temporary enablement is time-boxed and audited
- alarm actions are wired to SNS/on-call and the notification path has been smoke-tested
- break-glass steps are written down, executable, and reversible

If any check fails, treat the stack as not ready for broad production rollout.

## 2) Current branch reality

This branch is the runbook and validation lane, not the Terraform hardening implementation lane. As of this repo state:
- `customer_stack` still injects `DATABASE_URL` through `default_control_plane_env`
- `modules/ecs` still defaults `enable_execute_command = true`
- the scaffold still exposes a shared `ingress_cidrs` model
- WAF and CloudWatch alarm resources are not present in the Terraform scaffold on this branch

That means the acceptance commands in this runbook are expected to fail until the Terraform hardening work lands. That is intentional. The release gate should fail loudly instead of silently accepting an incomplete hardening posture.

## 3) Operator inputs and evidence bucket

Collect these before first apply:
- AWS account/profile and region
- state bucket, lock table, and optional KMS key for Terraform state
- customer slug and environment
- customer domain, Route53 zone ID, and ACM certificate ARN
- control-plane and gateway image URIs
- device CA/trust-store bucket, key, and optional object version
- on-call SNS topic ARN or the inputs required to create it
- CloudWatch alarm prefix you will use for this stack, recommended: `hardwareops-<customer>-<env>`
- incident ticket ID or change record for the deployment

Create one evidence directory per release candidate:

```bash
CUSTOMER=acme
ENV=prod
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
EVIDENCE_DIR="$PWD/out/aws-hardening-${CUSTOMER}-${ENV}-${STAMP}"
mkdir -p "$EVIDENCE_DIR"
```

Store the output of every acceptance command there.

## 4) CLI wrapper workflow

The AWS wrapper remains the normal deploy path:

```bash
scripts/aws-customer.sh requirements
scripts/aws-customer.sh bootstrap-state \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks

scripts/aws-customer.sh init \
  --customer acme \
  --env prod \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks \
  --customer-domain acme.example.com \
  --route53-zone-id Z1234567890 \
  --acm-cert-arn arn:aws:acm:us-east-1:111122223333:certificate/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee \
  --device-mtls-bucket hardwareops-acme-prod-security-assets \
  --control-plane-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-control-plane:20260309 \
  --gateway-image 111122223333.dkr.ecr.us-east-1.amazonaws.com/hardwareops-gateway:20260309
```

The wrapper generates:
- `./.aws-customers/<customer>/<env>/backend.hcl`
- `./.aws-customers/<customer>/<env>/terraform.tfvars`
- `./.aws-customers/<customer>/<env>/README.txt`

## 5) Pre-apply hardening gate

Run the repo/config gate before `terraform plan`:

```bash
./scripts/aws-hardening-check.sh config \
  --customer acme \
  --env prod | tee "$EVIDENCE_DIR/00-config-gate.txt"
```

Expected evidence:
- all checks pass

Failure interpretation:
- `DATABASE_URL` plaintext failure means the current scaffold still injects a DB URL directly into the task definition
- WAF/alarm/ingress failures mean the Terraform hardening lane has not landed yet
- ECS Exec failure means the scaffold still allows exec-by-default
- plaintext `AUTH_JWT_SECRET` or `AUTH_BOOTSTRAP_PASSWORD` means the generated tfvars were not converted to Secrets Manager-backed values

Do not continue to production apply until this gate is green or an explicit exception is recorded.

## 6) Certificates and Secrets Manager preparation

### 6.1 Device CA assets

Generate the per-customer device CA assets:

```bash
CUSTOMER=acme
ENV=prod
OUT_DIR="$PWD/out/${CUSTOMER}-${ENV}-certs" ./scripts/bootstrap-ca.sh
cat "out/${CUSTOMER}-${ENV}-certs/ca.crt" > "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem"
```

Upload the ALB trust bundle:

```bash
aws s3 cp \
  "out/${CUSTOMER}-${ENV}-certs/device-ca-bundle.pem" \
  "s3://hardwareops-${CUSTOMER}-${ENV}-security-assets/mtls/device-ca-bundle.pem"
```

### 6.2 Pull-adapter credentials secret

If pull ingest uses `credentialRef`, create the resolver secret first:

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

Recommended Terraform input form:

```hcl
artifact_pull_credentials_aws_secret_id = "arn:aws:secretsmanager:us-east-1:111122223333:secret:hardwareops/acme/prod/artifact-pull-credentials-AbCdEf"
```

### 6.3 Production secret-ARN map

For production, move sensitive values out of `control_plane_env` and into `control_plane_secret_arns`.

At minimum, wire:
- `AUTH_JWT_SECRET`
- `AUTH_BOOTSTRAP_PASSWORD`
- `DATABASE_URL`
- `MAINTENANCE_TOKEN` when maintenance automation uses it

Expected evidence:
- `terraform.tfvars` no longer carries those values inline
- the secret ARN map contains the production secret references

Failure interpretation:
- if `AUTH_JWT_SECRET` or bootstrap credentials remain inline, anyone with task-definition read access can recover them
- if `DATABASE_URL` remains inline, the database credential path is not hardened even if other secrets are

## 7) Plan and apply

Run plan and apply through the wrapper:

```bash
./scripts/aws-customer.sh plan \
  --customer acme \
  --env prod \
  --region us-east-1 | tee "$EVIDENCE_DIR/01-plan.txt"

./scripts/aws-customer.sh apply \
  --customer acme \
  --env prod \
  --region us-east-1 \
  --auto-approve | tee "$EVIDENCE_DIR/02-apply.txt"

./scripts/aws-customer.sh status \
  --customer acme \
  --env prod \
  --region us-east-1 | tee "$EVIDENCE_DIR/03-status.txt"
```

For first-time fleet enrollment only:
- start with `device_mtls_mode = "passthrough"`
- enroll the fleet
- switch to `device_mtls_mode = "verify"`
- re-apply before calling the stack hardened

## 8) Post-apply validation context

Set one shell context and reuse it for the raw checks:

```bash
CUSTOMER=acme
ENV=prod
AWS_REGION=us-east-1
AWS_PROFILE=hwops-admin
WORK_DIR="$PWD/.aws-customers"
TF_ENV_DIR="$PWD/deploy/aws/terraform/envs/$ENV"
TF_BACKEND_FILE="$WORK_DIR/$CUSTOMER/$ENV/backend.hcl"

AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
terraform -chdir="$TF_ENV_DIR" init -backend-config "$TF_BACKEND_FILE" -input=false

CLUSTER=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw ecs_cluster_name)
CP_SERVICE=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw ecs_control_plane_service_name)
GW_SERVICE=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw ecs_gateway_service_name)
ALB_DNS_NAME=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw alb_dns_name)
APP_URL=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw app_url)
DEVICES_URL=$(AWS_PROFILE="$AWS_PROFILE" AWS_REGION="$AWS_REGION" AWS_DEFAULT_REGION="$AWS_REGION" \
  terraform -chdir="$TF_ENV_DIR" output -raw devices_url)
```

Run the deployment helper first:

```bash
./scripts/aws-hardening-check.sh deployment \
  --customer "$CUSTOMER" \
  --env "$ENV" \
  --region "$AWS_REGION" \
  --profile "$AWS_PROFILE" | tee "$EVIDENCE_DIR/04-deployment-gate.txt"
```

Expected evidence:
- all checks pass

Failure interpretation:
- any failure is a release blocker for the hardening package

## 9) Raw acceptance checks

Use these when the helper flags a failure or when a reviewer wants the underlying evidence.

### 9.1 Secrets handling path

Commands:

```bash
CP_TASK_DEF=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs describe-services \
  --cluster "$CLUSTER" \
  --services "$CP_SERVICE" \
  --query 'services[0].taskDefinition' \
  --output text)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs describe-task-definition \
  --task-definition "$CP_TASK_DEF" \
  --query 'taskDefinition.containerDefinitions[?name==`control-plane`] | [0].{env:environment[].name,secrets:secrets[].name}' \
  --output json
```

Expected evidence:
- `AUTH_JWT_SECRET`, `AUTH_BOOTSTRAP_PASSWORD`, `DATABASE_URL`, and `MAINTENANCE_TOKEN` are absent from `env`
- secret-backed values appear under `secrets`
- `ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID` and `ARTIFACT_PULL_CREDENTIALS_AWS_REGION` appear only when pull credentials are enabled

Failure interpretation:
- any sensitive key in `env` means the task definition still exposes plaintext runtime secrets
- missing `secrets` entries means the production secret path was not wired

### 9.2 IAM scoping review points

Commands:

```bash
TASK_ROLE_ARN=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs describe-task-definition \
  --task-definition "$CP_TASK_DEF" \
  --query 'taskDefinition.taskRoleArn' \
  --output text)
TASK_ROLE_NAME=${TASK_ROLE_ARN##*/}

aws --profile "$AWS_PROFILE" iam list-attached-role-policies \
  --role-name "$TASK_ROLE_NAME" \
  --query 'AttachedPolicies[].{name:PolicyName,arn:PolicyArn}' \
  --output table

aws --profile "$AWS_PROFILE" iam list-role-policies \
  --role-name "$TASK_ROLE_NAME" \
  --query 'PolicyNames' \
  --output table
```

Expected evidence:
- no AWS-managed broad policies attached to the task role
- inline or customer-managed policies are scoped to the artifact bucket, exact secrets, and any required KMS keys

Failure interpretation:
- AWS-managed policies such as `AmazonS3ReadOnlyAccess` or `CloudWatchAgentServerPolicy` mean the role is still broader than the hardening target

### 9.3 WAF presence and behavior

Commands:

```bash
ALB_ARN=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" elbv2 describe-load-balancers \
  --query "LoadBalancers[?DNSName=='$ALB_DNS_NAME'].LoadBalancerArn | [0]" \
  --output text)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" wafv2 get-web-acl-for-resource \
  --resource-arn "$ALB_ARN"

curl -isk "https://$(printf '%s' "$APP_URL" | sed 's#^https://##')/?probe=<script>alert(1)</script>"
```

Expected evidence:
- `get-web-acl-for-resource` returns a `WebACL`
- the request probe is blocked or explicitly counted according to the chosen WAF rule mode

Failure interpretation:
- no `WebACL` means the app edge is unprotected
- probe request succeeds with no WAF evidence means the app listener is not benefiting from WAF policy enforcement

### 9.4 Ingress policy expectations

Commands:

```bash
ALB_SG_ID=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" elbv2 describe-load-balancers \
  --load-balancer-arns "$ALB_ARN" \
  --query 'LoadBalancers[0].SecurityGroups[0]' \
  --output text)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ec2 describe-security-groups \
  --group-ids "$ALB_SG_ID" \
  --query 'SecurityGroups[0].IpPermissions[?ToPort==`443` || ToPort==`8443`].[ToPort,IpRanges[].CidrIp]' \
  --output json
```

Expected evidence:
- port `443` is restricted to operator or corporate ingress CIDRs
- port `8443` remains aligned with device access requirements
- the two ports do not share the same broad CIDR policy

Failure interpretation:
- `443` on `0.0.0.0/0` is a hard fail for operator ingress
- identical CIDR sets on `443` and `8443` means ingress policy split has not landed

### 9.5 Device mTLS posture

Commands:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" elbv2 describe-listeners \
  --load-balancer-arn "$ALB_ARN" \
  --query 'Listeners[?Port==`8443`].MutualAuthentication' \
  --output json

curl -isk "$DEVICES_URL/healthz"
```

Expected evidence:
- `Mode` is `verify`
- `TrustStoreArn` is populated
- unauthenticated probe to the device endpoint is rejected once the fleet is in hardened mode

Failure interpretation:
- `passthrough` is valid only during first enrollment, not for final prod sign-off
- empty `TrustStoreArn` means the ALB is not actually validating client certificates

### 9.6 ECS Exec posture

Commands:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs describe-services \
  --cluster "$CLUSTER" \
  --services "$CP_SERVICE" "$GW_SERVICE" \
  --query 'services[].{service:serviceName,exec:enableExecuteCommand}' \
  --output table
```

Expected evidence:
- `enableExecuteCommand` is `False` for both services

Failure interpretation:
- `True` means interactive shell access is available in production by default and the stack has not met the hardening bar

### 9.7 Alarm coverage and SNS wiring

Commands:

```bash
ALARM_PREFIX="hardwareops-${CUSTOMER}-${ENV}"
TOPIC_ARN="arn:aws:sns:${AWS_REGION}:111122223333:hardwareops-${CUSTOMER}-${ENV}-security"

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch describe-alarms \
  --alarm-name-prefix "$ALARM_PREFIX" \
  --query 'MetricAlarms[].{name:AlarmName,state:StateValue,actions:AlarmActions}' \
  --output table

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-subscriptions-by-topic \
  --topic-arn "$TOPIC_ARN" \
  --query 'Subscriptions[].{protocol:Protocol,endpoint:Endpoint,status:SubscriptionArn}' \
  --output table
```

Expected evidence:
- required alarms exist
- every alarm action list includes the same SNS topic ARN
- the topic has at least one confirmed subscription for the on-call path

Failure interpretation:
- no alarms means the stack has no automated response wiring
- blank or missing alarm actions means the alarm will never page
- `PendingConfirmation` on the SNS subscription means the on-call path has not actually been activated

## 10) Response wiring guide

### 10.1 Create or verify the SNS topic

```bash
TOPIC_NAME="hardwareops-${CUSTOMER}-${ENV}-security"
TOPIC_ARN=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns create-topic \
  --name "$TOPIC_NAME" \
  --attributes KmsMasterKeyId=alias/aws/sns \
  --query 'TopicArn' \
  --output text)
printf '%s\n' "$TOPIC_ARN"
```

### 10.2 Subscribe the on-call target

PagerDuty/HTTPS example:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns subscribe \
  --topic-arn "$TOPIC_ARN" \
  --protocol https \
  --notification-endpoint https://events.pagerduty.com/integration/<integration-key>/enqueue
```

Email fallback example:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns subscribe \
  --topic-arn "$TOPIC_ARN" \
  --protocol email \
  --notification-endpoint oncall@example.com
```

### 10.3 Confirm the routing path

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-subscriptions-by-topic \
  --topic-arn "$TOPIC_ARN" \
  --output table

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns publish \
  --topic-arn "$TOPIC_ARN" \
  --subject "HardwareOps AWS hardening test" \
  --message "{\"customer\":\"$CUSTOMER\",\"env\":\"$ENV\",\"check\":\"sns-routing\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
```

Expected evidence:
- on-call receives the test message
- the ticket or ack ID is stored in the evidence directory

Failure interpretation:
- direct SNS publish failure means fix IAM or topic policy before worrying about alarm definitions

### 10.4 Recommended threshold set

Use this threshold set as the baseline until Terraform-managed alarms are added:
- `ALB target 5xx`: page at `>= 5` errors for `2/2` five-minute periods
- `ALB 4xx anomaly`: ticket at `>= 100` responses for `3/3` five-minute periods; page only when WAF blocks or invalid client-cert traffic do not explain the spike
- `ECS running-task deficit`: page when `runningCount < desiredCount` for `10` minutes
- `UnHealthyHostCount`: page when `> 0` for `2/2` one-minute periods
- `Auth failure spike`: page at `>= 15` failures in `5` minutes; until CloudWatch metric export exists, treat this as a manual dashboard/log review item
- `mTLS verification failure spike`: page at `>= 10` failures in `5` minutes or `> 5%` of device traffic; until metric export exists, treat this as a manual dashboard/log review item

Manual review items are still release blockers if no equivalent automated alarm exists.

## 11) Incident handling expectations

When a hardening alarm or validation failure occurs:
1. acknowledge the page within 5 minutes
2. open or update the incident ticket with customer, env, and alarm name
3. capture current output from `scripts/aws-hardening-check.sh deployment`
4. identify whether the event is edge-only (`WAF`, `ALB`, `ingress`), runtime-only (`ECS`, `IAM`, `secret path`), or credential-related
5. rotate or revoke exposed service tokens immediately if compromise is suspected
6. use ECS Exec only through the break-glass procedure below
7. after remediation, rerun the acceptance commands and attach the clean evidence

## 12) Break-glass procedure

### 12.1 Temporary ECS Exec enablement

Prerequisites:
- active incident ticket
- named operator performing the action
- reason recorded in the ticket

Commands:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs update-service \
  --cluster "$CLUSTER" \
  --service "$CP_SERVICE" \
  --enable-execute-command \
  --force-new-deployment

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs wait services-stable \
  --cluster "$CLUSTER" \
  --services "$CP_SERVICE"

TASK_ARN=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs list-tasks \
  --cluster "$CLUSTER" \
  --service-name "$CP_SERVICE" \
  --query 'taskArns[0]' \
  --output text)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs execute-command \
  --cluster "$CLUSTER" \
  --task "$TASK_ARN" \
  --container control-plane \
  --interactive \
  --command "/bin/sh"
```

Re-disable immediately after the session:

```bash
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs update-service \
  --cluster "$CLUSTER" \
  --service "$CP_SERVICE" \
  --disable-execute-command \
  --force-new-deployment

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecs wait services-stable \
  --cluster "$CLUSTER" \
  --services "$CP_SERVICE"
```

Expected evidence:
- CloudTrail shows the service update and `ExecuteCommand`
- incident ticket contains operator, timestamps, and reason
- final `describe-services` output shows `enableExecuteCommand=False`

Failure interpretation:
- if exec remains enabled after the incident, the stack is still out of compliance

### 12.2 Service-token compromise

```bash
curl --fail --silent --show-error \
  -H "Authorization: Bearer <operator-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"suspected CI token exposure","ttlHours":1}' \
  "$APP_URL/api/v1/auth/service-tokens/<token-id>/rotate"
```

Use revoke instead of rotate if the token should not survive:

```bash
curl --fail --silent --show-error \
  -H "Authorization: Bearer <operator-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"token no longer trusted"}' \
  "$APP_URL/api/v1/auth/service-tokens/<token-id>/revoke"
```

### 12.3 Certificate compromise

```bash
curl --fail --silent --show-error \
  -H "Authorization: Bearer <operator-jwt>" \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"reason":"break-glass CA rotation after suspected compromise"}' \
  "$APP_URL/api/v1/cert-rotation/rotate"
```

Follow with reload/cleanup from `../certs.md` after rotation coverage is verified.

## 13) Release gate checklist

The coordinator should not mark `D-AWS-HARDENING-RUNBOOK` complete for a customer environment until all of these are attached to the release record:
- `00-config-gate.txt` from `scripts/aws-hardening-check.sh config`
- `01-plan.txt`, `02-apply.txt`, and `03-status.txt`
- `04-deployment-gate.txt` from `scripts/aws-hardening-check.sh deployment`
- raw CLI evidence for secrets path, IAM review, WAF association, ingress rules, and ECS Exec status
- SNS publish test evidence and confirmed subscription output
- CloudWatch alarm inventory showing the chosen alarm prefix and SNS action wiring
- explicit note that device ingress is back on `verify` mode after enrollment
- explicit statement that any temporary ECS Exec session was disabled and closed

Any missing item is a release blocker, not a documentation TODO.
