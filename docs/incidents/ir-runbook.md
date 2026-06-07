# Incident Response Runbook

**Owner:** Platform Operations  
**Review cadence:** Annually or after any P0/P1 incident

This runbook governs how Parcel responds to security incidents and operational outages. It covers severity classification, response procedures, breach notification obligations, and communication templates.

Individual incident notes live alongside this file using the `YYYY-MM-DD-short-title.md` naming convention.

---

## 1. Severity Classification

| Severity | Definition | Examples |
|----------|-----------|---------|
| **P0 — Active Incident** | Confirmed breach, data exfiltration in progress, or complete service outage affecting production | Unauthorized access to the database; mass device command injection; control-plane unreachable |
| **P1 — Suspected Incident** | Strong indicators of compromise or a single-customer-impacting outage with no confirmed root cause | Anomalous audit log patterns; unexpected admin account creation; cert revocation failure; partial service degradation |
| **P2 — Vulnerability Discovered** | Security flaw confirmed but not actively exploited; no evidence of data access | CVE in a dependency with a viable exploit path; misconfigured WAF rule; expired signing key |
| **P3 — Operational Issue** | Non-security degradation or bug with no compliance impact | Webhook delivery history gap; UI rendering error; slow query |

When uncertain between P0 and P1, **treat as P0** until evidence rules it out.

---

## 2. Roles

| Role | Responsibility |
|------|---------------|
| **Incident Commander (IC)** | Owns the response end-to-end; makes containment and communication decisions; declares severity |
| **Technical Lead** | Drives investigation, containment, and remediation in the codebase and infrastructure |
| **Communications Lead** | Drafts and sends customer notices, coordinates with legal; owns the external narrative |
| **Legal / Compliance** | Assesses notification obligations; approves external communications |
| **Executive Escalation** | Notified immediately for any P0; available for P1 within 2 hours |

Assign roles at the start of the incident. One person can hold multiple roles for P2/P3.

---

## 3. Response Playbooks

### P0 — Active Incident

**Target:** Containment within 1 hour of declaration.

1. **Declare** — IC declares P0, opens a dedicated incident channel (Slack `#incident-YYYY-MM-DD`), and pages all roles
2. **Preserve evidence** — Before taking any containment action, snapshot affected RDS instance and CloudWatch logs; export relevant audit events via `GET /api/v1/audit` to a secure, offline location
3. **Contain**
   - Rotate compromised credentials immediately (JWT secret, service tokens, SMTP password, DB password) via AWS Secrets Manager; trigger ECS task replacement to pick up new secrets
   - If device fleet is affected: revoke compromised mTLS certificates via the cert revocation API; push a new CA bundle to ALB trust store
   - If control-plane is compromised: enable maintenance mode (`MAINTENANCE_MODE=1`) to halt inbound agent traffic; evaluate whether to take the service offline entirely
   - Block the attacker at the WAF layer if source IPs are known (`aws wafv2 update-ip-set`)
4. **Assess scope** — Determine what data was accessed: device records, artifact payloads, user credentials, or audit logs
5. **Remediate** — Patch the root cause; deploy a clean image; verify with the hardened-profile acceptance gate (`scripts/aws-hardening-check.sh`)
6. **Notify** — See Section 4 for breach notification timelines; Communications Lead sends initial customer notice within 24 hours of confirmed breach
7. **Post-mortem** — Written within 5 business days; stored in `docs/incidents/YYYY-MM-DD-short-title.md`

---

### P1 — Suspected Incident

**Target:** Root-cause determination within 4 hours; escalate to P0 or downgrade to P2/P3.

1. **Triage** — IC assembles Technical Lead; begins audit log review (`GET /api/v1/audit?limit=500`) and CloudWatch Insights query for anomalous patterns
2. **Isolate** — If a specific user account or service token is suspect, disable it immediately (`PATCH /api/v1/users/{id}` with `disabled: true`; delete the service token)
3. **Investigate** — Review: authentication events, IP addresses, audit actor fields, cert fingerprints, and WAF sampled requests in CloudWatch
4. **Escalate or downgrade** — If evidence of actual compromise is found, immediately escalate to P0; if investigation rules out compromise, downgrade to P2 or P3 and document findings
5. **Post-mortem** — Written within 10 business days; stored in `docs/incidents/`

---

### P2 — Vulnerability Discovered

**Target:** Patch plan within 48 hours; patch deployed within 90 days (or sooner per disclosure agreement).

1. **Assess** — Determine CVSS score, exploit prerequisites, and affected versions
2. **Plan** — Identify the fix; assess whether a hotfix release or the next scheduled release is appropriate
3. **Patch** — Implement fix; add a regression test
4. **Deploy** — Roll out to production; verify with acceptance gate
5. **Disclose** — If reported externally via `security@parcel.io`, coordinate disclosure timeline with reporter per the VDP in `SECURITY.md`; if discovered internally, document in release notes
6. **Record** — Add an entry to `docs/incidents/` if the vulnerability had meaningful production exposure

