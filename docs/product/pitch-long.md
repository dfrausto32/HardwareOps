# Parcel: Fleet Management for Hardware That Lives in the Real World

---

## The Problem

Your software team has excellent tools for deploying to the cloud. CI/CD pipelines, container orchestration, infrastructure-as-code — the cloud-native ecosystem has largely solved deployment for servers in data centers.

But most hardware is not in a data center.

Autonomous devices — embedded controllers, edge nodes, industrial equipment, field sensors, robotics platforms — operate in environments that are bandwidth-constrained, intermittently connected, or fully offline. They sit behind firewalls you don't control. They're physically inaccessible. And when you're managing hundreds or thousands of them, the stakes on a bad update are significant.

The typical tooling for this problem is SSH scripts, USB transfers, or ad-hoc deployment jobs with no rollback. These approaches work until they don't, and when they fail at fleet scale, the blast radius is large and recovery is slow.

**Parcel closes this gap.**

---

## What Parcel Does

Parcel is a control-plane platform for managing software and configuration on autonomous devices at scale. It gives your team a centralized, secure, policy-driven system for delivering updates to a fleet — whether those devices are cloud-connected, operating on intermittent links, or running dark.

### Device Enrollment and Cryptographic Identity

Every device in a Parcel-managed fleet gets a cryptographic identity at enrollment time. Devices submit a Certificate Signing Request (CSR) and receive a signed mTLS client certificate. There are no pre-shared secrets, no static API keys burned into firmware.

First-contact workflows can require operator approval before new devices are admitted to the fleet. Parcel detects hardware identity conflicts — a common failure mode when disk images are cloned at scale — and supports automatic certificate rotation with configurable grace windows to prevent fleet-wide outages during key rollovers.

### Artifact Management and Supply Chain Security

Artifacts are versioned, signed packages: `tar.gz` bundles containing a manifest, an optional declarative apply plan, and the files to deploy. Parcel supports application bundles, configuration bundles, data payloads, firmware, and container image references.

Artifacts can be ingested from CI/CD pipelines via presigned upload URLs, pulled from S3 or GCS buckets, or fetched from Artifactory. CI integrations use OIDC workload identity — no long-lived credentials stored in your pipeline. Artifacts can be signed with Ed25519 keys or keyless Cosign/Sigstore signatures, and Parcel can validate in-toto/SLSA attestations to enforce supply-chain provenance requirements.

Trust policy is configurable per deployment, from permissive (warn on unsigned artifacts) to strict (require verified signature with provenance attestation). Enforcement happens at both ingest and apply time.

### Fleet Policy and Desired State

Parcel uses a layered policy model. Operators define what software each device should be running. Per-device overrides take precedence over group policies, which take precedence over global defaults.

This lets you push a new firmware version to a canary group, validate it, then promote it fleet-wide without touching individual device records. Each device's desired state is multi-component and independently tracked: agent version, application, firmware, and configuration can all be managed on different release cadences.

For large-scale operations, Parcel supports bulk group management with shift-select multi-edit and batch API endpoints, as well as CSV-based rollback flows for fleet-wide recovery scenarios.

### On-Device Plan Execution with Rollback

The Parcel agent is a lightweight Go binary that runs as a systemd service on each device. It has minimal dependencies, no CGO, and is designed to be viable on resource-constrained embedded hardware.

The agent applies changes using a declarative `plan.yaml`: pre-apply hook scripts, file rendering with variable substitution, health probes after installation, and automatic rollback if a health check fails. Artifact files are staged and switched atomically via symlinks — a failed update never leaves a device in a partially-applied state.

Critically, **devices pull rather than push**. Parcel works even when devices are behind NAT, on intermittent connections, or temporarily offline. The agent applies the last known desired state and reconciles automatically when connectivity is restored.

### Observability, Audit, and Compliance

Every action in Parcel is recorded in an immutable audit log: artifact published, device enrolled, policy changed, operator logged in — each with timestamp, actor identity, and structured payload. Audit logs are exportable as CSV with configurable retention policies.

The control plane exposes Prometheus metrics, a health summary API, and real-time WebSocket event streams for live fleet monitoring. Device status is tracked and surfaced in the UI: active, stale, offline, or degraded.

---

## Security Architecture

Security in Parcel is structural, not a feature layer added on top.

