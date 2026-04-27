# Risk Assessment Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All risks to the confidentiality, integrity, and availability of the Parcel platform and customer data.

---

## 1. Purpose

This policy establishes the process for identifying, analyzing, treating, and monitoring information security risks to the Parcel platform. It provides the foundation for SOC 2 CC9 (risk mitigation) and NIST SP 800-171 RA (risk assessment) requirements.

---

## 2. Scope

Covers risks to:
- Customer data stored or processed by the platform (device telemetry, artifact binaries, audit logs)
- Platform availability (control-plane, regional planes, agent connectivity)
- Platform integrity (artifact signing chain, desired state enforcement)
- Confidentiality of operational secrets and configuration

---

## 3. Risk Assessment Process

The risk assessment follows an annual cycle with ad-hoc updates triggered by significant changes (new product features, new vendor relationships, security incidents, or material changes to the threat landscape).

### 3.1 Risk Identification

Risks are identified through:
- Engineering team review sessions (annual, facilitated by Engineering Lead)
- Post-mortem action items from security incidents
- Vulnerability scan findings (Trivy, Grype, Nessus) that cannot be immediately patched
- Threat intelligence from the security community relevant to the platform's technology stack
- Vendor security advisories
- Internal architecture reviews before significant feature launches

### 3.2 Risk Analysis

Each identified risk is analyzed on two dimensions:

**Likelihood** (probability of occurrence in the next 12 months):

| Score | Description |
|-------|-------------|
| 1 — Rare | Would require a sophisticated, targeted attack or highly unlikely failure |
| 2 — Unlikely | Could occur but no active indicators; requires attacker effort |
| 3 — Possible | Has occurred in comparable platforms; plausible with moderate attacker capability |
| 4 — Likely | Has occurred in similar environments in the past 12 months; limited attacker skill required |
| 5 — Almost Certain | Active exploitation observed in the wild or environmental conditions make occurrence expected |

**Impact** (consequence if the risk materializes):

| Score | Description |
|-------|-------------|
| 1 — Negligible | No customer impact; no data exposure; recoverable in minutes |
| 2 — Minor | Limited customer impact; no personal data exposure; recoverable same day |
| 3 — Moderate | Multiple customers affected; potential for personal data exposure; multi-day recovery |
| 4 — Significant | Widespread customer impact; confirmed personal data exposure; extended outage; regulatory notification likely |
| 5 — Catastrophic | Complete platform compromise; mass data breach; reputational damage; potential legal liability |

**Risk Score** = Likelihood × Impact (range 1–25)

| Score Range | Risk Level | Treatment Priority |
|-------------|------------|-------------------|
| 1–4 | Low | Accept or monitor |
| 5–9 | Medium | Treat within 90 days |
| 10–16 | High | Treat within 30 days |
| 17–25 | Critical | Treat immediately |

### 3.3 Risk Treatment

For each risk, one of the following treatment strategies is selected:

| Strategy | Description |
|----------|-------------|
| **Mitigate** | Implement or improve controls to reduce likelihood or impact |
| **Accept** | Risk is within acceptable tolerance; documented with owner and review date |
| **Transfer** | Shift risk via insurance, contractual terms, or vendor responsibility |
| **Avoid** | Eliminate the activity or asset that creates the risk |

Treatment actions are tracked in the risk register (Section 4) with an owner and target completion date.

---

## 4. Risk Register

The risk register is maintained in `docs/risk-register.md`. Each entry contains:

| Field | Description |
|-------|-------------|
| Risk ID | Unique identifier (e.g. `RSK-001`) |
| Description | Plain-language description of the risk |
| Category | One of: Access Control, Data Protection, Availability, Supply Chain, Compliance |
| Likelihood | Score 1–5 |
| Impact | Score 1–5 |
| Risk Score | Likelihood × Impact |
| Risk Level | Low / Medium / High / Critical |
| Treatment Strategy | Mitigate / Accept / Transfer / Avoid |
| Controls in Place | Existing controls that reduce the risk |
| Treatment Actions | Planned mitigations with owner and due date |
| Owner | Person responsible for tracking treatment |
| Status | Open / In Progress / Treated / Accepted |
| Last Reviewed | Date of last review |

The risk register is reviewed and updated:
- Annually (full review by Engineering Lead)
- After any P0 or P1 security incident
- Before significant feature launches or infrastructure changes
- When a new high/critical vulnerability is identified in platform dependencies

---

## 5. Initial Risk Register — Baseline Entries

The following risks are identified as of the policy effective date and are captured in `docs/risk-register.md`:

| Risk ID | Description | Likelihood | Impact | Score | Level | Treatment |
|---------|-------------|-----------|--------|-------|-------|-----------|
| RSK-001 | Compromised service token used to publish malicious artifact | 2 | 5 | 10 | High | Mitigate — scoped tokens, audit log, signature verification |
| RSK-002 | RDS credential exposure via misconfigured IAM or Secrets Manager | 2 | 4 | 8 | Medium | Mitigate — least-privilege IAM, Secrets Manager rotation |
| RSK-003 | Agent binary supply-chain compromise via unsigned artifact | 2 | 5 | 10 | High | Mitigate — Ed25519 signing enforced, Rekor transparency log |
| RSK-004 | Availability loss from single-AZ RDS failure | 2 | 3 | 6 | Medium | Mitigate — Multi-AZ deployment, 14-day automated backups |
| RSK-005 | Unauthorized access via stolen operator JWT | 3 | 4 | 12 | High | Mitigate — TOTP MFA, 12h TTL, audit log, login backoff |
| RSK-006 | CVE in third-party dependency exploited in production | 3 | 3 | 9 | Medium | Mitigate — continuous Trivy/Grype scanning, 14-day patch SLA |
| RSK-007 | Insider threat: admin account misuse | 1 | 5 | 5 | Medium | Mitigate — full audit log, quarterly access review, RBAC |
| RSK-008 | Data loss from accidental artifact deletion without backup | 1 | 3 | 3 | Low | Accept — audit log records deletions; S3 versioning available |
| RSK-009 | Formal risk assessment not yet performed (process gap) | 5 | 2 | 10 | High | Mitigate — this policy and risk register close the gap |
| RSK-010 | Export control violation: restricted party accesses platform | 2 | 4 | 8 | Medium | Mitigate — WAFv2 geo-block, EAR self-classification, ToS clause pending |

---

## 6. Risk Acceptance

Risks accepted rather than mitigated must be:
- Documented in the risk register with the acceptance rationale
- Reviewed at least annually
- Approved by the Engineering Lead
- Flagged for re-evaluation if threat landscape changes materially

---

## 7. Communication

- The risk register is shared with the Engineering Lead and relevant stakeholders annually.
- High and Critical risks are escalated to the CEO/founder immediately on identification.
- Risk register findings inform audit preparation, vendor management decisions, and product roadmap prioritization.

---

## 8. Related Documents

- `docs/risk-register.md` — live risk register
- `docs/policies/vendor-management-policy.md` — vendor risk assessment
- `docs/policies/vulnerability-management-policy.md` — vulnerability-driven risk inputs
- `docs/policies/incident-response-policy.md` — risk materialization response
- `docs/compliance-status.md` — compliance gap tracking