---

### P3 — Operational Issue

Follow the standard engineering bug process. No IR procedures required unless it escalates.

---

## 4. Breach Notification Obligations

### Determining notification is required

Notification is required when there is a **reasonable belief** that personal information of individuals was acquired by an unauthorized party. This includes: names, email addresses, hashed passwords, device metadata tied to individuals, or audit records.

If in doubt, consult Legal before deciding not to notify.

### Federal baseline

| Regulation | Trigger | Timeline |
|-----------|---------|---------|
| FTC Act (Section 5) | Breach of consumer data | No federal deadline; notify promptly |
| GLBA Safeguards Rule | Breach of customer financial information | 30 days to notify affected customers |

### State law summary (U.S.)

All 50 U.S. states have breach notification laws. The most common requirements:

| Jurisdiction | Notice to individuals | Notice to state AG |
|-------------|----------------------|-------------------|
| California (CCPA/CPRA) | "Expedient" / most restrictive in practice | AG if >500 CA residents |
| New York (SHIELD Act) | "Expedient" | AG if >500 NY residents |
| Colorado | 30 days | 30 days if >500 residents |
| Texas | "Expedient" | AG if 250+ Texas residents |
| Most other states | 30–90 days | Varies |

**Practical default:** Notify affected individuals within **30 days** of confirmed breach; notify applicable state AGs within **30 days** where required. Engage Legal to determine which states' laws apply based on the residency of affected users.

### Notification content requirements

Notices must generally include:
- Date of the breach (or estimated date range)
- Nature of the information exposed
- What the company has done or is doing to address the breach
- What steps affected individuals can take to protect themselves
- Contact information for questions

Use the template in Section 6.

---

## 5. Escalation Contacts

> **Note:** Replace placeholders with real contact information before this runbook goes live.

| Role | Name | Contact |
|------|------|---------|
| Incident Commander (primary) | _TBD_ | _TBD_ |
| Incident Commander (backup) | _TBD_ | _TBD_ |
| Technical Lead | _TBD_ | _TBD_ |
| Communications Lead | _TBD_ | _TBD_ |
| Legal / Compliance counsel | _TBD_ | _TBD_ |
| Executive escalation | _TBD_ | _TBD_ |
| AWS Support (if infrastructure involved) | — | AWS Support Console (Business/Enterprise plan required for phone) |

Security reports from external parties arrive at: **security@parcel.io** — this inbox must be monitored 24/7 or have an on-call forward configured.

---

## 6. Communication Templates

### Initial customer notice (breach confirmed)

> **Subject: Important Security Notice from Parcel**
>
> We are writing to inform you of a security incident that may have affected your account.
>
> **What happened:** [Brief description — e.g., "On [date], we discovered unauthorized access to [system]."]
>
> **What information was involved:** [e.g., "Device identifiers, audit records, and/or email addresses associated with your Parcel account."]
>
> **What we have done:** [e.g., "We immediately contained the incident by [actions]. We have patched the underlying issue and are continuing to monitor for any further activity."]
>
> **What you can do:** [e.g., "We recommend rotating any API service tokens associated with your account. If you use local authentication, we recommend changing your password. Instructions are available at [link]."]
>
> If you have questions, contact us at security@parcel.io.
>
> We take the security of your data seriously and apologize for any concern this may cause.
>
> — The Parcel Team

---

### Internal P0 declaration (Slack)

> 🔴 **P0 INCIDENT DECLARED** — [Date/time UTC]
>
> **Summary:** [One sentence describing what is known]
> **IC:** @[name]
> **Tech Lead:** @[name]
> **Evidence channel:** [link to log snapshot or ticket]
> **Next update:** [time — max 30 minutes out]
>
> All response discussion in this channel. Do not discuss externally until Communications Lead authorizes.

---

## 7. Post-Mortem Template

Post-mortems are blameless. File as `docs/incidents/YYYY-MM-DD-short-title.md`.

```
# Incident Post-Mortem: [Title]

**Date:** YYYY-MM-DD  
**Severity:** P0 / P1 / P2  
**Duration:** [detection time] → [resolution time]  
**IC:** [name]

## Summary
[Two to three sentences: what happened, what was affected, how it was resolved.]

## Timeline (UTC)
- HH:MM — [event]
- HH:MM — [event]

## Root Cause
[Specific technical cause.]

## Impact
[What data or systems were affected and for how long.]

## Containment & Remediation
[Actions taken, in order.]

## What Went Well
[Things that helped the response.]

## What Could Be Improved
[Gaps in tooling, process, or communication.]

## Action Items
| Item | Owner | Due |
|------|-------|-----|
| [task] | [name] | YYYY-MM-DD |
```

