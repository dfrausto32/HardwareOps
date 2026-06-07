# Backup + Restore Runbook (Postgres + Object Store)

Canonical operations guide: `operations.md`  
Use this file for backup/restore command detail.

This runbook covers **on‑prem** and **local dev** backups. It is intentionally simple and uses Docker‑native commands.

## Goals
- Capture **consistent Postgres** state.
- Capture **object store** (MinIO/S3) data.
- Provide a **repeatable restore** path.

## Prereqs
- Docker access on the control‑plane host.
- Maintenance mode available in the UI or via API.

## 0) Enter maintenance mode (recommended)
This prevents enrollments, check‑ins, and upgrades during backup.

UI: **Settings → Maintenance**  
API:
```
curl --cacert /opt/parcel/certs/ca.crt -X POST \
  https://parcel.internal/api/v1/maintenance \
  -H "Content-Type: application/json" \
  -d '{"enabled":true,"message":"Backup in progress"}'
```

## Backup commands

## 1) Postgres backup (consistent dump)
**Local dev (compose):**
```
docker exec -t compose-postgres-1 \
  pg_dump -U parcel -d parcel -Fc > hwops-postgres.dump
```

**On‑prem stack:**
```
docker exec -t parcel-postgres-1 \
  pg_dump -U parcel -d parcel -Fc > hwops-postgres.dump
```

## 2) Object store backup (MinIO volume)
Find the MinIO volume:
```
docker volume ls | grep minio
```

Backup the volume:
```
MINIO_VOL=<minio_volume_name>
docker run --rm -v ${MINIO_VOL}:/data -v "$PWD:/backup" alpine \
  tar -czf /backup/hwops-minio.tgz -C /data .
```

> If you’re using S3 (not MinIO), use your normal bucket backup policy instead.

## Restore commands

## 3) Restore Postgres
**Local dev / On‑prem:**
```
cat hwops-postgres.dump | docker exec -i parcel-postgres-1 \
  pg_restore -U parcel -d parcel -c
```

## 4) Restore MinIO volume
```
MINIO_VOL=<minio_volume_name>
docker run --rm -v ${MINIO_VOL}:/data -v "$PWD:/backup" alpine \
  sh -c 'rm -rf /data/* && tar -xzf /backup/hwops-minio.tgz -C /data'
```

## 5) Exit maintenance mode
```
curl --cacert /opt/parcel/certs/ca.crt -X POST \
  https://parcel.internal/api/v1/maintenance \
  -H "Content-Type: application/json" \
  -d '{"enabled":false,"message":""}'
```

## Verification
- `GET /healthz` returns `ok`.
- UI loads devices/artifacts correctly.
- Agents can check‑in.
- Sample artifact download succeeds.

## Notes / Pitfalls
- **Long‑running backups:** expect check‑ins to pause during maintenance.
- **Retention:** keep multiple backups (e.g., daily for 7–14 days).
- **Test restores:** perform a restore at least once to validate the process.
