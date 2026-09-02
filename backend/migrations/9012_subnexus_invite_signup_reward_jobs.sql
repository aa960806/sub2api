-- SubNexus invitation signup reward retry queue.
--
-- This migration is additive and intentionally does not seed or alter any
-- rollout setting is explicitly enabled.  Older
-- binaries ignore this table and can therefore be kept as a rollback target.

CREATE TABLE IF NOT EXISTS subnexus_affiliate_signup_reward_jobs (
    id BIGSERIAL PRIMARY KEY,
    inviter_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    inviter_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    invitee_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    client_ip VARCHAR(64) NOT NULL DEFAULT '',
    ip_limit_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ip_daily_limit INTEGER NOT NULL DEFAULT 3,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NOT NULL DEFAULT '',
    skip_reason VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subnexus_affiliate_signup_reward_jobs_status_check
        CHECK (status IN ('pending', 'completed', 'skipped')),
    CONSTRAINT subnexus_affiliate_signup_reward_jobs_amount_check
        CHECK (inviter_amount >= 0 AND invitee_amount >= 0),
    CONSTRAINT subnexus_affiliate_signup_reward_jobs_attempt_check
        CHECK (attempt_count >= 0),
    CONSTRAINT subnexus_affiliate_signup_reward_jobs_ip_limit_check
        CHECK (ip_daily_limit > 0),
    CONSTRAINT subnexus_affiliate_signup_reward_jobs_invitee_unique
        UNIQUE (invitee_user_id)
);

CREATE INDEX IF NOT EXISTS idx_subnexus_affiliate_signup_reward_jobs_pending
    ON subnexus_affiliate_signup_reward_jobs (next_attempt_at, id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_subnexus_affiliate_signup_reward_jobs_inviter
    ON subnexus_affiliate_signup_reward_jobs (inviter_id, created_at, id);

COMMENT ON TABLE subnexus_affiliate_signup_reward_jobs IS
    'Durable, idempotent retry queue for SubNexus invitation signup rewards.';
COMMENT ON COLUMN subnexus_affiliate_signup_reward_jobs.status IS
    'pending|completed|skipped; pending rows are retried with bounded backoff.';
COMMENT ON COLUMN subnexus_affiliate_signup_reward_jobs.next_attempt_at IS
    'Earliest retry time for a transient reward failure.';
