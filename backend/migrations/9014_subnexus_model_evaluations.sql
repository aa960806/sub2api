-- Isolated, opt-in group model output monitoring. Old binaries ignore these
-- tables; no existing group, account, key or gateway configuration is changed.
CREATE TABLE IF NOT EXISTS subnexus_model_evaluation_tasks (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    endpoint VARCHAR(2048) NOT NULL,
    api_format VARCHAR(32) NOT NULL DEFAULT 'chat_completions' CHECK (api_format IN ('chat_completions','responses','messages')),
    api_key_encrypted TEXT NOT NULL,
    model VARCHAR(200) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    interval_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (interval_seconds BETWEEN 60 AND 604800),
    retention_days INTEGER NOT NULL DEFAULT 7 CHECK (retention_days BETWEEN 1 AND 90),
    max_records INTEGER NOT NULL DEFAULT 50 CHECK (max_records BETWEEN 1 AND 200),
    revision BIGINT NOT NULL DEFAULT 1,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token VARCHAR(36) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subnexus_model_evaluation_tasks_due
    ON subnexus_model_evaluation_tasks(next_run_at) WHERE enabled;

-- Keep capacity leases independent from tasks: deleting an active task must not
-- free its HTTP slot until its worker observes cancellation or the lease expires.
CREATE TABLE IF NOT EXISTS subnexus_model_evaluation_slots (
    id INTEGER PRIMARY KEY CHECK (id BETWEEN 1 AND 2),
    lease_token VARCHAR(36) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ
);
INSERT INTO subnexus_model_evaluation_slots(id) VALUES (1),(2) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS subnexus_model_evaluation_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES subnexus_model_evaluation_tasks(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    group_name VARCHAR(100) NOT NULL,
    task_name VARCHAR(100) NOT NULL,
    model VARCHAR(200) NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (status IN ('success','error')),
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    error_message VARCHAR(512) NOT NULL DEFAULT '',
    html TEXT NOT NULL DEFAULT '' CHECK (octet_length(html) <= 524288),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((status = 'success' AND html <> '' AND error_message = '') OR (status = 'error' AND html = ''))
);

CREATE INDEX IF NOT EXISTS idx_subnexus_model_evaluation_results_task
    ON subnexus_model_evaluation_results(task_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_subnexus_model_evaluation_results_group
    ON subnexus_model_evaluation_results(group_id,created_at DESC,id DESC);

INSERT INTO settings(key,value,updated_at) VALUES ('subnexus_model_evaluation_enabled','false',NOW())
    ON CONFLICT (key) DO NOTHING;
