# Parcel Customer Documentation Bundle Source

This folder is the source for the customer-facing documentation bundle.

It is intended to be:
- zipped and sent directly to customers
- included in control-plane and stack build outputs
- kept focused on setup, integration, and day-2 operations

To build a distributable zip from the current repo state:

```bash
./scripts/build-customer-docs-bundle.sh
```

Authored docs in this folder:
- `setup-onprem.md` - initial on-prem control-plane + UI setup
- `setup-vendor-hosted-aws.md` - customer operator guide for vendor-hosted AWS deployments
- `first-agent-onboarding.md` - first agent install and approval onboarding
- `ci-workflows.md` - CI publish/pull workflows and workload identity
- `operations.md` - backup, restore, upgrade, certificate rotation
- `security-and-recovery.md` - artifact trust, auth recovery, and hardening notes

Assembled into the customer bundle at build time:
- `icd.md` - client integration contract (assembled into the bundle)
- `ci-templates/` - scaffold templates for GitHub Actions, GitLab CI, and Jenkins
- `ci-helpers/` - helper scripts for push/pull, workload identity exchange, and smoke validation

Recommended reading order:
1. `setup-onprem.md`
2. `setup-vendor-hosted-aws.md`
3. `first-agent-onboarding.md`
4. `ci-workflows.md`
5. `operations.md`
6. `security-and-recovery.md`
7. `icd.md`

This bundle is intentionally concise. For internal engineering detail, use the full repo docs, not this folder.

Deployment model selection:

| Model | Customer owns | Vendor owns |
|---|---|---|
| On-prem | host, Docker/runtime, backups, cert placement, upgrades | product bundle and release guidance |
| Vendor-hosted AWS | agent rollout, artifact trust policy, CI integration, user/access management | AWS stack, DNS/certs for hosted app, database/object store, platform operations |
