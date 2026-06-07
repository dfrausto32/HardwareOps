# Local Dev Compose

For the consolidated deployment flow, see `../../docs/guides/deploy.md`.
For local dev details, see `../../docs/guides/local-dev-wsl.md`.

1) Copy env:
   cp .env.example .env
2) Start services:
   docker compose up -d

Agent containers (multi-agent):
```
docker compose -f docker-compose.agents.yml up --build --scale agent=5 -d
```

Optional interval override:
```
CHECKIN_INTERVAL=10s docker compose -f docker-compose.agents.yml up --build --scale agent=5 -d
```

Enable mTLS (requires device cert/key + CA):
```
DEVICE_CERT_PATH=/tmp/device.crt \
DEVICE_KEY_PATH=/tmp/device.key \
CONTROL_PLANE_CA_CERT_PATH=/tmp/dev-ca.crt \
docker compose -f docker-compose.agents.yml -f docker-compose.agents.mtls.yml up --build --scale agent=3 -d
```

Enable agent log export:
```
LOG_EXPORT_ADDR=tcp://host.docker.internal:5560 \
docker compose -f docker-compose.agents.yml up --build --scale agent=3 -d
```

Reverse proxy (Caddy, automatic HTTPS):
```
cp .env.example .env
# set CADDY_DOMAIN + CADDY_EMAIL in .env
# ensure CADDY_CLIENT_CA_PATH points to your device CA
docker compose -f docker-compose.proxy.yml up -d
```

If you use `./scripts/run-proxy.sh`, it will load `deploy/compose/.env` automatically and default
`CADDY_CLIENT_CA_PATH` to `../dev-ca.crt`.

On-prem stack (NGINX + UI + control-plane):
```
cp .env.onprem.example .env.onprem
docker compose -f docker-compose.onprem.yml --env-file .env.onprem up -d
```

On-prem defaults now assume:
- Human UI/API: `PUBLIC_BASE_URL` (for example `https://parcel.internal`)
- Agent/API host: `AGENT_BASE_URL` (for example `https://agent.parcel.internal`)
- Local auth enabled (`AUTH_MODE=local`)
- Device identity hardening in audit mode (`DEVICE_IDENTITY_MODE=audit`)

Set DNS for both hostnames to the gateway host. Agent runtime endpoints are restricted to
the `agent.*` host and require client mTLS for check-in/apply traffic.
For first-contact approval onboarding, point the packaged agent's `CONTROL_PLANE_URL` at
`AGENT_BASE_URL`, not `PUBLIC_BASE_URL`, and stage the trust anchor before starting the service.
To block duplicate hardware identities, set `DEVICE_IDENTITY_MODE=enforce` and optionally require identity on enroll/check-in.
For forwarded client-cert trust, keep `TRUST_PROXY=1` and replace the loopback-only `TRUST_PROXY_CIDRS` example with your ingress/gateway networks before enabling `HARDENED_PROFILE=1`.

Server TLS vs device CA:
- Device CA (`ca.crt`/`ca.key`) is for agent enrollment + mTLS.
- Gateway server cert (`server.crt`/`server.key`) is for browser trust.
- For production browsers, set `SERVER_CERT_MODE=external` and provide a publicly/enterprise-trusted server cert.

If users need to bootstrap trust, set `BOOTSTRAP_TOKEN` and download the CA cert from:
`GET /api/v1/bootstrap/ca` with header `X-Bootstrap-Token: <token>`.

Metrics stack (Prometheus + Grafana):
```
docker compose -f docker-compose.metrics.yml up -d
```
Grafana is pre-provisioned with a Parcel dashboard on `http://localhost:3000` (admin/admin).
