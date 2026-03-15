-- Scalability: add indexes for common query patterns that will full-scan at 10K+ scale.
-- All created CONCURRENTLY to avoid locking production tables.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_devices_status
    ON devices (status);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_desired_state_group_updated_at
    ON desired_state_group (updated_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_desired_state_device_updated_at
    ON desired_state_device (updated_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_artifacts_status
    ON artifacts (status);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_artifacts_name_type
    ON artifacts (name, type);
