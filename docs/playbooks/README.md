# Deployment Playbooks

Situation-specific runbooks for deploying Parcel to AWS. Each playbook includes a **When to Use** section at the top — read that first to pick the right one.

| Playbook | Scenario |
|---|---|
| [managed-aws-hosted.md](managed-aws-hosted.md) | Parcel provisions and operates the AWS environment on the customer's behalf |
| [customer-self-hosted-aws.md](customer-self-hosted-aws.md) | Customer deploys and owns their own AWS environment; you provide the software and optionally engineering hours |
| [iso-fleet-provisioning.md](iso-fleet-provisioning.md) | Deploying many machines from a shared OS image (ISO / golden image / disk clone) |

## Quick decision

**You are deploying and managing the stack** → `managed-aws-hosted.md`

**Customer is deploying the stack themselves** → `customer-self-hosted-aws.md`

**Deploying agents to many machines from a shared image** → `iso-fleet-provisioning.md`
