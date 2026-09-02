-- SubNexus check-in migration.
--
-- This file is deliberately additive.  The old SubNexus deployment may have
-- already applied 151_activity_rewards.sql, 155_activity_checkin_ip.sql,
-- 163_checkin_frozen.sql, and 184_activity_checkin_streaks.sql under their
-- legacy filenames.  CREATE TABLE IF NOT EXISTS therefore cannot be treated
-- as proof that an existing object has the shape expected by the new binary;
-- the ADD COLUMN clauses below make the two known additive extensions
-- idempotent while preserving every existing row.
--
-- The runtime is gated by the independent `subnexus_checkin_enabled` setting
-- and fails closed when that setting is absent or unreadable.  This migration
-- intentionally does not seed or modify any setting.
CREATE TABLE IF NOT EXISTS activity_reward_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source VARCHAR(32) NOT NULL,
    period VARCHAR(32) NOT NULL DEFAULT '',
    rank INTEGER NOT NULL DEFAULT 0,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT activity_reward_logs_positive_amount CHECK (amount > 0)
);

-- Keep the legacy core column contract usable when an older compatible table
-- exists but predates one of the non-destructive reward metadata fields.
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS rank INTEGER NOT NULL DEFAULT 0;
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS amount DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS ip VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE activity_reward_logs
    ADD COLUMN IF NOT EXISTS frozen BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_activity_reward_logs_source_period_user
    ON activity_reward_logs(source, period, user_id);

-- These indexes mirror the legacy activity service access patterns: history
-- by user, reward-source reports, free-limit aggregation, frozen settlement,
-- and per-period IP checks.  None is unique because IP enforcement is an
-- explicit runtime policy and can be disabled by an administrator.
CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_user_created
    ON activity_reward_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_source_created
    ON activity_reward_logs(source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_user_source_period
    ON activity_reward_logs(user_id, source, period);
CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_source_period_ip
    ON activity_reward_logs(source, period, ip) WHERE ip <> '';
CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_frozen
    ON activity_reward_logs(user_id, source) WHERE frozen = TRUE;

CREATE TABLE IF NOT EXISTS activity_checkin_streaks (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    current_streak INTEGER NOT NULL DEFAULT 0,
    last_checkin_date DATE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT activity_checkin_streaks_non_negative CHECK (current_streak >= 0)
);

ALTER TABLE activity_checkin_streaks
    ADD COLUMN IF NOT EXISTS current_streak INTEGER NOT NULL DEFAULT 0;
ALTER TABLE activity_checkin_streaks
    ADD COLUMN IF NOT EXISTS last_checkin_date DATE;
ALTER TABLE activity_checkin_streaks
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
