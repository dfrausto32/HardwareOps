#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export WORKLOAD_IDENTITY_PROVIDER=${WORKLOAD_IDENTITY_PROVIDER:-jenkins}
export CI_WORKLOAD_IDENTITY_AUDIENCE=${CI_WORKLOAD_IDENTITY_AUDIENCE:-hardwareops-ci}

if [ -z "${CI_WORKLOAD_IDENTITY_TOKEN:-}" ]; then
  if [ -n "${JENKINS_OIDC_TOKEN:-}" ]; then
    export CI_WORKLOAD_IDENTITY_TOKEN="$JENKINS_OIDC_TOKEN"
  elif [ -n "${HWOPS_JENKINS_ID_TOKEN:-}" ]; then
    export CI_WORKLOAD_IDENTITY_TOKEN="$HWOPS_JENKINS_ID_TOKEN"
  fi
fi

exec "$BASE_DIR/scripts/ci-exchange-workload-identity.sh" "$@"
