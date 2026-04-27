# Vendor Management Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All third-party vendors, cloud providers, and open-source dependencies that process, store, or transmit Parcel platform data or that are operationally critical to the platform.

---

## 1. Purpose

This policy establishes requirements for selecting, onboarding, monitoring, and offboarding vendors to ensure that third-party relationships do not introduce unacceptable risk to the Parcel platform or customer data.

---

## 2. Scope

Covers:
- Cloud infrastructure providers (e.g. AWS)
- Open-source software integrated into the platform (Go modules, npm packages)
- SaaS tools used in platform operations (monitoring, CI/CD, source control)
- Any third party that receives or could access customer data

---

## 3. Vendor Inventory

All critical vendors are listed in the table below. This table is the authoritative vendor register and is updated whenever a new vendor is onboarded or an existing vendor is offboarded.

| Vendor | Category | Data Processed | Criticality | SOC 2 / Compliance | Review Date |
|--------|----------|---------------|-------------|-------------------|-------------|
| Amazon Web Services (AWS) | Cloud infrastructure | All platform data (RDS, S3, ECS, Secrets Manager) | Critical | SOC 2 Type II, ISO 27001, FedRAMP High | Annual |
| GitHub | Source control, CI/CD | Source code, CI secrets (no customer data) | High | SOC 2 Type II | Annual |
| MinIO (open-source) | Object storage (on-prem) | Artifact binaries, SBOMs | High | Open-source; no external data processing | Annual |
| PostgreSQL (open-source) | Database (on-prem) | All platform data | High | Open-source; no external data processing | Annual |
| Sigstore / Rekor | Artifact transparency log | Artifact signing metadata (public log) | Medium | Open infrastructure; logs are public by design | Annual |

Additional vendors are added via the onboarding process in Section 4.

---

## 4. Vendor Onboarding

Before engaging a new vendor that will process, store, or transmit customer data or that is operationally critical:

1. **Risk assessment:** Engineering Lead evaluates the vendor against criteria in Section 5.
2. **Compliance review:** Obtain and review the vendor's SOC 2 report, ISO 27001 certificate, or equivalent. Where these are unavailable (e.g. smaller vendors), complete a vendor security questionnaire.
3. **Data Processing Agreement (DPA):** If the vendor processes personal data of EU residents, a DPA must be in place before data flows.
4. **Approval:** Engineering Lead approves onboarding. For vendors accessing production customer data, approval also requires the CEO/founder.
5. **Registration:** Vendor is added to the inventory table above.
6. **Least-privilege access:** Vendor is granted only the access required for their service. Access is provisioned per `docs/policies/access-control-policy.md`.

---

## 5. Vendor Risk Assessment Criteria

| Criterion | Assessment Questions |
|-----------|---------------------|
| Data exposure | Does the vendor process, store, or have access to customer personal data or production secrets? |
| Criticality | Would a vendor outage or breach cause a platform outage or data exposure? |
| Compliance posture | Does the vendor hold SOC 2 Type II, ISO 27001, or FedRAMP? Is their report current (< 12 months)? |
| Subprocessors | Does the vendor use subprocessors? Are they disclosed? Do they have equivalent compliance posture? |
| Incident response | Does the vendor have a published VDP or responsible disclosure process? What is their breach notification timeline? |
| Data residency | Does the vendor store data in jurisdictions consistent with customer agreements? |

---

## 6. Ongoing Vendor Monitoring

| Activity | Frequency | Owner |
|----------|-----------|-------|
| Review AWS SOC 2 report | Annual | Engineering Lead |
| Review GitHub security advisories and trust center | Annual | Engineering Lead |
| Scan open-source dependencies for CVEs (Trivy/Grype) | Continuous (on artifact ingest) | Automated |
| Review Go module updates for new indirect dependencies | Per PR | Code reviewer |
| Review npm audit findings | Per PR | Code reviewer |
| Confirm vendor is still in business and contract is current | Annual (vendor review cycle) | Engineering Lead |

Findings from the annual vendor review are documented in `docs/vendor-reviews/` with date, findings, and any remediation actions.

---

## 7. Open-Source Dependency Management

Open-source libraries are treated as vendors. The following controls apply:

- **Dependency pinning:** `go.sum` and `package-lock.json` are committed to ensure reproducible builds.
- **Vulnerability scanning:** Trivy runs on all artifact ingests; critical findings in platform dependencies are patched within 14 days.
- **License review:** New Go modules and npm packages are reviewed for license compatibility (permissive licenses preferred; GPL-licensed packages require Engineering Lead approval before inclusion).
- **Removal:** Unused dependencies are removed on a best-effort basis during each quarterly dependency review.

---

## 8. Vendor Offboarding

When a vendor relationship ends or a service is migrated:

1. Revoke all API keys, tokens, and access credentials the vendor held.
2. Confirm data deletion or return per the vendor contract.
3. Remove the vendor from the inventory table.
4. Document the offboarding in `docs/vendor-reviews/`.

---

## 9. AWS Shared Responsibility Model

As the primary cloud provider, AWS is responsible for security **of** the cloud (physical infrastructure, hypervisor, managed service internals). Parcel is responsible for security **in** the cloud:

| AWS Responsibility | Parcel Responsibility |
|-------------------|----------------------|
| Physical data center security | IAM role scoping and least-privilege policy |
| Hypervisor and host OS | ECS task hardening (no ECS exec in production) |
| Managed service encryption (RDS, S3, Secrets Manager) | Key management and rotation policy |
| DDoS protection (Shield Standard) | WAFv2 rules, rate limiting, geo-blocking |
| Availability Zones | Multi-AZ RDS, ECS service placement |

---

## 10. Related Documents

- `docs/policies/access-control-policy.md` — vendor access provisioning
- `docs/policies/risk-assessment-policy.md` — risk register and treatment
- `docs/deployment-hardening.md` §7 — AWS encryption-at-rest evidence
- `docs/export-compliance.md` — third-party transfer obligations under EAR
