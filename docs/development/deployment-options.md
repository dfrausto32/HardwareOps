# Deployment Options (On‑Prem + AWS)

This doc outlines **supported deployment shapes** and the recommended path forward.
Assumption: single‑tenant per customer deployment (multi‑tenant can be added later).

## Recommendation (now)
- **On‑prem** as the default production target.
- **AWS ECS Fargate** as the cloud target (lowest ops, fastest to ship).

## On‑Prem Deployment (customer‑hosted)
**Use when:** customer requires local control, air‑gapped or low‑latency environment.

**Shape (current + next):**
- Control‑plane + UI adjacent on a single server
- Postgres (local or managed by customer)
- MinIO/S3‑compatible object store (local)
- Optional CoreDNS for internal DNS

**Why this works now:**
- Fits the current installer bundle model.
- Easy to secure with TLS + mTLS.
- Minimal external dependencies.

**Operational needs:**
- Backups: Postgres + MinIO
- CA rotation plan
- Host monitoring (CPU, disk, agent check‑ins)

## AWS Deployment (cloud)
**Use when:** customer wants managed infrastructure and internet‑reachable control‑plane.

### Recommended AWS shape (Phase 2)
- **ECS Fargate**: control‑plane and UI containers
- **RDS (Postgres)**: managed database
- **S3**: artifact storage
- **ALB**: TLS termination
- **ACM**: managed certificates
- **CloudWatch**: logs + metrics

**Why ECS Fargate:**
Lowest ops for a small team, fast iteration, no Kubernetes overhead.

### Alternative shapes
- **EKS**: best for large customers with existing Kubernetes ops.
- **EC2 + Docker Compose**: simplest ops, least scalable; good for POCs.

## DNS + TLS (both modes)
- Use a **customer‑owned domain** (e.g., `hardwareops.internal`).
- Install a **local CA** on all agents + operator machines.
- Control‑plane serves HTTPS; agents use **mTLS** with device certs.

## Upgrade Path
1. On‑prem installs for early customers.
2. AWS (ECS Fargate) for managed option.
3. Optional EKS for enterprise customers.
