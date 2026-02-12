#!/usr/bin/env sh
set -eu
mkdir -p files
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "agent preapply ok ${ts}" > files/preapply.txt
echo "${ts}" > files/last_applied.txt
echo "${ts} artifact=${HWOPS_ARTIFACT_ID:-} version=${HWOPS_ARTIFACT_VERSION:-}" >> files/apply.log
pid_file="${HWOPS_ARTIFACT_ROOT}/heartbeat.pid"
log_file="${HWOPS_ARTIFACT_DIR}/files/heartbeat.log"
if [ -f "$pid_file" ]; then
  old_pid=$(cat "$pid_file" 2>/dev/null || true)
  if [ -n "${old_pid:-}" ] && kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
fi
( while true; do
    beat_ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "$beat_ts" >> "$log_file"
    sleep 10
  done ) >/dev/null 2>&1 &
echo $! > "$pid_file"
