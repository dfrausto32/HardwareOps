# Operations

This guide covers the core day-2 tasks customers typically need.

## 1. Backups

Preferred path:
- use the UI: `Settings -> Backups`

Operational sequence:
1. enable maintenance mode if appropriate
2. create backup
3. store the backup outside the local host

After restore, always verify:
- `/healthz` is healthy
- UI loads
- artifacts list correctly
- devices resume check-in

## 2. Upgrades

Preferred path:
- use the UI: `Settings -> Maintenance -> Upgrade`

Flow:
1. stage the upgrade bundle on the stack host
2. run preflight
3. apply
4. verify services return healthy

If an upgrade fails:
- inspect upgrade logs under `/var/lib/parcel/logs`

## 3. Certificate rotation

Available paths:
- `Security -> Certificate Rotation`
- API endpoints
- helper scripts

After rotation:
- verify devices moved to the new CA fingerprint
- verify device count did not increase because of re-enroll

## 4. Pull credential reload

If repository credentials change for pull ingest:
1. update the credential source
2. reload pull credentials without restarting

Helper:

```bash
./scripts/reload-pull-credentials.sh
```

## 5. Monitoring

Primary checks:
- `/healthz`
- `/api/v1/health/summary`
- `/metrics`

Key signals:
- active devices
- check-in failures
- DB pressure
- storage growth
- pending-enrollment pressure

## 6. Recommended periodic checks

- confirm auth remains enabled
- confirm backups are recent and restorable
- confirm trusted proxy CIDRs are still restricted
- confirm artifact trust policy is still appropriate for the environment
