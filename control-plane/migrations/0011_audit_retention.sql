CREATE TABLE IF NOT EXISTS audit_retention (
  id int PRIMARY KEY DEFAULT 1,
  days int NOT NULL DEFAULT 90,
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO audit_retention (id, days)
VALUES (1, 90)
ON CONFLICT (id) DO NOTHING;
