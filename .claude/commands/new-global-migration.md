# /new-global-migration

Create the next sequential SQL migration file for the **global control-plane**.

## Steps

1. Find the current highest global migration number:
   ```bash
   ls control-plane/migrations/global/*.sql | sort | tail -3
   ```

2. Increment by 1 and create the file. If the user provided a name, use it (snake_case); otherwise ask.
   File path: `control-plane/migrations/global/NNNN_<name>.sql`

3. Write the file with this boilerplate:
   ```sql
   -- <Brief description of what this migration adds/changes>
   -- Global-plane migration — applies to the parcel_global database only.

   CREATE TABLE IF NOT EXISTS <table_name> (
       -- Standard FK pattern: reference regional_planes(plane_id) for per-plane tables
       -- Use JSONB for flexible metadata (labels, policy, selector)
       -- Use TIMESTAMPTZ for all timestamps
   );

   CREATE INDEX IF NOT EXISTS idx_<table>_<column> ON <table_name>(<column>);
   ```

4. Remind the user that:
   - Global migrations live in `migrations/global/` — separate from regional migrations
   - They apply to the `parcel_global` database (separate from the regional `parcel` database)
   - The global-plane binary runs them automatically on startup
   - All DDL must be idempotent

## Common FK targets in the global schema

- `regional_planes(plane_id)` — per-plane rows (use `ON DELETE CASCADE`)
- `global_groups(group_id)` — per-group rows (use `ON DELETE CASCADE`)
- `global_enrollment_profiles(profile_id)` — per-profile rows

## Usage examples

- `/new-global-migration` — prompts for a name
- `/new-global-migration add plane audit log` — creates `NNNN_add_plane_audit_log.sql`
