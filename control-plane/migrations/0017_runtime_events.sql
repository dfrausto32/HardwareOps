CREATE TABLE IF NOT EXISTS runtime_events (
	event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	occurred_at timestamptz NOT NULL DEFAULT now(),
	event_type text NOT NULL,
	device_id text,
	payload jsonb
);

CREATE INDEX IF NOT EXISTS idx_runtime_events_occurred_at ON runtime_events (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_runtime_events_type ON runtime_events (event_type);
CREATE INDEX IF NOT EXISTS idx_runtime_events_device_id ON runtime_events (device_id);

CREATE TABLE IF NOT EXISTS runtime_event_retention (
	id smallint PRIMARY KEY DEFAULT 1,
	days integer NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT runtime_event_retention_singleton CHECK (id = 1),
	CONSTRAINT runtime_event_retention_days_positive CHECK (days > 0)
);

INSERT INTO runtime_event_retention (id, days, updated_at)
VALUES (1, 30, now())
ON CONFLICT (id) DO NOTHING;
