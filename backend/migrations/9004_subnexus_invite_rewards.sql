-- SubNexus F06: additive schema for invitation signup rewards.
--
-- This migration deliberately does not seed settings, rewrite balances, or
-- touch existing affiliate rows. Runtime code remains fail-closed until an
-- administrator explicitly enables the SubNexus settings.

ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS source_ip VARCHAR(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN user_affiliate_ledger.source_ip IS
    'Trusted client IP captured for SubNexus invitation signup reward risk controls.';

-- One lookup covers both recipient actions and the per-IP daily counter.
CREATE INDEX IF NOT EXISTS idx_subnexus_invite_signup_reward_ip_daily
    ON user_affiliate_ledger (source_ip, created_at, source_user_id)
    WHERE action IN ('signup_bonus_inviter', 'signup_bonus_invitee')
      AND source_ip <> '';

-- A registration can produce at most one inviter row and one invitee row for
-- the same invitee. These partial unique indexes make retries idempotent even
-- if an application-level advisory lock is bypassed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_signup_reward_inviter_once
    ON user_affiliate_ledger (source_user_id)
    WHERE action = 'signup_bonus_inviter'
      AND source_user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_signup_reward_invitee_once
    ON user_affiliate_ledger (source_user_id)
    WHERE action = 'signup_bonus_invitee'
      AND source_user_id IS NOT NULL;
