-- Publishing now requires a successful private configuration test. Existing
-- tasks intentionally become unpublished/untested; an administrator must test
-- and explicitly publish them. No stored output or credential is removed.
ALTER TABLE subnexus_model_evaluation_tasks
    ADD COLUMN IF NOT EXISTS published BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS test_status VARCHAR(16) NOT NULL DEFAULT 'untested'
        CHECK (test_status IN ('untested','running','passed','failed')),
    ADD COLUMN IF NOT EXISTS last_tested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS test_error VARCHAR(512) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS configuration_revision BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS lease_is_test BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subnexus_model_evaluation_results
    ADD COLUMN IF NOT EXISTS is_test BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS configuration_revision BIGINT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_subnexus_model_evaluation_tasks_published_due
    ON subnexus_model_evaluation_tasks(next_run_at) WHERE enabled AND published;
