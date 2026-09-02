-- SubNexus marquee messages remain independent from upstream announcements.
-- Reusing the legacy table preserves administrator-authored messages during a
-- same-database cutover; runtime SQL only reads and mutates source='admin'.
CREATE TABLE IF NOT EXISTS activity_broadcasts (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(120) NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'admin',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 0,
    start_at TIMESTAMPTZ,
    end_at TIMESTAMPTZ,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subnexus_marquee_admin_active
    ON activity_broadcasts(enabled, priority DESC, created_at DESC)
    WHERE source = 'admin';

CREATE INDEX IF NOT EXISTS idx_subnexus_marquee_admin_window
    ON activity_broadcasts(start_at, end_at)
    WHERE source = 'admin';
