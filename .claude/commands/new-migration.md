# /new-migration

Create the next sequential SQL migration file for the **regional control-plane**.

## Steps

1. Find the current highest migration number:
   ```bash
   ls control-plane/migrations/*.sql | sort | tail -3
   ```

2. Increment by 1 and create the file. If the user provided a name, use it (snake_case); otherwise ask.
   File path: `control-plane/migrations/NNNN_<name>.sql`

3. Write the file with this boilerplate:
   ```sql
   -- <Brief description of what this migration adds/changes>

   -- Use IF NOT EXISTS / IF NOT EXISTS everywhere so migrations are idempotent.

   CREATE TABLE IF NOT EXISTS <table_name> (
       -- columns here
   );
   ```

4. Remind the user that:
   - Migrations run automatically on next server start when `AUTO_MIGRATE=1` is set
   - All DDL should be idempotent (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`)
   - Never edit or delete existing migration files — always add a new one

## Usage examples

- `/new-migration` — prompts for a name, then creates the file
- `/new-migration add device groups index` — creates the next file named `NNNN_add_device_groups_index.sql`
