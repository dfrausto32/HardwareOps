CREATE TABLE IF NOT EXISTS groups (
  group_id uuid PRIMARY KEY,
  name text UNIQUE,
  selector jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS groups_selector_gin_idx ON groups USING GIN (selector);
