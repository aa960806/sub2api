-- Independent durable Seedance journal. No existing user balances, usage,
-- orders, or subscriptions are updated. Terminal IDs remain as idempotency
-- tombstones; unfinished tasks and their reconciliation records never expire.
CREATE TABLE IF NOT EXISTS subnexus_seedance_tasks (
    task_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_hash TEXT NOT NULL,
    unit_price NUMERIC(20, 8) NOT NULL CHECK (unit_price >= 0),
    phase TEXT NOT NULL CHECK (phase IN ('prepared', 'reserving', 'reserved', 'submitting', 'submission_unknown', 'submitted', 'capturing', 'releasing', 'manual_review', 'settled', 'released', 'rejected')),
    next_attempt_at BIGINT NOT NULL DEFAULT 0,
    record JSONB NOT NULL,
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (jsonb_typeof(record) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_subnexus_seedance_tasks_recovery
    ON subnexus_seedance_tasks (next_attempt_at, created_at, task_id)
    WHERE phase NOT IN ('settled', 'released', 'rejected', 'manual_review');
CREATE INDEX IF NOT EXISTS idx_subnexus_seedance_tasks_owner
    ON subnexus_seedance_tasks (user_id, api_key_id, created_at);
CREATE INDEX IF NOT EXISTS idx_subnexus_seedance_tasks_pending_quota
    ON subnexus_seedance_tasks (api_key_id) INCLUDE (unit_price)
    WHERE phase NOT IN ('settled', 'released', 'rejected');

CREATE TABLE IF NOT EXISTS subnexus_seedance_files (
    file_id TEXT PRIMARY KEY,
    record JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (jsonb_typeof(record) = 'object')
);
CREATE INDEX IF NOT EXISTS idx_subnexus_seedance_files_expiry ON subnexus_seedance_files (expires_at);

-- Only remove a hold record after a confirmed capture or release.
CREATE TABLE IF NOT EXISTS subnexus_seedance_pending_holds (
    task_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    amount NUMERIC(20, 8) NOT NULL CHECK (amount >= 0),
    due_at BIGINT NOT NULL,
    record JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (jsonb_typeof(record) = 'object')
);
CREATE INDEX IF NOT EXISTS idx_subnexus_seedance_pending_holds_due ON subnexus_seedance_pending_holds (due_at, task_id);

CREATE TABLE IF NOT EXISTS subnexus_seedance_settlement_claims (
    task_id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
