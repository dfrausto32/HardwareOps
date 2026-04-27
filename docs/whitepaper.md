# Parcel: Fleet Control for the Real World
### A Technical White Paper

---

## The Problem With Hardware in Production

Software teams have excellent tools for deploying to servers. Kubernetes, Terraform, CI/CD pipelines: the cloud-native ecosystem has largely solved the problem of "how do we ship code to a machine we control in a data center."

The problem is most hardware is not in a data center.

Autonomous devices (embedded controllers, edge nodes, industrial equipment, remote sensors) operate in environments that are bandwidth-constrained, intermittently connected, or fully offline. They run on hardware with limited compute. They sit behind firewalls you don't control. And they're often in the field, physically inaccessible, sometimes in the thousands.

When something needs to change (a firmware update, a configuration push, a new application binary) the tooling most teams reach for wasn't built for this. Shell scripts over SSH, manual USB transfers, ad-hoc deployment jobs with no rollback: these approaches work until they don't, and when they fail in a distributed fleet, the blast radius is significant.

**Parcel was built to close this gap.**

---

## What Parcel Is

Parcel is a control-plane platform for managing software and configuration on autonomous devices at scale. It gives development teams a centralized, secure, policy-driven system for delivering artifacts to a fleet, regardless of whether those devices are cloud-connected or dark.

The platform has three components:

- **Control Plane**: A Go API server backed by PostgreSQL and object storage. This is the authoritative source of truth for device identity, artifact inventory, desired state, and audit history.
- **Agent**: A lightweight Go binary that runs on each device. It checks in periodically, fetches its desired state, applies changes using a declarative plan, and reports results. No inbound ports required. No persistent cloud connection required.
- **Web Console**: A React-based UI for operators and engineers to manage devices, publish artifacts, define policies, and monitor fleet health.

The key design decision is that **devices pull, not push**. This means Parcel works even when devices are behind NAT, on intermittent connections, or temporarily offline. The agent applies the last known desired state and reconciles when connectivity is restored.

---

## Core Capabilities

### Device Enrollment and Identity

Devices enroll by generating a Certificate Signing Request (CSR) and exchanging it for a signed mTLS client certificate. There are no pre-shared secrets, no static API keys burned into firmware. Each device gets a cryptographic identity at enrollment time and uses it for all subsequent communication.

Operators can configure first-contact approval workflows, where new devices are held in a pending state until reviewed. Parcel detects hardware identity conflicts (a common failure mode when disk images are cloned), and supports automatic certificate rotation with grace windows to prevent fleet-wide outages during key rollovers.

### Artifact Management

Artifacts are versioned, signed packages: `tar.gz` bundles containing a manifest, an optional declarative apply plan, and the files to be deployed. Parcel supports several artifact types out of the box: application bundles, configuration bundles, data payloads, firmware, and container image references.

Artifacts can be ingested from CI/CD pipelines via presigned upload URLs, pulled from S3 or GCS buckets, or fetched from Artifactory. Parcel integrates with GitHub Actions, GitLab CI, and Jenkins via OIDC workload identity, with no long-lived credentials stored in CI. Published artifacts can be signed with Ed25519 keys or keyless Cosign signatures (Fulcio-based), and Parcel can validate in-toto/SLSA attestations to enforce supply-chain provenance requirements.

Once in the system, artifacts follow a managed lifecycle: active, deprecated, and pruned. Storage is handled by MinIO (or native S3 in cloud deployments).

### Desired State and Fleet Policy

Parcel uses a layered policy model. Operators define what software each device should be running. Per-device overrides take precedence over group policies, which take precedence over global defaults. This lets you push a new firmware version to a canary group, validate it, and then promote it fleet-wide without touching individual device records.

Each device's desired state is multi-component and independent: agent version, application, firmware, and configuration can all be managed separately on different release cadences.

### On-Device Plan Execution

The Parcel agent applies changes using a declarative plan defined in `plan.yaml`. This includes pre-apply hook scripts, file rendering with variable substitution, health probes after installation, and automatic rollback if a health check fails. Artifact files are staged and switched atomically via symlinks, which means a failed update never leaves a device in a half-applied state.

