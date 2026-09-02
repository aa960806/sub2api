-- SubNexus invitation activities query indexes.
--
-- This migration is intentionally non-transactional so PostgreSQL can build
-- each index concurrently on a live database.  It is purely additive: no
-- settings are seeded, no activity/recharge rows are rewritten, and no
-- activity-center cards are inserted.  The runtime remains fail-closed until
-- the administrator enables the dedicated SubNexus setting.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subnexus_invite_activities_affiliate_inviter
    ON user_affiliates (inviter_id, user_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subnexus_invite_activities_payment_user
    ON payment_orders (user_id, status, order_type)
    WHERE status IN ('COMPLETED', 'PARTIALLY_REFUNDED')
      AND order_type IN ('balance', 'subscription', 'first_recharge_gift');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subnexus_invite_activities_reward_user_source
    ON activity_reward_logs (user_id, source);
