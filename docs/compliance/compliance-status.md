# Parcel — Compliance & Standards Status
**Date:** 2026-04-03
**Scope:** Assessment of implemented vs. not-yet-implemented compliance controls across all platform tiers

This document maps the compliance landscape relevant to a B2B SaaS software distribution platform against what Parcel has already built, what is partially addressed, and what is explicitly out of scope or not yet implemented.

---

## How to Read This Document

| Symbol | Meaning |
|--------|---------|
| ✅ Implemented | Control exists in the codebase and is operational |
| ⚠️ Partial | Meaningful progress; gaps remain |
| ❌ Not implemented | No evidence in codebase; gap exists |
| N/A | Not applicable to current product scope |

---

## 1. Core Baseline (Any SaaS / Software Distribution)

### PCI DSS
**Status: N/A — by design**

Parcel does not handle payment card data. There is no payment processing code, no credit card fields, and no Stripe or payment gateway integration in the platform itself. If a billing portal is added in the future, PCI scope should be minimized by delegating to a hosted checkout provider (Stripe, etc.), which would reduce compliance obligation to SAQ A.

---

### NIST SP 800-218 (Secure Software Development Framework — SSDF)
**Status: ⚠️ Partial**

| Practice | Status | Evidence |
|----------|--------|---------|
| Protect all forms of code from unauthorized access and tampering | ✅ | Ed25519 artifact signing (`artifacttrust/`), mTLS device identity, service token scoping |
| Produce well-secured software | ✅ | Hardened profile (`config/hardening.go`) enforces security invariants at startup |
| Respond to vulnerability reports | ✅ | `SECURITY.md` at repo root — VDP with 48h acknowledgement, 90-day disclosure timeline, scope, and `security@parcel.io` contact |
| Archive and protect release integrity | ✅ | Artifact lifecycle management, Rekor transparency log integration, signed artifact registry |

**Status upgraded to ✅ Complete** — `SECURITY.md` published. Outstanding: `security@parcel.io` mailbox must be created before the repo goes public (tracked in roadmap work queue).

---

### NIST SP 800-63B (Authentication & Identity Assurance)
**Status: ✅ Implemented** (MFA gap closed 2026-04-02)

| Control | Status | Evidence |
|---------|--------|---------|
| Memorized secret verifiers (bcrypt, no hints) | ✅ | `auth.go` — passwords stored as bcrypt hashes, no plaintext |
| Multi-factor authentication | ✅ | TOTP (RFC 6238 / SHA-1, 30-second window) implemented: `auth/totp.go`, `handlers/auth_totp.go`, migration `0036`; secrets encrypted AES-256-GCM at rest; two-step login flow via `totp_pending` JWT |
| Account lockout / throttling | ✅ | `ratelimit.go` — login rate limiting with configurable RPM and optional exponential backoff |
| Minimum password length | ✅ | Hardened profile enforces ≥ 12 chars for bootstrap password |
| Password expiration | ❌ | No forced rotation policy or configurable expiry |
| Session management | ✅ | Configurable JWT TTL (default 12h), hardened profile caps at 24h |

OIDC-federated logins inherit MFA from the identity provider. For local accounts, TOTP provides a second factor when `TOTP_ENCRYPTION_KEY` is configured.

---

### NIST SP 800-52 Rev.2 (TLS Guidelines)
**Status: ✅ Implemented**

- TLS is enforced for all control-plane endpoints (`ENABLE_TLS=1`)
- mTLS is the mandatory authentication channel for devices
- TLS configuration supports custom CA chains (`TLSClientCA`, `CACertPath`)
- Global-plane sync client enforces CA-pinned TLS (`globalplane/sync/client.go`)
- SMTP integration supports certificate verification (hardened profile disallows InsecureSkipVerify)

---

## 2. Software Distribution & Supply Chain Security

### Code Signing
**Status: ✅ Implemented**

Parcel has a mature artifact signing and verification pipeline:

- **Ed25519 key-based signing** — trusted key registry configurable via file, JSON, or AWS Secrets Manager
- **Keyless signing (Cosign/Fulcio)** — `artifacttrust/provenance.go` validates ECDSA signatures via Fulcio-issued short-lived certificates
- **Rekor transparency log** — signed entry timestamps (SETs) verified both online and offline; builder identity extracted from Sigstore OID extensions
- **Policy enforcement** — configurable from permissive (warn) to strict (require signature on ingest and apply); hardened profile mandates strict enforcement

---

### SBOM (Software Bill of Materials)
**Status: ✅ Implemented**

