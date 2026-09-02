-- SubNexus custom activity-center shell.
-- Existing legacy rows are preserved; the migrated runtime only reads and mutates activity_type='custom'.
CREATE TABLE IF NOT EXISTS activity_center_items (
    id BIGSERIAL PRIMARY KEY,
    slug VARCHAR(80) NOT NULL UNIQUE,
    title VARCHAR(120) NOT NULL,
    subtitle VARCHAR(240) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    icon VARCHAR(64) NOT NULL DEFAULT 'gift',
    cover_url TEXT NOT NULL DEFAULT '',
    route_path VARCHAR(255) NOT NULL DEFAULT '',
    external_url TEXT NOT NULL DEFAULT '',
    action_label VARCHAR(40) NOT NULL DEFAULT '',
    activity_type VARCHAR(32) NOT NULL DEFAULT 'custom',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    start_at TIMESTAMPTZ,
    end_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activity_center_items_visible
    ON activity_center_items(enabled, sort_order ASC, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_center_items_window
    ON activity_center_items(start_at, end_at);
