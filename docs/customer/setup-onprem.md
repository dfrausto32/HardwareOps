# On-Prem Setup

Use this guide when a customer is deploying the Parcel control-plane and UI on their own host.

## 1. Build the installer bundle

On the build machine:

```bash
./scripts/build-installers.sh
```

Primary output:

```text
dist/installers/<version>/parcel-stack-<version>-linux-<arch>.tar.gz
```

## 2. Copy and extract on the target host

```bash
tar -xzf parcel-stack-<version>-linux-<arch>.tar.gz
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
DOMAIN=parcel.internal
PUBLIC_BASE_URL=https://parcel.internal
AGENT_BASE_URL=https://agent.parcel.internal
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
TRUSTED_SIGNING_KEYS_FILE=/opt/parcel/trust/trusted-signing-keys.json
ARTIFACT_TRUST_ALLOWED_SIGNING_KEY_IDS=<comma-separated-key-ids>
```

If using CI workload identity from day one:

```env
CI_WORKLOAD_IDENTITY_PROVIDERS_FILE=/opt/parcel/ci/workload-identity-providers.json
```

For connected environments using AWS Secrets Manager as the provider-config source:

```env
CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_SECRET_ID=<secret-id-or-arn>
CI_WORKLOAD_IDENTITY_PROVIDERS_AWS_REGION=<aws-region>
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
curl --cacert /opt/parcel/certs/ca.crt https://parcel.internal/healthz
```

UI:
- open `https://parcel.internal`
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
