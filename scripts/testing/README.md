# Test Script Layout

This directory is the canonical home for end-to-end and environment validation scripts.

Current scripts:

- `prod-lab-init.sh` - creates a production-like Docker lab config with hardened defaults.
- `prod-lab-up.sh` - starts the lab stack.
- `prod-lab-seed.sh` - enrolls demo devices and uploads signed demo artifacts.
- `prod-lab-smoke.sh` - runs a quick health/auth/metrics smoke pass.
- `prod-lab-down.sh` - stops (and optionally wipes) the lab stack.

Defaults:

- `LAB_ROOT=/tmp/hardwareops-prod-docker`
- override with `LAB_ROOT=<path>` when you need repo-local state

Conventions:

- Keep scripts environment-driven (`VAR=value script.sh`) so they work in CI and local runs.
- Prefer deterministic outputs and explicit non-zero exits on failure.
- Keep one script focused on one workflow stage (init/up/smoke/down) instead of monolithic scripts.
