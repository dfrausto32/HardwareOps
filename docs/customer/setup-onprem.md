# On-Prem Setup

Use this guide when a customer is deploying the HardwareOps control-plane and UI on their own host.

## 1. Build the installer bundle

On the build machine:

```bash
./scripts/build-installers.sh
```

Primary output:

```text
dist/installers/<version>/hardwareops-stack-<version>-linux-<arch>.tar.gz
```

## 2. Copy and extract on the target host

```bash
tar -xzf hardwareops-stack-<version>-linux-<arch>.tar.gz
cd stack-<version>-linux-<arch>
```

Install Docker if needed:

```bash
sudo ./scripts/install-docker-ubuntu.sh
```

## 3. Configure the environment

Create the runtime env file:

```bash
cp .env.onprem.example .env.onprem
```

Set at minimum:

```env
DOMAIN=hardwareops.internal
PUBLIC_BASE_URL=https://hardwareops.internal
AGENT_BASE_URL=https://agent.hardwareops.internal
AUTH_MODE=local
AUTH_JWT_SECRET=<strong-random-secret>
AUTH_BOOTSTRAP_EMAIL=<admin-email>
AUTH_BOOTSTRAP_PASSWORD=<strong-random-password>
TRUST_PROXY=1
TRUST_PROXY_CIDRS=<exact-gateway-or-proxy-cidrs>
```

If using trusted artifact verification from day one:

```env
ARTIFACT_TRUST_VERIFICATION_MODE=require_verified
TRUSTED_SIGNING_KEYS_FILE=/opt/hardwareops/trust/trusted-signing-keys.json
ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS=<comma-separated-key-ids>
```

## 4. Start the stack

```bash
sudo ./scripts/run-stack.sh
```

If you need to restart after env changes:

```bash
docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem up -d
```

## 5. Verify access

Health:

```bash
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/healthz
```

UI:
- open `https://hardwareops.internal`
- log in with:
  - `AUTH_BOOTSTRAP_EMAIL`
  - `AUTH_BOOTSTRAP_PASSWORD`

## 6. What the bundle contains

- prebuilt control-plane and gateway images
- on-prem compose file
- runtime env example
- install and support scripts
- customer documentation bundle

## 7. Next steps

After the stack is up:
1. create an enrollment profile
2. onboard the first agent
3. configure artifact trust policy
4. configure CI artifact publish or pull ingest