- CycloneDX SBOM generated automatically on artifact ingest using `trivy fs --format cyclonedx`
- Runs as a background job (`sbom.ArtifactSBOMJob`) — same pattern as vulnerability scanning; does not block the ingest response
- SBOM stored in MinIO at `sboms/{artifactID}.cdx.json` alongside the artifact
- `sbomObjectKey` field returned on artifact API responses once generation completes
- Download via `POST /artifacts/{id}/sbom/presign` (returns a presigned MinIO URL, audited)
- Enabled with `SBOM_ENABLED=1`; binary path configurable via `SBOM_BIN`; types to skip via `SBOM_SKIP_TYPES`
- New migration `0035_artifact_sbom.sql` adds `sbom_object_key` column to the `artifacts` table

---

### Dependency / Vulnerability Scanning (SCA)
**Status: ✅ Implemented**

- **Trivy** — integrated scanner (`vulnscan/trivy.go`)
- **Grype** — integrated scanner (`vulnscan/grype.go`)
- **Nessus** — integration for infrastructure-level scanning (`vulnscan/nessus.go`)
- Scan jobs are scheduled and triggered on artifact ingest
- Results are persisted and surfaced via the API
- Scanner is configurable via `VULN_ARTIFACT_SCANNER` env var

---

## 3. Data Protection & Incident Response

### FTC Safeguards Rule
**Status: ⚠️ Partial**

The Safeguards Rule requires a written information security program with designated personnel, risk assessment, and specific technical safeguards for customer financial data. Parcel does not handle financial data directly, which significantly reduces exposure. However, the rule's general data protection requirements (access controls, encryption, audit) are largely addressed:

| Requirement | Status |
|-------------|--------|
| Access controls | ✅ RBAC (`viewer`, `operator`, `admin`), scoped service tokens, OIDC federation |
| Encryption in transit | ✅ TLS everywhere, mTLS for devices |
| Encryption at rest | ✅ RDS: `storage_encrypted = true` hardcoded on both instance variants; S3: SSE always active (AWS-KMS CMK by default, AES256 fallback); Secrets Manager: AWS-managed encryption; ECS ephemeral storage: Fargate-encrypted. See `docs/guides/deployment-hardening.md` §7 |
| Audit logging | ✅ Full audit trail with actor, IP, action, target, timestamp |
| Incident response plan | ✅ `docs/incidents/ir-runbook.md` — severity ladder, P0/P1/P2 playbooks, breach notification timelines, escalation contacts (contacts TBD), and communication templates |

---

### State Data Breach Notification Laws
**Status: ⚠️ Partial**

Technical controls for detection and logging are in place. However:

- ✅ IR runbook published at `docs/incidents/ir-runbook.md` with breach notification timelines and communication templates
- ❌ Escalation contacts in runbook Section 5 are still placeholder — must be populated before first production deployment

**Note:** `docs/incidents/` directory exists in the repository — this is the correct location for IR procedures.

---

### Audit Logging
**Status: ✅ Implemented**

Parcel maintains a structured, persistent audit log for all significant platform actions:

- Every event records: timestamp, actor type (user/device/service token/workload identity), actor ID, email, roles, auth method, source IP, user agent, request ID, action, target, and outcome
- Stored in PostgreSQL `audit_events` table
- Configurable TTL-based retention (`DeleteAuditEventsBefore`)
- Exportable (CSV noted in whitepaper)

---

### Backup & Recovery
**Status: ✅ Implemented**

- `backup/runner.go` supports local, Docker-containerized, and remote backup execution modes
- Configurable backup command, working directory, and log output
- Supports separate Postgres and MinIO container backup targets
- Remote runner API with token authentication
- Status tracking (idle / running / success / failed) and timestamped log files

**Gap:** No automated backup verification or restore-drill documentation.

---

## 4. Enterprise / Sales-Enabling

### SOC 2 Type I / II
**Status: ⚠️ Partial — policy layer complete; audit engagement not yet initiated**

SOC 2 is an audit report, not a technical implementation. The underlying security controls that a Type II audit would assess are substantially present, and the formal policy layer is now in place:

