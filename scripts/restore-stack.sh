#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR=${BACKUP_DIR:-/stack/backups}
POSTGRES_CONTAINER=${BACKUP_POSTGRES_CONTAINER:-${POSTGRES_CONTAINER:-}}
MINIO_CONTAINER=${BACKUP_MINIO_CONTAINER:-${MINIO_CONTAINER:-}}
POSTGRES_USER=${BACKUP_POSTGRES_USER:-${POSTGRES_USER:-parcel}}
POSTGRES_DB=${BACKUP_POSTGRES_DB:-${POSTGRES_DB:-parcel}}
BACKUP_ID=${BACKUP_ID:-}
WIPE=${WIPE:-0}

if [ -z "$BACKUP_ID" ]; then
  echo "BACKUP_ID required" >&2
  exit 1
fi
if [ "$WIPE" != "1" ]; then
  echo "WIPE=1 required to restore" >&2
  exit 1
fi

resolve_container() {
  local svc="$1"
  local name="$2"
  local label="com.docker.compose.service=${svc}"
  if [ -n "$name" ]; then
    echo "$name"
    return 0
  fi
  local found
  found=$(docker ps --filter "label=${label}" --format '{{.Names}}' | head -n1)
  if [ -n "$found" ]; then
    echo "$found"
    return 0
  fi
  found=$(docker ps --format '{{.Names}}' | grep -E "${svc}" | head -n1 || true)
  if [ -n "$found" ]; then
    echo "$found"
    return 0
  fi
  return 1
}

POSTGRES_CONTAINER=$(resolve_container postgres "$POSTGRES_CONTAINER" || true)
MINIO_CONTAINER=$(resolve_container minio "$MINIO_CONTAINER" || true)

if [ -z "$POSTGRES_CONTAINER" ]; then
  echo "postgres container not found. Set BACKUP_POSTGRES_CONTAINER." >&2
  exit 1
fi
if [ -z "$MINIO_CONTAINER" ]; then
  echo "minio container not found. Set BACKUP_MINIO_CONTAINER." >&2
  exit 1
fi

pg_dump_path="$BACKUP_DIR/${BACKUP_ID}.pg.dump"
minio_path="$BACKUP_DIR/${BACKUP_ID}.minio.tgz"

if [ ! -f "$pg_dump_path" ]; then
  echo "postgres dump not found: $pg_dump_path" >&2
  exit 1
fi
if [ ! -f "$minio_path" ]; then
  echo "minio archive not found: $minio_path" >&2
  exit 1
fi

echo "Restoring Postgres (${POSTGRES_CONTAINER})..."
cat "$pg_dump_path" | docker exec -i "$POSTGRES_CONTAINER" pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c

minio_vol=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$MINIO_CONTAINER")
if [ -z "$minio_vol" ]; then
  echo "minio volume not found for ${MINIO_CONTAINER}" >&2
  exit 1
fi

echo "Restoring MinIO volume (${minio_vol})..."
docker run --rm -v "${minio_vol}:/data" -v "${BACKUP_DIR}:/backup" alpine \
  sh -c "rm -rf /data/* && tar -xzf /backup/${BACKUP_ID}.minio.tgz -C /data"

echo "Restore complete: ${BACKUP_ID}"
