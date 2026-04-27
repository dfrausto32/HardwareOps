#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export WORKLOAD_IDENTITY_PROVIDER=${WORKLOAD_IDENTITY_PROVIDER:-gitlab-ci}
export CI_WORKLOAD_IDENTITY_AUDIENCE=${CI_WORKLOAD_IDENTITY_AUDIENCE:-parcel-ci}

if [ -z "${CI_WORKLOAD_IDENTITY_TOKEN:-}" ]; then
  if [ -n "${HWOPS_GITLAB_ID_TOKEN:-}" ]; then
    export CI_WORKLOAD_IDENTITY_TOKEN="$HWOPS_GITLAB_ID_TOKEN"
  elif [ -n "${CI_JOB_JWT_V2:-}" ]; then
    export CI_WORKLOAD_IDENTITY_TOKEN="$CI_JOB_JWT_V2"
  elif [ -n "${CI_JOB_JWT:-}" ]; then
    export CI_WORKLOAD_IDENTITY_TOKEN="$CI_JOB_JWT"
  fi
fi

exec "$BASE_DIR/scripts/ci-exchange-workload-identity.sh" "$@"