| Trust Service Criterion | Technical Status | Policy Status |
|------------------------|-----------------|--------------|
| CC6 — Logical and physical access controls | ✅ RBAC, mTLS, service tokens, OIDC, TOTP MFA | ✅ `docs/compliance/policies/access-control-policy.md` |
| CC7 — System operations (monitoring, anomalies) | ✅ Audit log, Prometheus metrics, WebSocket event streams | ✅ `docs/compliance/policies/incident-response-policy.md` |
| CC8 — Change management | ✅ Artifact versioning, lifecycle management, audit trail | ✅ `docs/compliance/policies/change-management-policy.md` |
| CC9 — Risk mitigation | ✅ Vuln scanning + formal risk register | ✅ `docs/compliance/policies/risk-assessment-policy.md`, `docs/compliance/risk-register.md` |
| A1 — Availability (backup, recovery) | ✅ Backup runner, health APIs, RDS PITR | ✅ `docs/compliance/policies/backup-and-recovery-policy.md` |
| C1 — Confidentiality | ✅ Encryption at rest and in transit (AWS + on-prem) | ✅ `docs/compliance/policies/vendor-management-policy.md` |

**Remaining to pursue SOC 2 Type I:** Engage an AICPA-accredited auditor; populate IR runbook escalation contacts; execute first quarterly restore drill. Type II additionally requires ≥ 6 months of operation under audit observation.

---

### ISO/IEC 27001
**Status: ❌ Not yet obtained**

ISO 27001 requires a formal Information Security Management System (ISMS) with documented policies, risk treatment plans, internal audits, and management review cycles. Technical controls are a prerequisite, not the whole certification. No formal ISMS documentation is present in this repository.

---

## 5. Defense / Government Customers

### NIST SP 800-171 (CUI Protection)
**Status: ⚠️ Partial — technical controls present, formal assessment absent**

800-171's 110 controls are grouped into 14 families. Parcel's implementation status by family:

| Family | Status | Notes |
|--------|--------|-------|
| Access Control (AC) | ✅ | RBAC, scoped tokens, mTLS |
| Audit & Accountability (AU) | ✅ | Full audit log |
| Configuration Management (CM) | ✅ | Artifact versioning, desired state, hardened profile |
| Identification & Authentication (IA) | ✅ | Strong auth + TOTP MFA for local accounts; OIDC inherits IdP MFA |
| Incident Response (IR) | ⚠️ | IR runbook published; escalation contacts still TBD |
| Maintenance (MA) | N/A | Physical maintenance out of scope |
| Media Protection (MP) | ❌ | No media sanitization controls |
| Personnel Security (PS) | ❌ | Out of scope for software platform |
| Physical Protection (PE) | N/A | AWS shared-responsibility model |
| Risk Assessment (RA) | ✅ | `docs/compliance/risk-register.md` + `docs/compliance/policies/risk-assessment-policy.md` |
| Security Assessment (CA) | ❌ | No formal self-assessment or third-party assessment |
| System & Communications Protection (SC) | ✅ | TLS, mTLS, network segmentation via Terraform |
| System & Information Integrity (SI) | ✅ | Vuln scanning, artifact signing, audit |
| Program Management | ⚠️ | 7 formal policies in `docs/compliance/policies/`; no appointed ISSO; formal ISMS not yet established |

---

### CMMC (Cybersecurity Maturity Model Certification)
**Status: ❌ Not assessed**

CMMC Level 2 maps directly to NIST 800-171. CMMC Level 3 adds NIST 800-172 practices. No self-assessment or third-party certification has been performed. The technical controls required for Level 2 are substantially present (see 800-171 table above); the gap is the formal assessment and documentation layer.

---

### FedRAMP
**Status: ❌ Not applicable at current stage**

FedRAMP authorization requires an agency sponsor, a 3PAO assessment, and ongoing continuous monitoring. This is a multi-year, high-cost process appropriate only when directly contracting to provide SaaS to federal agencies. Not a near-term requirement.

---

## 6. Export & Legal Compliance

### EAR (Export Administration Regulations)
**Status: ⚠️ Partial**

| Obligation | Status | Notes |
|-----------|--------|-------|
| Self-classification documented | ✅ | `docs/compliance/export-compliance.md` — ECCN 5D002.c.1, License Exception ENC 740.17(b)(1) |
| Annual BIS/NSA SNAP-R report | ❌ | Due February 1, 2027 (first filing); tracked in roadmap work queue |
| ToS export control clause | ❌ | Clause drafted in `docs/compliance/export-compliance.md` Section 4 — not yet in published ToS |
| Restricted-party screening | ❌ | Process documented; not yet operationalized |
| Embargoed country blocking | ✅ | WAFv2 geo-block rule covers all 5 OFAC-embargoed countries |

---

### OFAC Sanctions Compliance
**Status: ⚠️ Partial**

