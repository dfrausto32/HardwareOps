# Production-Like Docker Lab

Use this lab when you want to validate on-prem production behavior without bare-metal hardware.

This path runs the full on-prem compose stack (`control-plane`, `gateway`, `maintenance-runner`, `postgres`, `minio`) using Docker.

## What This Lab Covers

- Local auth and admin workflows
- Agent mTLS enrollment/check-in
- Artifact ingest/apply
- Maintenance operations (upgrade, backup, restore)
- Certificate rotation and re-enroll
- Metrics, event history, and retention behavior
- Security hardening controls in a production-like topology

## What This Lab Does Not Cover

- Real cloud LB/WAF behavior (AWS ALB, Cloudflare, etc.)
- Real customer network segmentation
- Hardware-specific identity signals beyond what demo agents report

---

## 1) Prerequisites

- Docker Engine + Compose plugin
- `openssl`, `python3`, `curl`
- Free local ports: `80`, `443`, `53` (if CoreDNS is used separately)
- Repo root as current directory

---

## 2) Initialize the Lab (hardened-by-default)

Use the dedicated test scripts:

```bash
./scripts/testing/prod-lab-init.sh
```

This generates a dedicated lab workspace under `.tmp/prod-docker`:
This generates a dedicated lab workspace (default: `/tmp/hardwareops-prod-docker`):

- `<LAB_ROOT>/.env.onprem`
- `<LAB_ROOT>/certs/*`
- `<LAB_ROOT>/stack/license.json`
- `<LAB_ROOT>/stack/license-keys/*`
- `<LAB_ROOT>/stack/signing/*`

Defaults are tuned for production parity:

- `AUTH_MODE=local` with generated strong secrets
- `HARDENED_PROFILE=1`
- `LICENSE_ENFORCE=1` with generated signed lab license
- `DEVICE_IDENTITY_MODE=enforce`
- `TRUST_PROXY_CIDRS=127.0.0.1/32,::1/128,172.16.0.0/12` so the Docker-network gateway can forward client-cert headers
- signature policy defaults enabled
- remote maintenance runner mode (control-plane without docker socket)

If your shell does not resolve `*.localhost` subdomains automatically, add:

```bash
echo "127.0.0.1 hwops.localhost agent.hwops.localhost" | sudo tee -a /etc/hosts
```

Note: `scripts/testing/prod-lab-seed.sh` can now run without host entries by using `curl --resolve` fallback automatically.

You can override values at runtime, for example:

```bash
DOMAIN=parcel.localhost MAX_DEVICES=500 ./scripts/testing/prod-lab-init.sh
```

Set a custom lab root if you want repo-local files:

```bash
LAB_ROOT=$PWD/.tmp/prod-docker ./scripts/testing/prod-lab-init.sh
```

---

## 3) Start the Production-Like Stack

```bash
./scripts/testing/prod-lab-up.sh
```

The script uses:

- compose file: `deploy/compose/docker-compose.onprem.yml`
- env file: `.tmp/prod-docker/.env.onprem`
- project name: `hwops-prodtest`

---

## 4) Smoke Test the Lab

```bash
./scripts/testing/prod-lab-smoke.sh
```

This verifies:

- `/healthz`
- admin login
- `/api/v1/devices` with auth token
- `/metrics`

---

## 5) Validate First-Contact Approval Onboarding

Build the packaged Linux agent bundle first:

```bash
AGENT_PLATFORMS=linux/<arch> CONTROL_PLANE_PLATFORMS=linux/<arch> BUILD_STACK=0 ./scripts/build-installers.sh
```

Use `amd64` on x86_64 hosts and `arm64` on ARM hosts.

Then run the dedicated onboarding validation:

```bash
./scripts/testing/prod-lab-first-contact.sh
```

What this covers:
- creates an enrollment profile in the running lab
- extracts the packaged `hardwareops-agent` Linux bundle
- runs `scripts/agent-install.sh` inside an ephemeral Ubuntu container
- starts the real Go agent in `AGENT_ENROLL_MODE=approval`
- verifies the request waits in the pending queue until operator approval
- approves the request and confirms the same installed agent enters active mTLS check-in

Current lab boundary:
- the bundle/install path is real
- the agent process is real
- the service handoff is simulated by launching the installed agent directly after `agent-install.sh`
  because this lab does not assume passwordless root or host systemd access

---

## 6) Verify Access Manually

API health check:

```bash
curl --resolve hwops.localhost:443:127.0.0.1 \
  --cacert /tmp/hardwareops-prod-docker/certs/ca.crt \
  https://hwops.localhost/healthz
```

UI:
- Open `https://hwops.localhost`
- Login with `AUTH_BOOTSTRAP_EMAIL` / `AUTH_BOOTSTRAP_PASSWORD`

---

## 7) Seed Demo Load (Optional but Recommended)

Seed devices + signed artifacts in one step:

```bash
DEMO_COUNT=5 DEMO_RESET=1 ./scripts/testing/prod-lab-seed.sh
```

This does all of the following:

- Enroll `DEMO_COUNT` demo agents using the lab bootstrap admin
- Upload signed `agent_bundle` artifact
- Upload signed multi-component app artifacts (`customer` + `integration`, versions `0.1.0` and `0.2.0`)
- Reuse the lab signing key so signature-enforcement policies stay valid

Customize artifact sets:

```bash
DEMO_COUNT=3 APP_NAMES=customer,integration,ops VERSIONS=0.1.0,0.2.0,0.3.0 \
./scripts/testing/prod-lab-seed.sh
```

---

## 8) Run the Full Validation Plan

Use:
- `docs/testing/prod-docker-test-plan.md`

That plan includes feature checks, edge cases, and security hardening tests.

---

## 9) Teardown / Reset

Stop:

```bash
./scripts/testing/prod-lab-down.sh
```

Full wipe (containers, volumes, and lab files):

```bash
WIPE=1 ./scripts/testing/prod-lab-down.sh
```

---

## Recommendations to Increase Prod Parity Further

1. Run from installer bundle artifacts in CI (`scripts/build-installers.sh`) and start from extracted bundle instead of source compose.
2. Keep a dedicated test domain and real TLS cert (`SERVER_CERT_MODE=external`) for browser/operator flows.
3. Add a nightly lab gate that runs `prod-lab-smoke.sh` plus key cases from `prod-docker-test-plan.md`.
4. Add one test run with `LAB_BUILD=0` and pinned prebuilt images to validate immutable image deploy behavior.
