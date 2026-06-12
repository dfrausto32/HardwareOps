# Tech E&O Application — Open Questions

Fill in answers below (or reply in chat) and I'll apply them to `Tech E&O App - filled.pdf`.

## 1. General Information
- Legal name of Applicant (exact legal entity name): ___
- Street address / City, State, Zip: ___
- Phone: ___  Fax (if any): ___
- Website (is it parcel.io? the repo references security@parcel.io): ___
- Are you applying for General Liability coverage too? If yes: square footage of all owned/leased locations: ___

## 2. Form of Business
- Entity type (Individual / Corporation / Partnership / Other e.g. LLC): ___
- Date established: ___
- Total number of employees: ___
- Any subsidiaries or affiliated entities? (app asks for an attached list): ___

## 3. Revenues
- Current fiscal year end (month/day) and projected gross revenue: ___
- Last fiscal year revenue: ___
- Two fiscal years ago revenue: ___

## 4. Records
- Approximate number of unique electronic records containing personal info (user accounts, emails, etc. across your environments): ___
  (I marked 4.a "Yes" because the platform stores usernames, passwords, and email addresses, and set paper records to 0 — correct me if wrong.)

## 5. IT Department (answered by the person responsible for network security — you?)
- Confirm name "Dylan Frausto" and email fraustofrontiercoding@gmail.com: ___
- Title: ___  Phone: ___
- IT security designations/certifications (if any): ___
- Number of IT personnel: ___  Dedicated IT security personnel: ___
  (I marked network security as "Managed internally/in-house".)

## 6. Information & Network Security Controls
- 6.b — Is MFA enforced on all cloud provider services (your AWS account(s), and anything else like GitHub, Google Workspace)? Y/N: ___
- 6.c — Is all sensitive/confidential info encrypted at rest on your systems (incl. laptops)? Y/N: ___
  (If No: are servers with sensitive data segregated? Role-based access control? — platform has RBAC, so likely Y for the second.)

## 7. Ransomware Controls (these are about YOUR org/endpoints, not just the product)
- a — Email pre-screening for malicious attachments/links? Y/N. Provider (dropdown: Avanan, Barracuda, Cisco, Microsoft Defender, Mimecast, Proofpoint, SonicWall, Symantec, Trend Micro, Other — Gmail/Google Workspace would be "Other"): ___
  - Sandbox detonation of attachments before delivery? Y/N: ___
- b — Remote access to your network allowed? Y/N. If yes, MFA on all remote access incl. RDP? Y/N. MFA provider (Auth0/Duo/LastPass/Okta/OneLogin/Other): ___  MFA type (Mobile OTP / Physical Key / Push / Certificate-based / Other): ___
  - Does compromise of a single device only compromise a single authenticator? Y/N: ___
  - NOTE: the PDF has ONE shared "MFA type" dropdown used by both 7.b(2) and 7.f — it can only hold a single value.
- c — Can users access email via web app or non-corporate device? Y/N. MFA enforced? Y/N: ___
- d — NGAV on all endpoints? Y/N. Provider: ___
- e — EDR with centralized monitoring/logging? Y/N. Provider: ___
  - App whitelisting/blacklisting? Y/N: ___  EDR on 100% of endpoints? Y/N: ___  BYOD allowed? Y/N; EDR required on BYOD? Y/N: ___
- f — MFA on all local and remote privileged-account access? Y/N. MFA type: ___
- g — Do you use a SOC? Y/N. 24/7? Outsourced (provider) or in-house?: ___
- h — Vulnerability management tool? Y/N. Provider (InsightVM/Rapid7, Nessus/Tenable, Qualys, Other): ___  Patching cadence (1-3d / 4-7d / 8-30d / 1mo+): ___
- i — Backups: I pre-checked Yes / dedicated cloud backup / encrypted / separate credentials / daily / restore 0-24h based on the documented AWS production setup (RDS daily snapshots + PITR, AES-256, write-only backup bucket, RTO ≤ 4h). Confirm, and answer the ones I left blank:
  - Immutable backups? Y/N: ___
  - MFA for internal and external access to backups? Y/N: ___
  - Successful restore test performed in the LAST 6 MONTHS (policy mandates quarterly drills — has one actually been run since ~2025-12-12)? Y/N: ___
  - Able to test backup integrity (malware-free) before restore? Y/N: ___

## 8. Phishing Controls
- a — Social engineering training for employees with financial responsibilities? Y/N: ___  Without? Y/N: ___  Includes phishing simulation? Y/N: ___
- b — Does the business send/receive wire transfers? Y/N: ___
  - If yes: wire request documentation form? written authorization protocol? separation of authority? call-back verification for new vendor payment instructions? call-back verification for account-change requests? (Y/N each): ___

## 9. Professional Services
- b — Any business other than described in 9.a? Y/N (+ revenue estimate): ___
- c — Revenue % by service type (must total 100%). My guess: mostly "Development, Publication or Reproduction of Prepackaged Software", possibly some consulting/custom dev — give me the split: ___
- d — Customer industries % (Aeronautics / Communications / Consumer / Engineering / Healthcare / Internet / Manufacturing / Govt-Military / Govt-Non-Military / Office / Retail / Other — must total 100%). Note the repo has a healthcare/medical deployment profile and robotics/industrial positioning: ___
- e — Five largest jobs/projects in past 3 years (client, start date, nature of services, revenue, % of gross revenue): ___

## 10. Contractual
- a — Written contract/agreement with clients? (Always / Most / Some / Never): ___  (sample contract attachment — skipping per your note)
- b — Indemnification/hold-harmless clauses in your favor? (A/M/S/N): ___
- c — Limitation of liability clauses? (A/M/S/N): ___
- d — Exclusion of consequential damages? (A/M/S/N): ___
- e — Guarantees or warranties? (A/M/S/N): ___
- f — Do you assume liability for others? (A/M/S/N): ___
- g — Fees ever contingent on client cost reductions/results? Y/N: ___

## 11. Media Liability
- a — Do you display third-party content (music, graphics, video) on your website/media? Y/N. If yes: do you always obtain rights/licenses, and what's the process?: ___
- b — Policies for identifying/removing defamatory or infringing content: ___

## 12. Loss History (past 3 years — I cannot attest to these; each "Yes" requires a Claim Supplemental Form)
- (1) Complaints/demands/litigation/investigations re professional E&O? Y/N: ___
- (2) Complaints/litigation re privacy, network security, defamation, infringement, etc.? Y/N: ___
- (3) Government action re privacy law violation? Y/N: ___
- (4) Notified anyone of a security/privacy breach? Y/N: ___
- (5) Cyber extortion demand or threat? Y/N: ___
- (6) Any unscheduled network outage/interruption? Y/N: ___
- (7) Property damage / business interruption from a cyber attack? Y/N: ___
- (8) Losses from wire/telecom/phishing fraud? Y/N: ___
- b — Knowledge of any act/error/incident that may give rise to a claim? Y/N: ___
- c — Any service provider with network access had an outage > 4 hours in past 3 years (e.g., AWS regional outages affecting you)? Y/N; did it interrupt your business? Y/N: ___

## 13. General Liability Loss History (only if GL coverage desired)
- a — Knowledge of circumstances that may give rise to a BI/PD claim? Y/N: ___
- b — Any BI/PD/advertising-injury claim in past 5 years? Y/N: ___

## Signature
- Officer who will sign + title (print name/title fields on page 6 are fillable; signature itself should be signed by hand or e-signature): ___
