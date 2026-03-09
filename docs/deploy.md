# Deployment Guide (Canonical)

This is the primary deployment guide for HardwareOps. It consolidates the previous split across installer, AWS, and hardening docs.

Use this file for:
- on-prem deployment (installer bundle)
- AWS per-customer deployment (Terraform + helper script)
- baseline security hardening checks

Deep-dive references are listed at the end.

---

## 1) Choose a deployment path

- **Local dev (WSL):** `local-dev-wsl.md`
- **Production-like Docker validation:** `testing/prod-docker-lab.md`
- **On-prem customer stack:** Section 2 in this file
- **AWS vendor-hosted per customer:** Section 3 in this file

---

## 2) On-prem deployment (step by step)

### 2.1 Build installers (build machine)

From repo root:

```bash
./scripts/build-installers.sh
```

Output bundle:
- `dist/installers/<version>/hardwareops-stack-<version>-linux-<arch>.tar.gz`

### 2.2 Install on target host

Copy bundle to target host, then:

```bash
tar -xzf hardwareops-stack-<version>-linux-<arch>.tar.gz
cd stack-<version>-linux-<arch>
sudo ./scripts/install-docker-ubuntu.sh
cp .env.onprem.example .env.onprem
# edit .env.onprem before first start, especially TRUST_PROXY_CIDRS
sudo ./scripts/run-stack.sh
```

Alternative desktop flow:
- run `./hardwareops-installer.sh` from bundle root.

### 2.3 Configure `.env.onprem`

Edit the stack's `.env.onprem` and set at minimum:

```env
DOMAIN=hardwareops.internal
PUBLIC_BASE_URL=https://hardwareops.internal
AGENT_BASE_URL=https://agent.hardwareops.internal
AUTH_MODE=local
AUTH_JWT_SECRET=<strong-random-secret>
AUTH_BOOTSTRAP_EMAIL=<admin-email>
AUTH_BOOTSTRAP_PASSWORD=<strong-random-password>
TRUST_PROXY=1
TRUST_PROXY_CIDRS=<only-your-gateway-or-proxy-cidrs>
```

Then restart:

```bash
docker compose -f docker-compose.onprem.bundle.yml --env-file .env.onprem up -d
```

### 2.4 Verify control-plane and UI

```bash
curl --cacert /opt/hardwareops/certs/ca.crt https://hardwareops.internal/healthz
```

Open:
- `https://hardwareops.internal`

Login with:
- `AUTH_BOOTSTRAP_EMAIL`
- `AUTH_BOOTSTRAP_PASSWORD`

### 2.5 Add agents

Use `agent-systemd.md` for Linux systemd agent install and enrollment.

---

## 3) AWS deployment (step by step, one stack per customer)

### 3.1 Prerequisites

- AWS CLI authenticated (`aws sts get-caller-identity` works)
- Terraform installed
- ECR images pushed:
  - control-plane image
  - gateway image
- ACM certificate for app/device hosts
- Route53 zone (or external DNS plan, see 3.5)

### 3.2 Bootstrap Terraform state (once per account/region)

```bash
./scripts/aws-customer.sh bootstrap-state \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks
```

### 3.3 Initialize a customer environment

```bash
./scripts/aws-customer.sh init \
  --customer parcel \
  --env dev \
  --region us-east-1 \
  --state-bucket hwops-tf-state \
  --state-lock-table hwops-tf-locks \
  --customer-domain parcel.tryparcel.dev \
  --route53-zone-id <zone-id> \
  --acm-cert-arn <acm-arn> \
  --control-plane-image <ecr-control-plane-image> \
  --gateway-image <ecr-gateway-image>
```

### 3.4 Plan/apply

```bash
./scripts/aws-customer.sh plan --customer parcel --env dev --region us-east-1
./scripts/aws-customer.sh apply --customer parcel --env dev --region us-east-1 --auto-approve
./scripts/aws-customer.sh output --customer parcel --env dev --region us-east-1
```

Open the `app_url` output in your browser.

### 3.5 DNS when domain is in Cloudflare

Two supported patterns:

1. **Delegate a subdomain to Route53** (recommended for cleaner AWS ownership)
2. **Keep Cloudflare authoritative** and create CNAMEs to the ALB DNS name

If using CNAMEs, map:
- `app.<customer-domain>` -> `<alb_dns_name>`
- `agent.<customer-domain>` -> `<alb_dns_name>`

### 3.6 Optional demo fleet (persistent demo agents)

Build and push demo image:

```bash
./scripts/aws-demo-image.sh
```

Enable during init:

```bash
./scripts/aws-customer.sh init ... \
  --enable-demo-fleet \
  --demo-agent-image <ecr-demo-agent-image> \
  --demo-agent-count 3
```

Seed demo artifacts:

```bash
BASE_URL=https://app.<customer-domain> \
AUTH_EMAIL=<bootstrap-admin-email> \
AUTH_PASSWORD=<bootstrap-admin-password> \
./scripts/aws-demo-seed.sh
```

---

## 4) Hardening baseline (required)

### 4.1 Proxy trust lock-down

Never trust all proxies. Keep:

```env
TRUST_PROXY=1
TRUST_PROXY_CIDRS=<restricted-cidrs-only>
```

- On AWS, default is the ECS private subnet CIDRs via Terraform.
- On-prem bundle templates now default to loopback-only trust; replace `TRUST_PROXY_CIDRS` with your real gateway/proxy subnet(s) before enabling `HARDENED_PROFILE=1`.

### 4.2 Auth defaults

- Keep `AUTH_MODE=local` or stronger.
- Do not ship with default bootstrap password in customer environments.
- Rotate `AUTH_JWT_SECRET`.

### 4.3 Secrets handling

- AWS: use Secrets Manager (for pull adapter credentials and sensitive env values).
- On-prem: protect `.env.onprem` with root-only permissions and immutable bit where possible.

### 4.4 On-prem anti-tamper (practical)

```bash
sudo chown root:root .env.onprem docker-compose.onprem.bundle.yml
sudo chmod 0400 .env.onprem
sudo chmod 0444 docker-compose.onprem.bundle.yml
sudo chattr +i .env.onprem docker-compose.onprem.bundle.yml
```

---

## 5) Quick troubleshooting

- **UI loads but API fails:** verify hostnames and TLS trust; open the exact domain in `PUBLIC_BASE_URL`.
- **CORS 400/401 in cloud:** verify `PUBLIC_BASE_URL`, `CORS_ALLOWED_ORIGINS`, and auth mode/env in running control-plane task.
- **Agents fail TLS:** verify agent CA path and matching control-plane server cert chain.
- **Upgrade stuck:** check `/var/lib/hardwareops/logs/upgrade-*.log` inside control-plane container.

---

## Deep-dive references

- Installer details: `installers.md`
- Fresh-machine installer flow: `installer-flow.md`
- Deployment hardening details: `deployment-hardening.md`
- Production-like Docker validation lab: `testing/prod-docker-lab.md`
- AWS strategy: `development/aws-cloud-setup-plan.md`
- AWS runbook (expanded): `development/aws-customer-deployment-runbook.md`
- Terraform module details: `../deploy/aws/terraform/README.md`
