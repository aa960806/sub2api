-- SubNexus leaderboard reward settlement contract.
--
-- 9002 creates the shared activity_reward_logs table on a fresh database.
-- Older upgraded databases may have received the table from the legacy
-- activity migrations instead, so this migration is deliberately additive and
-- idempotent. It never deletes or rewrites existing reward rows.

CREATE UNIQUE INDEX IF NOT EXISTS idx_activity_reward_logs_source_period_user
    ON activity_reward_logs(source, period, user_id);

CREATE INDEX IF NOT EXISTS idx_activity_reward_logs_leaderboard_period
    ON activity_reward_logs(source, period, created_at DESC)
    WHERE source IN ('leaderboard_week', 'leaderboard_month');
