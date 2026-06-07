# Parcel: Software Deployment for Hardware That Isn't in the Cloud

---

Your team has great tools for deploying to servers. Kubernetes, CI/CD pipelines, cloud infrastructure — all solved problems.

But your hardware is not in a data center. It's in the field, behind firewalls you don't control, on intermittent connections, sometimes fully offline. And when something needs to change — a firmware update, a config push, a new application binary — the tooling most teams reach for wasn't built for this.

**Parcel was.**

---

## What Parcel Gives You

**Secure device enrollment** — Every device gets a cryptographic identity (mTLS client cert) at enrollment. No pre-shared secrets. No static API keys burned into firmware. First-contact approval workflows available.

**Artifact management with supply-chain controls** — Versioned, signed packages with Ed25519 or Cosign/SLSA signatures. Integrated with GitHub Actions, GitLab CI, and Jenkins via OIDC workload identity. Pull directly from S3, GCS, or Artifactory.

**Declarative desired state with rollback** — Define what should run on each device or device group. The agent applies changes atomically with health probes and automatic rollback. A bad update never leaves a device partially applied.

**Offline-first agent** — Devices pull rather than push. The agent works behind NAT, on intermittent links, or fully offline — it applies the last known desired state and reconciles when connectivity returns.

**Full audit trail and observability** — Every action logged with actor identity and timestamp. Prometheus metrics, real-time WebSocket event streams, and CSV-exportable audit logs built in.

**Enterprise authentication** — Local auth, LDAP/Active Directory, OIDC SSO (Okta, Entra ID, Google Workspace), TOTP MFA, RBAC, Vault/Secrets Manager integration. Works in air-gapped environments.

---

## Deployment Options

- **On-premises** — systemd installer, no Kubernetes required. Runs air-gapped.
- **AWS (vendor-hosted)** — managed, per-customer cloud deployment with Terraform.
- **Customer-hosted AWS** — same Terraform stack in the customer's own account.
- **Multi-region federation** — global control plane above regional deployments, with artifact replication and fleet-wide policy push.

---

## Who It's For

Robotics companies. Industrial automation teams. Smart infrastructure providers. Any team that writes software and manages the hardware it runs on — and whose deployment story for the device side hasn't kept up with their software practices.

---

## Where Things Stand

All core capabilities are production-ready: enrollment, artifact management, desired state, mTLS, signing, RBAC, SSO, CI/CD integration, audit, cert rotation, backup/restore, and AWS deployment. Multi-region federation is live and actively extended.

---

**If your team is shipping updates manually, without rollback, without audit, and without a fleet-level view of what's running where — that's exactly the problem Parcel solves.**

*Get in touch to see a demo or schedule a technical deep-dive.*