The agent is designed to run as a systemd service on embedded Linux. It has minimal dependencies, no CGO, and is intentionally small to be viable on resource-constrained hardware.

### Audit, Observability, and Compliance

Every action in Parcel (artifact published, device enrolled, policy changed, operator logged in) is recorded in an immutable audit log with timestamp, actor identity, and a structured payload. Audit logs are exportable as CSV and subject to configurable retention policies.

The control plane exposes Prometheus metrics, a health summary API, and real-time WebSocket event streams for live fleet monitoring. Device status is tracked and surfaced in the UI: active, stale, offline, or degraded.

---

## Security Architecture

Security in Parcel is not a feature; it is a structural property of the system.

**Identity**: All device communication uses mTLS. Operator authentication supports local credentials, LDAP/Active Directory, and OIDC federation (Okta, Azure Entra ID, Google Workspace). Service tokens for CI/CD are scoped to specific operations (`artifact.publish`, `deployment.trigger`, etc.) and can be revoked independently.

**Supply chain**: Artifact trust policy is configurable per deployment, ranging from permissive (warn on unsigned artifacts) to strict (require verified Ed25519 or Cosign signature with provenance attestation). Trust enforcement happens at both ingest and apply time.

**Secrets**: Credential resolution for cloud artifact sources integrates with HashiCorp Vault and AWS Secrets Manager, with an abstraction layer that supports additional providers. Operator-triggered reload refreshes credentials without restarting the control plane.

**Access control**: Role-based access control with viewer, operator, and admin roles is enforced across both the API and the UI. Break-glass workflows are audited with a required reason field.

---

## Deployment Options

| Model | Stack | Best For |
|---|---|---|
| **Local / Dev** | Docker Compose (Postgres + MinIO) | Development, testing |
| **On-Premises** | Systemd + Caddy + Docker Compose | Air-gapped or private deployments |
| **AWS** | ECS Fargate + RDS Aurora + S3 + ALB + WAFv2 | Cloud-hosted, multi-customer |
| **Multi-Region** | Federated control planes + artifact replication | Global fleets, regional resilience |

The AWS Terraform configuration provisions a full production stack per customer, per region, with least-privilege IAM, WAFv2 rules, CloudWatch alarms, and Secrets Manager integration included. Multi-region federation is an active capability: artifact uploads replicate automatically, and global desired state pushes fan out to regional planes.

---

## Who This Is For

Parcel is built for teams that write software and also manage the hardware it runs on. This is increasingly common: robotics companies, industrial automation teams, smart infrastructure providers, embedded systems groups inside larger organizations.

These teams typically have strong software engineering practices (CI/CD, code review, automated testing), but their deployment story for the hardware side of the house lags significantly behind. They're often shipping updates manually, without rollback, without audit, and without a clear fleet-level view of what's running where.

Parcel brings the same operational rigor to edge and embedded deployments that cloud-native teams have come to expect for server workloads. You get a proper artifact registry, a policy engine, cryptographic device identity, a feedback loop back into your CI pipeline, and a full audit trail, all in a system designed from the ground up for devices that are not always online.

---

## Current Maturity

Parcel has completed five development phases covering core deployability, operational observability, enterprise authentication, cloud deployment, and multi-region federation. All foundational capabilities (enrollment, artifact management, desired state, RBAC, SSO, CI/CD integration, audit, and mTLS) are production-ready. Active development continues on AWS deployment hardening and the remaining federation phases (enrollment profiles, PKI hierarchy, and aggregate licensing across regions).

---

## Summary

If your team builds software that runs on hardware, you already know the deployment problem. Parcel solves it without requiring you to rebuild your CI/CD practices from scratch or accept the risk of ad-hoc fleet management at scale.

Devices enroll securely. Artifacts are signed and verified. Policies are declarative and auditable. The agent works offline. And when something goes wrong, you have a rollback, a health probe, and a full audit trail telling you exactly what happened.

That's what production-grade hardware fleet management looks like.

---

*For API documentation, deployment guides, and architecture internals, see the [Integration Control Document](icd.md), [Deployment Guide](deploy.md), and [Operations Runbook](operations.md).*
