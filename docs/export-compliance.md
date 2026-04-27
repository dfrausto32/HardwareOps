# Export Compliance — EAR Self-Classification

**Prepared:** 2026-04-03  
**Review cadence:** Annually or when cryptographic capabilities change materially

This document records Parcel's self-classification under the U.S. Export Administration Regulations (EAR) and describes the obligations that flow from that classification. It is an internal reference and does not constitute legal advice. Consult export counsel before entering new markets or changing the platform's cryptographic architecture.

---

## 1. Classification

| Field | Value |
|-------|-------|
| **Product** | Parcel (control-plane, agent, and web console) |
| **ECCN** | **5D002.c.1** — Software that uses encryption for confidentiality |
| **License exception** | **ENC — Section 740.17(b)(1)** — Unrestricted encryption software for consumer/commercial use |
| **AT controls** | Yes (Anti-Terrorism) — applies to all ECCN 5D002 items |
| **NS controls** | Yes (National Security) — satisfied by License Exception ENC |

### Basis for 5D002 classification

Parcel incorporates the following encryption:

| Use | Algorithm | Key length |
|-----|-----------|-----------|
| TLS (all control-plane and agent communication) | TLS 1.2/1.3 (AES-256, ChaCha20) | 256-bit |
| mTLS device identity | RSA (cert issuance), ECDSA | 2048-bit / P-256 |
| Artifact signing | Ed25519 | 256-bit |
| Webhook secret encryption | AES-256-GCM | 256-bit |
| Password hashing | bcrypt | N/A (KDF) |
| Keyless artifact signing (Cosign/Sigstore) | ECDSA (P-256) | 256-bit |

All cryptographic implementations are sourced from the Go standard library (`crypto/tls`, `crypto/ecdsa`, `crypto/ed25519`, `crypto/aes`) and the `golang.org/x/crypto` module. No proprietary or custom cryptographic algorithms are used.

### License Exception ENC eligibility (740.17(b)(1))

ENC (b)(1) applies to encryption software that:
- Is generally available to the public (or will be)
- Does not provide non-standard cryptographic functionality
- Uses algorithms and key lengths that appear on the ENC Annex

Parcel meets all three criteria. The software uses only standard, publicly documented algorithms; no algorithm is proprietary; key lengths are within the standard ENC Annex parameters.

---

## 2. Obligations Under License Exception ENC

### Annual self-classification report (Section 740.17(e)(3))

Companies using License Exception ENC must submit an annual self-classification report to BIS and NSA by **February 1** of each year. The report covers products that were exported under the exception during the prior calendar year.

**Submission:** File via the BIS Simplified Network Application Process Redesign (SNAP-R) system at https://snapr.bis.doc.gov. Select form BIS-748P and choose "encryption registration/annual report."

**Action item:** Submit initial classification report for calendar year 2026 by February 1, 2027. Calendar a recurring annual reminder.

### No license required for most destinations

Under ENC (b)(1), Parcel may be exported without a license to any destination **except** embargoed countries (see Section 3) and any entity on BIS's Entity List, Denied Persons List, or OFAC's SDN List.

### Record-keeping

EAR requires transaction records to be kept for **5 years** from the date of export. For a SaaS download product, this means retaining:
- Records of customer onboarding (email, entity name, jurisdiction)
- Any Know Your Customer (KYC) checks performed
- Evidence of screening against restricted-party lists

---

## 3. Prohibited Destinations and Parties

### Embargoed countries (comprehensive embargo — no exports without a license)

| Country | Basis |
|---------|-------|
| Cuba | OFAC Cuban Assets Control Regulations |
| Iran | OFAC Iranian Transactions and Sanctions Regulations |
| North Korea | OFAC North Korea Sanctions Regulations |
| Russia | BIS/OFAC Russia-related sanctions (broad since 2022) |
| Syria | OFAC Syrian Sanctions Regulations |

**Platform control:** WAFv2 geo-block rules are deployed for all five countries by default (`waf_blocked_country_codes` in the ALB Terraform module). This does not substitute for legal screening — a VPN can bypass geo-blocking — but satisfies the technical due-diligence expectation.

### Restricted-party screening

Before onboarding an enterprise customer, screen the entity name and key personnel against:
- [BIS Entity List](https://www.bis.gov/supplemental-no-1-consolidated-screening-list) (part of the Consolidated Screening List)
- [OFAC SDN List](https://ofac.treasury.gov/sanctions-list-service)
- [BIS Denied Persons List](https://www.bis.gov/denied-persons-list)

Free consolidated screening is available via the [CISA Consolidated Screening List API](https://api.trade.gov/consolidated_screening_list/search). For high-volume or automated screening, a commercial OFAC screening service (e.g., Dow Jones, Kharon) is appropriate.

---

## 4. Terms of Service Language

The following clause must appear in Parcel's Terms of Service (or End User License Agreement). It satisfies the EAR requirement that the exporter provide notice of export restrictions and obtain user acknowledgement.

> **Export Controls.** The Software is subject to U.S. export control laws, including the Export Administration Regulations (EAR). You agree that you will not export, re-export, or transfer the Software, directly or indirectly, to (a) any country or territory subject to a U.S. comprehensive embargo (currently Cuba, Iran, North Korea, Russia, and Syria), (b) any person or entity on the U.S. Department of Commerce Denied Persons List or Entity List, the U.S. Treasury Department OFAC Specially Designated Nationals List, or any other applicable restricted-party list, or (c) for any end use prohibited by U.S. law. By using the Software, you represent and warrant that you are not located in, under the control of, or a national or resident of any such country, and that you are not on any such list.

**Action item:** Insert this clause into the Terms of Service before any commercial distribution outside the United States.

---

## 5. Cryptographic Change Control

This classification must be reviewed if any of the following occur:

- A new cryptographic algorithm or key exchange mechanism is introduced
- Encryption is added to a component that previously had none (e.g., agent local state encryption)
- The product is forked into a variant targeting a government or classified environment
- A material change to the EAR or ENC exception is published by BIS

Changes to cryptographic architecture should be noted in the pull request description and flagged to the export compliance owner for re-evaluation.

---

## 6. Responsible Parties

| Function | Owner |
|----------|-------|
| Annual BIS/NSA self-classification report | _TBD — Legal / Compliance_ |
| Restricted-party screening for enterprise onboarding | _TBD — Sales / Legal_ |
| ToS export clause maintenance | _TBD — Legal_ |
| Cryptographic change review | Engineering lead |
