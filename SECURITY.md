# Security Policy

## Reporting a Vulnerability

If you believe you have found a security vulnerability in Parcel, please report it to us privately. **Do not open a public GitHub issue.**

**Contact:** security@parcel.io

We will acknowledge your report within **48 hours** and provide a resolution timeline within **7 business days**. We aim to patch confirmed vulnerabilities within **90 days** of disclosure.

---

## Scope

The following are in scope for security reports:

- Control-plane API (`control-plane/`)
- Device agent (`agent/`)
- Web console (`ui/`)
- AWS Terraform deployment modules (`deploy/aws/terraform/`)
- Authentication, authorization, and mTLS certificate handling
- Artifact signing and verification pipeline
- Supply-chain integrity controls

The following are **out of scope**:

- Denial-of-service attacks requiring large volumes of traffic
- Attacks requiring physical access to a device
- Vulnerabilities in third-party dependencies already tracked in their own CVE advisories (report upstream; we will patch our dependency pin)
- Findings from automated scanners submitted without a demonstrated proof of concept

---

## Preferred Report Format

Please include:

1. A description of the vulnerability and its potential impact
2. Steps to reproduce (including any relevant environment, version, or configuration details)
3. Any proof-of-concept code or a description of the attack chain
4. Your suggested severity (CVSS score if available)

---

## Disclosure Policy

- We request **90 days** from initial contact before public disclosure, following the industry-standard coordinated disclosure model.
- If a fix is available sooner, we will coordinate an earlier disclosure date with you.
- We will credit reporters in release notes unless you prefer to remain anonymous.
- We do not currently operate a bug bounty program.

---

## Export Restrictions

Parcel includes cryptographic software and is subject to U.S. Export Administration Regulations (EAR) under ECCN 5D002. Use, export, re-export, or transfer of this software to or within embargoed countries or to sanctioned parties is prohibited under applicable U.S. law.

---

## Supported Versions

Security fixes are applied to the current release only. We do not backport patches to older versions.

| Version | Supported |
|---------|-----------|
| Latest  | Yes |
| Older   | No |
