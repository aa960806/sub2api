-- Independent rollout switches for migrated SubNexus workflows.
-- Existing values are preserved so a staged deployment or rollback cannot
-- silently overwrite an operator's explicit decision.  This post-migration
-- seed is needed because InitializeDefaultSettings intentionally exits as
-- soon as it finds any existing setting; without it, an upgraded database
-- would have missing rows and transaction-local gates could fail closed
-- inconsistently.
INSERT INTO settings (key, value)
VALUES
    ('registration_ip_cooldown_enabled', 'false'),
    ('subnexus_activity_center_enabled', 'false'),
    ('subnexus_checkin_enabled', 'false'),
    ('subnexus_leaderboard_enabled', 'false'),
    ('subnexus_marquee_enabled', 'false'),
    ('subnexus_invite_activities_enabled', 'false'),
    ('subnexus_invite_rewards_enabled', 'false'),
    ('subnexus_first_recharge_enabled', 'false'),
    ('battle_pass_enabled', 'false'),
    ('subnexus_student_recharge_benefit_enabled', 'false'),
    ('subnexus_invoice_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
