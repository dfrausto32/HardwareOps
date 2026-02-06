# On‑Prem Deployment (v1)

This document walks through deploying the control‑plane + UI stack on‑prem using Docker Compose, with CoreDNS and mTLS.

For VM-based testing, see `docs/vm-testing.md`. For fresh-machine install using bundles,
see `docs/installer-flow.md`.

## Troubleshooting
### TLS mismatch or `authority and subject key identifier mismatch`
Re‑issue certs and restart:
```
sudo FORCE=1 OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
docker compose -f deploy/compose/docker-compose.onprem.yml --env-file deploy/compose/.env.onprem restart gateway control-plane
```

### `502 Bad Gateway`
Control‑plane is down or DB not ready. Check logs:
```
docker compose -f deploy/compose/docker-compose.onprem.yml --env-file deploy/compose/.env.onprem logs --tail=200 control-plane
```

## 0) Build installers (prereq)
On a build machine, create OS/arch bundles so agents can be installed later:
```
./scripts/build-installers.sh
```
Bundles are written to `dist/installers/<version>/`.
See `docs/installers.md` for details.

## 1) Prepare the server
- Docker + Docker Compose installed
- A static IP (recommended)
- Decide on hostname: `hardwareops.internal`

## 2) TLS + CA
Fast path (generates CA + server cert and writes an env file):
```
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/setup-control-plane.sh
```

Manual path:
```
sudo OUT_DIR=/opt/hardwareops/certs ./scripts/bootstrap-ca.sh
sudo OUT_DIR=/opt/hardwareops/certs DOMAIN=hardwareops.internal ./scripts/issue-server-cert.sh
```

Trust the CA on client machines:
See `docs/certs.md`.

## 3) CoreDNS
Set up local DNS:
See `docs/dns-coredns.md`.

## 4) Start the stack
```
cp deploy/compose/.env.onprem.example deploy/compose/.env.onprem
```
Edit `deploy/compose/.env.onprem` and set:
- `CERTS_DIR=/opt/hardwareops/certs`
- `PUBLIC_BASE_URL=https://hardwareops.internal`

Bring up the stack:
```
docker compose -f deploy/compose/docker-compose.onprem.yml --env-file deploy/compose/.env.onprem up -d
```

Verify:
```
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/healthz
```

## 5) Enroll agents
See `docs/agent-systemd.md`.

## 6) UI
Open:
```
https://hardwareops.internal/
```

## Notes
- UI + API are served from the same hostname (NGINX proxy).
- Agents use mTLS; control‑plane uses the internal CA to sign device certs.
