CREATE TABLE IF NOT EXISTS registration_ip_cooldowns (
    ip_hash CHAR(64) PRIMARY KEY,
    last_registered_at TIMESTAMPTZ,
    last_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reservation_token CHAR(64),
    reserved_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_registration_ip_cooldowns_last_registered_at
    ON registration_ip_cooldowns(last_registered_at DESC)
    WHERE last_registered_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_registration_ip_cooldowns_reserved_until
    ON registration_ip_cooldowns(reserved_until)
    WHERE reserved_until IS NOT NULL;

COMMENT ON TABLE registration_ip_cooldowns IS 'Hashed trusted client IP cooldown state for anti-abuse registration control.';
COMMENT ON COLUMN registration_ip_cooldowns.ip_hash IS 'SHA-256 hash of the trusted client IP with the server secret when configured.';
COMMENT ON COLUMN registration_ip_cooldowns.reservation_token IS 'Short-lived registration reservation token used to block concurrent same-IP signups.';
