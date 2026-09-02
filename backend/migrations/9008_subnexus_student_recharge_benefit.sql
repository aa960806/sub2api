-- SubNexus student identity and ordinary-recharge bonus ledger.
--
-- This is deliberately a new migration name for the fork.  Some shared
-- databases may already contain the legacy 199_student_recharge_benefit.sql
-- objects; every statement is idempotent and leaves existing rows untouched.
-- No payment/order/user business table is altered by this migration.

CREATE TABLE IF NOT EXISTS student_account_status (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    is_student BOOLEAN NOT NULL DEFAULT TRUE,
    granted_by BIGINT NOT NULL REFERENCES users(id),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_by BIGINT REFERENCES users(id),
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT student_account_status_revocation_check CHECK (
        (is_student AND revoked_at IS NULL AND revoked_by IS NULL) OR
        (NOT is_student AND revoked_at IS NOT NULL AND revoked_by IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_student_account_status_active
    ON student_account_status(is_student, updated_at DESC);

CREATE TABLE IF NOT EXISTS student_account_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    admin_user_id BIGINT NOT NULL REFERENCES users(id),
    action VARCHAR(24) NOT NULL,
    previous_is_student BOOLEAN NOT NULL,
    current_is_student BOOLEAN NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    client_ip VARCHAR(45) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT student_account_audit_action_check CHECK (action IN ('grant', 'revoke'))
);

CREATE INDEX IF NOT EXISTS idx_student_account_audit_user
    ON student_account_audit_logs(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_student_account_audit_admin
    ON student_account_audit_logs(admin_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS student_recharge_bonus_logs (
    id BIGSERIAL PRIMARY KEY,
    payment_order_id BIGINT NOT NULL UNIQUE REFERENCES payment_orders(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    base_amount DECIMAL(20,8) NOT NULL,
    bonus_rate DECIMAL(10,6) NOT NULL,
    bonus_amount DECIMAL(20,8) NOT NULL,
    config_snapshot JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    granted_at TIMESTAMPTZ,
    reversed_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    reversed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT student_recharge_bonus_status_check CHECK (
        status IN ('pending', 'granted', 'reversed', 'failed')
    ),
    CONSTRAINT student_recharge_bonus_amount_check CHECK (
        base_amount > 0 AND bonus_rate > 0 AND bonus_amount > 0 AND
        reversed_amount >= 0 AND reversed_amount <= bonus_amount
    )
);

CREATE INDEX IF NOT EXISTS idx_student_recharge_bonus_pending
    ON student_recharge_bonus_logs(status, created_at)
    WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS idx_student_recharge_bonus_user
    ON student_recharge_bonus_logs(user_id, created_at DESC);
