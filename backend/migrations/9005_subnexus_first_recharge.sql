-- SubNexus first-recharge offer reservation state.
-- This migration is additive and safe for a shared database: it never edits
-- payment_orders, users, balances, subscriptions, or usage records.
CREATE TABLE IF NOT EXISTS first_recharge_gift_purchases (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id BIGINT REFERENCES payment_orders(id) ON DELETE SET NULL,
    price DECIMAL(20, 2) NOT NULL DEFAULT 0,
    credited_amount DECIMAL(20, 2) NOT NULL DEFAULT 0,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_first_recharge_gift_purchases_user
    ON first_recharge_gift_purchases(user_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_first_recharge_gift_purchases_order
    ON first_recharge_gift_purchases(order_id)
    WHERE order_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_first_recharge_gift_purchases_status
    ON first_recharge_gift_purchases(status, created_at DESC);
