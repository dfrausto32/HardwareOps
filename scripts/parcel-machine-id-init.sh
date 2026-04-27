#!/usr/bin/env bash
# parcel-machine-id-init.sh
#
# Ensures /etc/machine-id is unique before the Parcel agent enrolls.
# Run once on first boot via parcel-machine-id-init.service.
#
# Problem: machines cloned from the same ISO share an identical /etc/machine-id.
# The Parcel agent hashes machine-id as its hardware fingerprint. If two clones
# present the same fingerprint, the second enrollment is rejected with 409 Conflict.
#
# This script regenerates machine-id when the current value looks like a factory
# default (all-zeros, empty, or the placeholder systemd writes before setup),
# then drops a sentinel file so this logic never runs again on the same device.

set -euo pipefail

SENTINEL_FILE=${SENTINEL_FILE:-/var/lib/parcel/agent/.machine-id-initialized}
MACHINE_ID_FILE=${MACHINE_ID_FILE:-/etc/machine-id}

log() {
  echo "parcel-machine-id-init: $*" >&2
}

needs_new_machine_id() {
  # Not present at all
  if [ ! -f "$MACHINE_ID_FILE" ]; then
    return 0
  fi

  local id
  id=$(tr -d '[:space:]' < "$MACHINE_ID_FILE")

  # Empty file
  if [ -z "$id" ]; then
    return 0
  fi

  # All-zeros — systemd placeholder value used in container/image builds
  if [ "$id" = "00000000000000000000000000000000" ]; then
    return 0
  fi

  # Uninitialized placeholder that systemd-machine-id-setup recognizes
  if [ "$id" = "uninitialized" ]; then
    return 0
  fi

  return 1
}

mkdir -p "$(dirname "$SENTINEL_FILE")"

# Sentinel already exists — this device has been through first-boot init.
if [ -f "$SENTINEL_FILE" ]; then
  log "already initialized, nothing to do."
  exit 0
fi

if needs_new_machine_id; then
  log "machine-id is absent or uninitialized, generating a new one."
  systemd-machine-id-setup --commit
  log "new machine-id: $(cat "$MACHINE_ID_FILE")"
else
  log "machine-id looks valid ($(cat "$MACHINE_ID_FILE")), keeping it."
fi

# Drop sentinel so this service is a no-op on every subsequent boot.
touch "$SENTINEL_FILE"
log "sentinel written to $SENTINEL_FILE."
