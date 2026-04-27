# Incident Response Policy
**Version:** 1.0  
**Effective Date:** 2026-04-06  
**Owner:** Engineering Lead  
**Review Cycle:** Annual (next review 2027-04-06)  
**Applies To:** All security incidents, service disruptions, and data breaches affecting the Parcel platform or customer data.

---

## 1. Purpose

This policy establishes the organizational commitment to detecting, containing, eradicating, and recovering from security and operational incidents. It formalizes the tactical procedures documented in `docs/incidents/ir-runbook.md` into a standing company policy.

---

## 2. Scope

Covers any event that:
- Compromises or threatens the confidentiality, integrity, or availability of customer data
- Results in unauthorized access to the Parcel control-plane, infrastructure, or source code
- Causes unplanned service degradation or outage
- Indicates active exploitation of a vulnerability in the platform
- Triggers a breach notification obligation under applicable law

---

## 3. Incident Severity Classification

| Severity | Criteria | Initial Response Target |
|----------|----------|------------------------|
| **P0 — Critical** | Active data breach; unauthorized access to production data; complete service outage; active exploit in the wild against a shipped vulnerability | Immediate (within 15 minutes of detection) |
| **P1 — High** | Credible compromise attempt; partial service degradation affecting multiple customers; cryptographic key or secret suspected compromised | Within 1 hour |
| **P2 — Medium** | Isolated service error; single-customer impact; anomalous but unconfirmed activity | Within 4 hours (business hours) |
| **P3 — Low** | Minor bug with security implications; informational finding from external report | Within 2 business days |

---

## 4. Roles and Responsibilities

| Role | Responsibility |
|------|---------------|
| **Incident Commander (IC)** | Owns the incident from declaration to closure; coordinates all response activities; makes go/no-go decisions on containment and recovery actions |
| **Technical Lead** | Performs technical investigation and remediation; operates platform tooling; owns timeline of technical actions |
| **Communications Lead** | Manages internal and external communications; drafts customer notifications; coordinates with Legal |
| **Legal / Compliance** | Advises on breach notification obligations; approves external communications; manages regulatory filings |
| **Engineering Lead** | Escalation path for resource or authority decisions; approves emergency changes during incidents |

Specific named contacts for each role are maintained in `docs/incidents/ir-runbook.md` Section 5 and must be updated before first production deployment.

---

## 5. Incident Response Lifecycle

### 5.1 Detection and Declaration

Incidents may be detected via:
- CloudWatch alarms (availability, error rate, latency)
- Audit log anomalies (unusual actor, unusual action, unexpected volume)
- Vulnerability Disclosure Program reports (via `security@parcel.io`)
- Customer reports
- Automated vulnerability scan findings indicating active exploitation
- Internal engineering observation

Any engineer may declare an incident. On declaration:
1. Open an incident channel (Slack `#incidents` or equivalent).
2. Page the on-call engineer if outside business hours.
3. Assign an Incident Commander.
4. Create an incident record (see Section 5.5).

### 5.2 Containment

The IC and Technical Lead determine the appropriate containment action based on severity:

- **Isolate** affected ECS services or devices (maintenance mode via `MAINTENANCE_MODE=1` or ECS service stop)
- **Revoke** compromised credentials, service tokens, or device certificates via platform API or Secrets Manager rotation
- **Block** source IPs via WAFv2 rule or security group update
- **Preserve** evidence — snapshot affected RDS instance, export relevant audit log entries, capture CloudWatch logs before rotation

Do not destroy evidence during containment. Snapshots and log exports take priority.

### 5.3 Eradication

After containment:
1. Identify and eliminate the root cause (patch vulnerability, revoke compromised access, fix misconfiguration).
2. Apply the fix via the emergency change process (`docs/policies/change-management-policy.md` Section 6).
3. Confirm the attack vector is closed before moving to recovery.

### 5.4 Recovery

1. Restore service from a known-good state (prior ECS task definition, RDS snapshot, or clean deployment).
2. Verify platform integrity: run `scripts/artifact-e2e.sh` and core enrollment/checkin paths against the recovered environment.
3. Monitor for recurrence for at least 48 hours post-recovery.
4. Lift maintenance mode once the IC is satisfied the platform is stable.

### 5.5 Incident Record

Every declared incident must have a written record in `docs/incidents/` containing:
- Incident ID and severity
- Timeline of events (detection, containment, eradication, recovery, closure)
- Technical actions taken (with timestamps and actors)
- Root cause
- Customer impact (scope and duration)
- Breach notification determination (see Section 6)
- Post-mortem findings and action items

---

## 6. Breach Notification

### 6.1 Internal Escalation

On any incident where unauthorized access to customer personal data is confirmed or reasonably suspected:
1. IC notifies Legal/Compliance immediately (within 1 hour of determination).
2. Legal/Compliance determines whether a notification obligation exists under applicable law.

### 6.2 Notification Timelines

| Jurisdiction / Law | Deadline |
|--------------------|----------|
| U.S. state breach notification laws (default) | 30 days from discovery (varies by state; 30 days is a conservative default) |
| GDPR Article 33 (if EU personal data is involved) | 72 hours from discovery to supervisory authority |
| Contractual SLA obligations | Per customer agreement (check MSA/DPA) |

### 6.3 Customer Notification

Customer notifications are drafted by the Communications Lead, reviewed by Legal, and approved by the IC before sending. Notification content includes:
- Date and nature of the incident
- Categories of data affected
- Steps taken to contain and remediate
- Recommended customer actions (if any)
- Contact for questions (`security@parcel.io`)

Templates are in `docs/incidents/ir-runbook.md` Section 6.

---

## 7. Post-Mortem

A blameless post-mortem is conducted for all P0 and P1 incidents, and optionally for P2 incidents with significant learning potential:
- Held within 5 business days of incident closure
- Attended by IC, Technical Lead, Engineering Lead, and any engineers involved in response
- Output: documented root cause, timeline, what went well, what to improve, and time-bounded action items assigned to owners
- Post-mortem record stored in `docs/incidents/`

---

## 8. Testing and Training

- The IR runbook is reviewed annually (aligned with this policy's review cycle).
- A tabletop exercise simulating a P1 incident scenario is conducted annually.
- Results and action items from tabletop exercises are documented in `docs/incidents/`.

---

## 9. Related Documents

- `docs/incidents/ir-runbook.md` — tactical playbooks for P0/P1/P2 scenarios
- `docs/policies/access-control-policy.md` — access revocation procedures
- `docs/policies/change-management-policy.md` — emergency change process
- `SECURITY.md` — vulnerability disclosure program and external reporting
