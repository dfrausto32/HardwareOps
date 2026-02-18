#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

AWS_REGION=${AWS_REGION:-$(aws configure get region 2>/dev/null || true)}
AWS_PROFILE=${AWS_PROFILE:-}
ECR_REPOSITORY=${ECR_REPOSITORY:-hardwareops-demo-agent}
IMAGE_TAG=${IMAGE_TAG:-latest}
PUSH_IMAGE=${PUSH_IMAGE:-1}
CREATE_REPO=${CREATE_REPO:-1}
NO_CACHE=${NO_CACHE:-0}

if [ -z "$AWS_REGION" ]; then
  echo "AWS_REGION is required (or configure a default AWS CLI region)." >&2
  exit 1
fi

command -v aws >/dev/null 2>&1 || { echo "aws CLI not found" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "docker not found" >&2; exit 1; }

aws_cmd=(aws --region "$AWS_REGION")
if [ -n "$AWS_PROFILE" ]; then
  aws_cmd+=(--profile "$AWS_PROFILE")
fi

ACCOUNT_ID=$("${aws_cmd[@]}" sts get-caller-identity --query Account --output text)
REPO_URI="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPOSITORY}"
IMAGE_URI="${REPO_URI}:${IMAGE_TAG}"

if ! "${aws_cmd[@]}" ecr describe-repositories --repository-names "$ECR_REPOSITORY" >/dev/null 2>&1; then
  if [ "$CREATE_REPO" = "1" ]; then
    "${aws_cmd[@]}" ecr create-repository --repository-name "$ECR_REPOSITORY" >/dev/null
  else
    echo "ECR repository $ECR_REPOSITORY does not exist and CREATE_REPO=0" >&2
    exit 1
  fi
fi

build_args=()
if [ "$NO_CACHE" = "1" ]; then
  build_args+=(--no-cache)
fi

docker build "${build_args[@]}" -f "$BASE_DIR/agent/Dockerfile.aws-demo" -t "$IMAGE_URI" "$BASE_DIR"

if [ "$PUSH_IMAGE" = "1" ]; then
  "${aws_cmd[@]}" ecr get-login-password | docker login --username AWS --password-stdin "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com" >/dev/null
  docker push "$IMAGE_URI" >/dev/null
fi

cat <<EOF
Demo agent image ready:
  $IMAGE_URI

Use this in Terraform:
  demo_agent_image = "$IMAGE_URI"
EOF
