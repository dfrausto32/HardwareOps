# Local Dev Compose

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

Reverse proxy (Caddy, automatic HTTPS):
```
cp .env.example .env
# set CADDY_DOMAIN + CADDY_EMAIL in .env
# ensure CADDY_CLIENT_CA_PATH points to your device CA
docker compose -f docker-compose.proxy.yml up -d
```

If you use `./scripts/run-proxy.sh`, it will load `deploy/compose/.env` automatically and default
`CADDY_CLIENT_CA_PATH` to `../dev-ca.crt`.
