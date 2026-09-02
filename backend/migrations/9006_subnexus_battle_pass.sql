-- SubNexus battle pass is an isolated activity module. The public switch
-- battle_pass_enabled is deliberately not seeded here: a missing, unreadable,
-- or non-literal-true setting is fail-closed in the service layer.
CREATE TABLE IF NOT EXISTS battle_pass_seasons (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    statistics_start_at TIMESTAMPTZ,
    enabled_at_snapshot TIMESTAMPTZ,
    activation_epoch INTEGER NOT NULL DEFAULT 0,
    premium_price DECIMAL(20,8) NOT NULL DEFAULT 0,
    price_currency VARCHAR(16) NOT NULL DEFAULT 'balance',
    max_level INTEGER NOT NULL DEFAULT 1,
    config_version INTEGER NOT NULL DEFAULT 1,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_at TIMESTAMPTZ,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_seasons_status_check
        CHECK (status IN ('draft', 'scheduled', 'paused', 'ended', 'archived')),
    CONSTRAINT battle_pass_seasons_time_order CHECK (end_at > start_at),
    CONSTRAINT battle_pass_seasons_price_nonnegative CHECK (premium_price >= 0),
    CONSTRAINT battle_pass_seasons_max_level_positive CHECK (max_level >= 1),
    CONSTRAINT battle_pass_seasons_currency_check CHECK (price_currency = 'balance')
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_seasons_status_window
    ON battle_pass_seasons (status, start_at, end_at);

CREATE TABLE IF NOT EXISTS battle_pass_levels (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    level INTEGER NOT NULL,
    required_exp BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_levels_unique UNIQUE (season_id, level),
    CONSTRAINT battle_pass_levels_level_positive CHECK (level >= 1),
    CONSTRAINT battle_pass_levels_exp_nonnegative CHECK (required_exp >= 0)
);

CREATE TABLE IF NOT EXISTS battle_pass_tasks (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    task_type VARCHAR(64) NOT NULL,
    period_type VARCHAR(16) NOT NULL,
    target_value DECIMAL(20,10) NOT NULL,
    exp_reward BIGINT NOT NULL,
    filter_scope VARCHAR(32) NOT NULL DEFAULT 'all',
    filter_values JSONB NOT NULL DEFAULT '[]'::jsonb,
    display_order INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_tasks_period_check CHECK (period_type IN ('daily', 'season')),
    CONSTRAINT battle_pass_tasks_target_positive CHECK (target_value > 0),
    CONSTRAINT battle_pass_tasks_exp_positive CHECK (exp_reward > 0),
    CONSTRAINT battle_pass_tasks_filter_scope_check CHECK (filter_scope IN ('all', 'model_family', 'exact_model'))
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_tasks_season
    ON battle_pass_tasks (season_id, enabled, display_order);

CREATE TABLE IF NOT EXISTS battle_pass_rewards (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    level INTEGER NOT NULL,
    track VARCHAR(16) NOT NULL,
    reward_type VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_rewards_track_check CHECK (track IN ('free', 'premium')),
    CONSTRAINT battle_pass_rewards_level_positive CHECK (level >= 1)
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_rewards_season_level
    ON battle_pass_rewards (season_id, level, track);

CREATE TABLE IF NOT EXISTS battle_pass_user_progress (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exp BIGINT NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1,
    premium_unlocked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_user_progress_unique UNIQUE (season_id, user_id),
    CONSTRAINT battle_pass_user_progress_exp_nonnegative CHECK (exp >= 0),
    CONSTRAINT battle_pass_user_progress_level_positive CHECK (level >= 1)
);

CREATE TABLE IF NOT EXISTS battle_pass_task_progress (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL REFERENCES battle_pass_tasks(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period_key VARCHAR(32) NOT NULL,
    current_value DECIMAL(20,10) NOT NULL DEFAULT 0,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_task_progress_unique UNIQUE (season_id, task_id, user_id, period_key),
    CONSTRAINT battle_pass_task_progress_value_nonnegative CHECK (current_value >= 0)
);

CREATE TABLE IF NOT EXISTS battle_pass_exp_ledger (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id BIGINT REFERENCES battle_pass_tasks(id) ON DELETE SET NULL,
    period_key VARCHAR(32) NOT NULL DEFAULT '',
    exp_delta BIGINT NOT NULL,
    reason VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_exp_ledger_unique UNIQUE (season_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_exp_ledger_user
    ON battle_pass_exp_ledger (season_id, user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS battle_pass_reward_grants (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reward_id BIGINT NOT NULL REFERENCES battle_pass_rewards(id) ON DELETE CASCADE,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error VARCHAR(160) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    granted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_reward_grants_unique UNIQUE (season_id, user_id, reward_id),
    CONSTRAINT battle_pass_reward_grants_status_check
        CHECK (status IN ('pending', 'processing', 'granted', 'failed', 'granted_capped', 'blocked_config'))
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_reward_grants_pending
    ON battle_pass_reward_grants (status, updated_at)
    WHERE status IN ('pending', 'failed');

CREATE TABLE IF NOT EXISTS battle_pass_purchases (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    price DECIMAL(20,8) NOT NULL,
    currency VARCHAR(16) NOT NULL DEFAULT 'balance',
    balance_before DECIMAL(20,8) NOT NULL,
    balance_after DECIMAL(20,8) NOT NULL,
    idempotency_key VARCHAR(160) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'completed',
    purchased_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_purchases_season_user UNIQUE (season_id, user_id),
    CONSTRAINT battle_pass_purchases_idempotency UNIQUE (user_id, idempotency_key),
    CONSTRAINT battle_pass_purchases_currency_check CHECK (currency = 'balance'),
    CONSTRAINT battle_pass_purchases_status_check CHECK (status = 'completed'),
    CONSTRAINT battle_pass_purchases_price_positive CHECK (price > 0)
);

CREATE TABLE IF NOT EXISTS battle_pass_source_cursors (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    source_type VARCHAR(32) NOT NULL,
    last_id BIGINT NOT NULL DEFAULT 0,
    last_updated_at TIMESTAMPTZ,
    activation_epoch INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_source_cursors_unique UNIQUE (season_id, source_type),
    CONSTRAINT battle_pass_source_cursors_type_check
        CHECK (source_type IN ('usage_log', 'payment_order', 'affiliate'))
);

CREATE TABLE IF NOT EXISTS battle_pass_source_contributions (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL REFERENCES battle_pass_tasks(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    contribution_value DECIMAL(20,10) NOT NULL DEFAULT 0,
    source_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_source_contributions_unique
        UNIQUE (season_id, task_id, user_id, source_type, source_id)
);

CREATE TABLE IF NOT EXISTS battle_pass_cosmetics (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(120) NOT NULL,
    color_token VARCHAR(32) NOT NULL DEFAULT '',
    asset_key VARCHAR(120) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_cosmetics_kind_check CHECK (kind IN ('badge', 'title')),
    CONSTRAINT battle_pass_cosmetics_unique UNIQUE (season_id, kind, code)
);

CREATE TABLE IF NOT EXISTS battle_pass_user_cosmetics (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cosmetic_id BIGINT NOT NULL REFERENCES battle_pass_cosmetics(id) ON DELETE CASCADE,
    equipped BOOLEAN NOT NULL DEFAULT FALSE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_user_cosmetics_unique UNIQUE (user_id, cosmetic_id)
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_user_cosmetics_equipped
    ON battle_pass_user_cosmetics (user_id, equipped)
    WHERE equipped;

CREATE TABLE IF NOT EXISTS battle_pass_pause_windows (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES battle_pass_seasons(id) ON DELETE CASCADE,
    paused_at TIMESTAMPTZ NOT NULL,
    resumed_at TIMESTAMPTZ,
    paused_by BIGINT,
    reason VARCHAR(160) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT battle_pass_pause_windows_order CHECK (resumed_at IS NULL OR resumed_at >= paused_at)
);

CREATE INDEX IF NOT EXISTS idx_battle_pass_pause_windows_season
    ON battle_pass_pause_windows (season_id, paused_at);
