# Vendor-Hosted AWS Customer Operator Guide

Use this guide when Parcel is hosted by the vendor in AWS and the customer is only operating the fleet and integrations.

This is not an infrastructure deployment guide. It explains what the customer operator is responsible for and how that differs from on-prem.

## 1. What the customer receives

The vendor should provide:
- app URL, for example `https://app.customer.example.com`
- agent URL, for example `https://agent.customer.example.com`
- initial admin account or account invitation flow
- agent trust material or public trust instructions
- enrollment profile/bootstrap token procedure
- CI integration details if artifact publish is needed

The customer should not need direct AWS access for normal operations.

## 2. Customer responsibilities

Customer operators are responsible for:
- local user/admin management inside Parcel
- enrollment profile creation and first-agent approval
- desired-state management
- artifact upload or CI/repository integration
- artifact trust policy and trusted signing keys, if delegated
- password recovery and admin recovery procedures

The vendor is typically responsible for:
- AWS infrastructure lifecycle
- database and object storage availability
- hosted TLS certificates and DNS for the application
- platform upgrades and hosted backup policy
- central monitoring and incident response

## 3. First login

1. Open the vendor-provided app URL.
2. Log in with the vendor-provided bootstrap admin account.
3. Immediately:
   - rotate the bootstrap password
   - generate recovery codes
   - add a second admin account
   - review trust and auth settings

## 4. Agent onboarding

For first agent onboarding, use `first-agent-onboarding.md`.

The main difference from on-prem:
- you usually do not manage the control-plane host or compose stack
- you only need:
  - the agent URL
  - the trust chain or public CA guidance
  - the enrollment profile token

## 5. Artifact ingest and CI

Use `ci-workflows.md`.

Typical hosted-customer options:
- manual upload in the UI
- CI push using workload identity
- repository pull from Artifactory or another approved source

Recommended:
- use workload identity for CI where possible
- require verified artifacts for controlled environments

When CI workload identity is enabled, the vendor should provide:
- provider name
- expected audience
- allowed repository/project/branch/workflow constraints
- whether GitHub, GitLab, or Jenkins helper templates are the supported path

## 6. Recovery and security

Use `security-and-recovery.md`.

Recommended baseline for hosted customers:
- keep at least two admin accounts
- generate recovery codes for all admins
- use trusted signing keys and verified artifacts
- avoid shared static service tokens when workload identity is available

## 7. What to escalate to the vendor

Escalate platform issues such as:
- app URL unavailable
- agent URL TLS or connectivity failures affecting multiple devices
- backup/restore requests for hosted infrastructure
- hosted certificate rotation issues
- database/object store availability incidents
- suspected AWS-side security incident

Do not escalate normal product operations such as:
- creating groups
- uploading artifacts
- approving pending enrollments
- resetting user passwords

## 8. Hosted vs on-prem integration differences

| Topic | On-prem | Vendor-hosted AWS |
|---|---|---|
| Control-plane deployment | customer runs stack | vendor runs stack |
| TLS / DNS | customer-managed | vendor-managed |
| Backup/restore of platform | customer-managed | usually vendor-managed |
| CI integration | same product APIs/helpers | same product APIs/helpers |
| Agent onboarding | same | same |
| Artifact trust | same | same |
| User/admin recovery | same product flows | same product flows |