| Control | Status | Notes |
|---------|--------|-------|
| IP-based geo-blocking for embargoed countries | ✅ | WAFv2 `BlockSanctionedCountries` rule deployed by default in ALB module (`waf_blocked_country_codes = ["CU","IR","KP","RU","SY"]`) |
| ToS prohibition clause | ❌ | Clause drafted in `docs/compliance/export-compliance.md` Section 4 — not yet inserted into published Terms of Service |
| Restricted-party (SDN) screening for enterprise onboarding | ❌ | No automated screening; manual process not yet defined |

---

## 7. Additional Controls Implemented (Not in Original List)

These controls exist in Parcel and strengthen the overall compliance posture beyond what was explicitly scoped above.

| Control | Evidence |
|---------|---------|
| **Hardware-bound device identity** | `device_identity.go` — TPM/serial/IMEI binding, conflict detection |
| **Certificate revocation** | `mtls.go` — serial-based revocation checking at every device request |
| **Workload identity federation** | `workload_identity.go` — GitHub Actions OIDC token exchange; no long-lived CI secrets |
| **Hardened security profile** | `config/hardening.go` — 16 invariants enforced at startup; fails fast if violated |
| **Scoped service tokens** | `service_token.go` — least-privilege CI tokens with per-capability scopes |
| **License enforcement** | `license/license.go` — Ed25519-signed license with device cap and validity window |
| **Artifact provenance attestations** | `handlers/attestations.go` — SLSA/in-toto attestation storage and retrieval |
| **Webhook secret encryption** | `webhooks/crypto.go` — AES-256-GCM encrypted at rest |

---

## 8. Compliance Gap Summary

### Gaps Requiring Technical Work

| Gap | Priority | Estimated Effort |
|-----|----------|-----------------|
| Password expiration policy | Low | Low |

### Gaps Requiring Process / Documentation Work (open actions)

| Gap | Status | Location |
|-----|--------|---------|
| Create `security@parcel.io` inbox | ❌ Open | Roadmap work queue |
| Populate IR runbook escalation contacts (Section 5) | ❌ Open | `docs/incidents/ir-runbook.md` |
| Insert export control clause into Terms of Service | ❌ Open | Clause at `docs/compliance/export-compliance.md` §4 |
| Submit annual BIS/NSA SNAP-R report | ❌ Open — due 2027-02-01 | `docs/compliance/export-compliance.md` §2; roadmap work queue |
| Assign export compliance responsible parties | ❌ Open | `docs/compliance/export-compliance.md` §6 |
| Formal risk assessment | ✅ Done | `docs/compliance/risk-register.md` + `docs/compliance/policies/risk-assessment-policy.md` |
| SOC 2 policy layer | ✅ Done | 7 policies in `docs/compliance/policies/` covering CC6–CC9, A1, C1 |
| SOC 2 audit engagement | ❌ Open | Policy docs complete; select AICPA-accredited auditor and begin Type I engagement |
| Backup verification / restore drills | ❌ Open | Procedure documented in `docs/compliance/policies/backup-and-recovery-policy.md`; first drill not yet executed |

---

## 9. Recommended Phased Roadmap

### Phase 1 — Close Immediate Gaps (0–3 months)
1. ✅ Publish a security contact and VDP policy — `SECURITY.md`
2. ✅ Write an incident response runbook — `docs/incidents/ir-runbook.md`
3. ✅ Enable WAFv2 geo-blocking for OFAC-embargoed countries — `modules/alb` Terraform
4. ✅ Document EAR self-classification — `docs/compliance/export-compliance.md`
5. ✅ Add SBOM generation to the artifact ingest pipeline — `internal/sbom/`, migration `0035`

### Phase 2 — Win Enterprise Deals (3–9 months)
1. ✅ Verify and document encryption at rest — `docs/guides/deployment-hardening.md` §7
2. ✅ Implement TOTP-based MFA for operator accounts — `auth/totp.go`, `handlers/auth_totp.go`, migration `0036`
3. ✅ SOC 2 policy documentation — 7 policies in `docs/compliance/policies/`, risk register in `docs/compliance/risk-register.md`
4. ✅ Quarterly vulnerability assessment process — `docs/compliance/policies/vulnerability-management-policy.md`

### Phase 3 — Win Government / Defense Deals (9–18 months)
1. Complete NIST 800-171 self-assessment and document POA&M
2. Pursue SOC 2 Type II (requires 6-month observation window)
3. Evaluate CMMC Level 2 readiness assessment if DoD pipeline materializes
4. Consider ISO 27001 if international enterprise sales require it

---

*This document reflects the state of the codebase as of 2026-04-03. It is not a legal opinion or official compliance certification. Engage qualified counsel and auditors for formal assessments.*
