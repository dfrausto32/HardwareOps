# Software Update & Configuration Application Design (v1)

## 1. Purpose & Scope

This document defines how software updates, configuration changes, and
secrets are applied, validated, rolled back, and audited on managed
machines using the Parcel control-plane and agent.

Goals: - Safely update platform services, customer applications, and
OS-level dependencies - Guarantee machine survivability via automatic
rollback - Support robotics and edge workloads with diverse runtime
needs - Enable fleet-wide, group-level, and per-device control without
manual machine access - Maintain strong security, auditability, and
reproducibility

Non-goals (v1): - Full OS image replacement (A/B rootfs) - Advanced
secret rotation policies - Arbitrary remote execution outside defined
plans

## 2. High-Level Architecture

### Components

**Control-Plane** - Stores desired state, artifacts, configs, secrets -
Issues short-TTL presigned URLs - Records apply results and audit
history

**Agent** - Runs as a systemd service - Polls desired state - Applies
updates declaratively - Enforces rollback and health gates - Reports
results

### Trust Model

-   All communication uses TLS + mTLS
-   Device identity derived from client cert fingerprint
-   Agents never possess long-lived storage credentials
-   Artifacts, configs, and secrets retrieved via short-TTL presigned
    URLs

## 3. Update Types

1.  Platform & Customer Applications
2.  Containerized Applications
3.  OS-Level Dependencies
4.  Configuration & Secrets

## 4. Artifact Model

Artifacts are immutable bundles containing assets, templates, manifests,
and an apply plan.

## 5. Apply Plan (YAML)

Declarative, ordered, whitelisted step execution defined in plan.yaml.

## 6. Supported Step Types

-   systemd.*, docker.*, apt.*, file.*, symlink.switch
-   script.run (guarded)
-   probe.http, probe.exec, probe.file_exists

## 7. Checkpointing & Rollback

Rollback is mandatory on any failure. Application stability is
guaranteed; OS rollback is best-effort.

## 8. Health & Remediation

Updates succeed only after health checks pass within defined timeouts.

## 9. Configuration System

-   Structured YAML/JSON
-   Layered (Base → Tenant → Group → Device)
-   Versioned and revertible
-   Template-rendered at the agent

## 10. Secrets Management

-   First-class, versioned, encrypted
-   Retrieved via presigned URLs
-   Never embedded in artifacts
-   Fully revertible

## 11. Apply Flow

1.  Check-in
2.  Stage artifact
3.  Merge config
4.  Render templates
5.  Fetch secrets
6.  Execute plan
7.  Health gate
8.  Commit or rollback

## 12. Cleanup & Safety

Disk space guards, release pruning, rollback retention.

## 13. Observability & Audit

Step-level apply results and full change history.

## 14. Future Extensions

K8s mode, OS image updates, secret rotation, TPM, rollout waves.

## 15. Summary

A deterministic, secure, rollback-safe update system designed for
robotics and edge fleets.
