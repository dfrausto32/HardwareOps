#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR=${BACKUP_DIR:-/stack/backups}
POSTGRES_CONTAINER=${BACKUP_POSTGRES_CONTAINER:-${POSTGRES_CONTAINER:-}}
MINIO_CONTAINER=${BACKUP_MINIO_CONTAINER:-${MINIO_CONTAINER:-}}
POSTGRES_USER=${BACKUP_POSTGRES_USER:-${POSTGRES_USER:-hardwareops}}
POSTGRES_DB=${BACKUP_POSTGRES_DB:-${POSTGRES_DB:-hardwareops}}

mkdir -p "$BACKUP_DIR"

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

ts=$(date -u +"%Y%m%d-%H%M%S")
id="backup-${ts}"
pg_dump_path="$BACKUP_DIR/${id}.pg.dump"
minio_path="$BACKUP_DIR/${id}.minio.tgz"

echo "Backing up Postgres (${POSTGRES_CONTAINER})..."
docker exec "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > "$pg_dump_path"

minio_vol=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$MINIO_CONTAINER")
if [ -z "$minio_vol" ]; then
  echo "minio volume not found for ${MINIO_CONTAINER}" >&2
  exit 1
fi

echo "Backing up MinIO volume (${minio_vol})..."
docker run --rm -v "${minio_vol}:/data:ro" -v "${BACKUP_DIR}:/backup" alpine \
  sh -c "cd /data && tar -czf /backup/${id}.minio.tgz ."

echo "Backup complete: ${id}"
