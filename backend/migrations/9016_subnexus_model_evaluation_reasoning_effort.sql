-- Optional reasoning intensity for model evaluation requests. Existing tasks
-- retain provider defaults and no stored credentials or generated history change.
ALTER TABLE subnexus_model_evaluation_tasks
    ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(16) NOT NULL DEFAULT ''
        CHECK (reasoning_effort IN ('','none','minimal','low','medium','high','xhigh','max','ultra'));
