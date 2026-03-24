#!/usr/bin/env bash
set -euo pipefail

BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <output-dir>" >&2
  exit 1
fi

OUT_DIR=$1

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/ci-templates" "$OUT_DIR/ci-helpers"

cp -a "$BASE_DIR/docs/customer/README.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/setup-onprem.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/setup-vendor-hosted-aws.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/first-agent-onboarding.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/ci-workflows.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/operations.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/customer/security-and-recovery.md" "$OUT_DIR/"
cp -a "$BASE_DIR/docs/icd.md" "$OUT_DIR/icd.md"
cp -a "$BASE_DIR/docs/ldap-auth.md" "$OUT_DIR/ldap-auth.md"
cp -a "$BASE_DIR/docs/artifact-provenance.md" "$OUT_DIR/artifact-provenance.md"
cp -a "$BASE_DIR/docs/cloud-pull-adapters.md" "$OUT_DIR/cloud-pull-adapters.md"
cp -a "$BASE_DIR/docs/vulnerability-scanning.md" "$OUT_DIR/vulnerability-scanning.md"
cp -a "$BASE_DIR/deploy/ci/templates/." "$OUT_DIR/ci-templates/"
cp -a "$BASE_DIR/scripts/ci-upload-artifact.sh" "$OUT_DIR/ci-helpers/"
cp -a "$BASE_DIR/scripts/ci-pull-artifact.sh" "$OUT_DIR/ci-helpers/"
cp -a "$BASE_DIR/scripts/ci-exchange-workload-identity.sh" "$OUT_DIR/ci-helpers/"
cp -a "$BASE_DIR/scripts/ci-exchange-gitlab-workload-identity.sh" "$OUT_DIR/ci-helpers/"
cp -a "$BASE_DIR/scripts/ci-exchange-jenkins-workload-identity.sh" "$OUT_DIR/ci-helpers/"
cp -a "$BASE_DIR/scripts/test-workload-identity.sh" "$OUT_DIR/ci-helpers/"
