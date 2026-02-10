#!/usr/bin/env sh
set -eu
mkdir -p files
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "agent preapply ok ${ts}" > files/preapply.txt
echo "${ts}" > files/last_applied.txt
echo "${ts} artifact=${HWOPS_ARTIFACT_ID:-} version=${HWOPS_ARTIFACT_VERSION:-}" >> files/apply.log