**Identity and access**: All device communication uses mTLS. Operator authentication supports local credentials, LDAP/Active Directory, and OIDC federation (Okta, Azure Entra ID, Google Workspace). TOTP MFA is available for local accounts. Service tokens for CI/CD systems are scoped to specific operations (`artifact.publish`, `deployment.trigger`) and can be revoked independently.

**Secrets management**: Credential resolution for cloud artifact sources integrates with HashiCorp Vault and AWS Secrets Manager, with an abstraction layer that supports additional providers.

**Role-based access control**: Viewer, operator, and admin roles are enforced across both the API and UI. Break-glass workflows are audited with a required reason field. Local auth recovery includes recovery codes, reset tokens, and a break-glass CLI path.

**Vulnerability scanning**: Parcel integrates with Grype and Trivy for artifact scanning and Nessus for device scanning.

---

## Deployment Options

| Model | Stack | Best For |
|---|---|---|
| **On-Premises** | Systemd + Caddy + Docker Compose | Air-gapped or private deployments |
| **AWS (Vendor-Hosted)** | ECS Fargate + RDS Aurora + S3 + ALB + WAFv2 | Managed, multi-customer cloud |
| **Customer-Hosted AWS** | Same Terraform, customer AWS account | Customer data residency requirements |
| **Multi-Region Federation** | Federated control planes + artifact replication | Global fleets, regional resilience |

The AWS Terraform configuration provisions a full production stack per customer with least-privilege IAM, WAFv2 rules, CloudWatch alarms, and Secrets Manager integration. Multi-region federation is a live capability: artifact uploads replicate automatically, and global desired state pushes fan out to regional planes.

On-premises deployment uses a systemd installer bundle that runs on any Linux server. No Kubernetes required.

---

## CI/CD Integration

Parcel is designed to fit into your existing CI pipeline, not replace it.

GitHub Actions, GitLab CI, and Jenkins integrations are available via OIDC workload identity — no long-lived credentials stored in your CI system. A typical workflow: build your artifact, push it to Parcel via the artifact ingest API, then trigger a deployment to a target device group. Parcel sends a webhook back to CI with deployment status: which devices applied successfully, which rolled back, and why.

Service tokens can be scoped to a single operation, a single artifact name, or a specific deployment target, giving you fine-grained control over what CI systems can do in production.

---

## Maturity and Readiness

Parcel has completed five development phases covering core deployability, operational observability, enterprise authentication, AWS cloud deployment, and multi-region federation:

- **Foundation**: Enrollment, artifact management, desired state, mTLS, signing, cert rotation, backup/restore, and fleet licensing — all production-ready.
- **Operational Maturity**: Audit logs, Prometheus metrics, WebSocket event streaming, artifact lifecycle management, CI ingest, and bulk fleet operations — complete.
- **Enterprise Readiness**: RBAC, OIDC SSO, LDAP/AD auth, TOTP MFA, break-glass workflows, trusted-key artifact verification, SLSA provenance policy, CI workload identity federation, and Vault integration — complete.
- **Cloud Deployment**: AWS reference architecture with per-customer Terraform, WAFv2, CloudWatch alarms, encrypted secrets — complete and deployed.
- **Federated Multi-Region**: Global aggregation plane, artifact federation, and global desired state push — complete. Sync reconciler and aggregate licensing in progress.

---

## Who This Is For

Parcel is built for teams that write software and also manage the hardware it runs on. Robotics companies. Industrial automation teams. Smart infrastructure providers. Embedded systems groups inside larger organizations.

These teams typically have strong software practices (CI/CD, code review, automated testing) but their deployment story for the hardware side lags behind. They're shipping updates manually, without rollback, without audit, and without a fleet-level view of what's actually running where.

Parcel brings the same operational rigor to edge and embedded deployments that cloud-native teams expect for server workloads — without requiring you to rebuild your CI/CD practices or accept the risk of ad-hoc fleet management at scale.

---

## Summary

Devices enroll securely. Artifacts are signed and verified. Policies are declarative and auditable. The agent works offline. When something goes wrong, you have automatic rollback, a health probe gate, and a full audit trail.

That's what production-grade hardware fleet management looks like.

---

*For API documentation, deployment guides, and architecture internals, contact us to schedule a technical deep-dive.*