---

## 9. Ransomware Response Playbook

> Declare **P0** immediately. Do not wait for confirmation of encryption — bulk deletion of artifacts is treated as ransomware until proven otherwise.

### Step 1 — Contain (target: within 15 minutes of detection)

1. **Enable maintenance mode** to halt all inbound agent traffic and stop desired-state pushes to the fleet:
   - AWS: update ECS task environment `MAINTENANCE_MODE=1`; trigger a new task deployment.
   - On-prem: set `MAINTENANCE_MODE=1` in `.env.onprem`; restart the control-plane service.
2. **Revoke the suspected account or token** immediately:
   - Service token: `DELETE /api/v1/service-tokens/{id}`
   - User account: `PATCH /api/v1/users/{id}` with `{"disabled": true}`
3. **Snapshot RDS before touching anything else** — preserve forensic state:
   ```bash
   aws rds create-db-snapshot \
     --db-instance-identifier <prod-instance-id> \
     --db-snapshot-identifier ransomware-forensic-$(date +%Y%m%d%H%M)
   ```
4. **Export audit log** for the past 24–48 hours to a secure offline location:
   ```bash
   curl -H "Authorization: Bearer $ADMIN_TOKEN" \
     "$BASE_URL/api/v1/audit?limit=1000" > /tmp/audit-export-$(date +%Y%m%d%H%M).json
   ```
5. **Do not delete any objects** from MinIO/S3 — metadata and access logs are forensic evidence.

### Step 2 — Determine Scope

Query the audit log for deletion events in the blast window:

```bash
# Filter for artifact.delete and artifact.prune events
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/v1/audit?action=artifact.delete&limit=500" | jq .

curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/api/v1/audit?action=artifact.prune&limit=500" | jq .
```

Determine:
- Which actor (user ID, service token ID, source IP) performed the deletions
- Time window of the event
- Which artifact names and versions were affected
- Whether database records (devices, desired state) were also modified

### Step 3 — Assess Fleet Impact

1. Check whether any devices are currently attempting to apply artifacts that no longer exist — look for apply-result failures in the runtime event stream.
2. Determine whether a malicious artifact was pushed to desired state before containment. If yes, the desired state must be reverted before lifting maintenance mode.
3. For each affected device group, confirm the current desired artifact version resolves to an intact object in the store.

### Step 4 — Recover

**If S3 Object Lock (R-01) is in place:**
- Artifacts within the retention window are not deletable — confirm object integrity via S3 inventory or `mc ls --recursive`.
- Recovery is typically a database restore (PITR) to re-point desired state to the intact objects.

**If Object Lock is not in place:**
- Restore artifact objects from isolated backup (R-05).
- If backup is unavailable or also compromised, re-ingest artifacts from source CI systems using `scripts/artifact-e2e.sh` or the CI ingest API.

**Database recovery:**
```bash
# RDS PITR — restore to a point 5 minutes before the earliest audit event showing bulk deletion
aws rds restore-db-instance-to-point-in-time \
  --source-db-instance-identifier <prod-instance-id> \
  --target-db-instance-identifier <recovery-instance-id> \
  --restore-time <ISO-8601-timestamp>
```

Follow the full restore drill procedure in `docs/compliance/policies/backup-and-recovery-policy.md` §4.1.

### Step 5 — Verify and Lift Maintenance Mode

1. Run `./scripts/artifact-e2e.sh` against the recovered environment.
2. Confirm artifact object count and SHA256 checksums match pre-event expectations.
3. Verify a representative sample of devices can check in and receive their desired state.
4. Confirm the compromised credential is fully revoked and the attack vector is closed.
5. Disable maintenance mode and monitor for recurrence for 48 hours.

### Step 6 — Notify and Post-Mortem

- Follow breach notification obligations in Section 4.
- File post-mortem per Section 7 within 5 business days.
- Update `docs/compliance/risk-register.md` with post-incident status for RSK-013 through RSK-015.

---

## 8. Key Platform Controls for Incident Response

| Need | How to do it in Parcel |
|------|----------------------|
| Export audit log | `GET /api/v1/audit?limit=500&offset=0` (paginate); export to CSV via UI |
| Disable a user account | `PATCH /api/v1/users/{id}` with `{"disabled": true}` |
| Revoke a service token | `DELETE /api/v1/service-tokens/{id}` |
| Revoke a device certificate | Certificate revocation via serial number in the cert management API |
| Enable maintenance mode | Set `MAINTENANCE_MODE=1` in ECS task env; trigger new deployment |
| Rotate JWT secret | Update secret in Secrets Manager; ECS task replacement picks it up |
| Block IPs at WAF | `aws wafv2 update-ip-set --id <id> --addresses <ip-list>` |
| Run acceptance gate | `./scripts/aws-hardening-check.sh` |
