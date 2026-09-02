CREATE TABLE IF NOT EXISTS invoice_requests (
    id BIGSERIAL PRIMARY KEY,
    request_no VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    user_email_snapshot VARCHAR(255) NOT NULL,
    user_name_snapshot VARCHAR(100) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL,
    title_type VARCHAR(20) NOT NULL,
    title_name VARCHAR(200) NOT NULL,
    taxpayer_id VARCHAR(32) NOT NULL DEFAULT '',
    recipient_email VARCHAR(255) NOT NULL,
    recipient_phone VARCHAR(32) NOT NULL DEFAULT '',
    company_address VARCHAR(255) NOT NULL DEFAULT '',
    company_phone VARCHAR(32) NOT NULL DEFAULT '',
    bank_name VARCHAR(100) NOT NULL DEFAULT '',
    bank_account VARCHAR(64) NOT NULL DEFAULT '',
    invoice_item_name VARCHAR(100) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'CNY',
    total_amount NUMERIC(20,2) NOT NULL,
    order_count INTEGER NOT NULL,
    user_note VARCHAR(500) NOT NULL DEFAULT '',
    admin_note VARCHAR(1000) NOT NULL DEFAULT '',
    reject_reason VARCHAR(1000) NOT NULL DEFAULT '',
    invoice_code VARCHAR(64) NOT NULL DEFAULT '',
    invoice_number VARCHAR(128) NOT NULL DEFAULT '',
    invoice_date DATE,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    revision INTEGER NOT NULL DEFAULT 1,
    accepted_by BIGINT,
    accepted_at TIMESTAMPTZ,
    issued_by BIGINT,
    issued_at TIMESTAMPTZ,
    rejected_by BIGINT,
    rejected_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    voided_by BIGINT,
    voided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoice_requests_status_check CHECK (
        status IN ('PENDING', 'PROCESSING', 'REJECTED', 'CANCELLED', 'ISSUED', 'VOIDED')
    ),
    CONSTRAINT invoice_requests_title_type_check CHECK (title_type IN ('PERSONAL', 'COMPANY')),
    CONSTRAINT invoice_requests_title_fields_check CHECK (
        (title_type = 'PERSONAL' AND taxpayer_id = '') OR
        (title_type = 'COMPANY' AND taxpayer_id <> '')
    ),
    CONSTRAINT invoice_requests_currency_check CHECK (currency = 'CNY'),
    CONSTRAINT invoice_requests_total_amount_check CHECK (total_amount > 0),
    CONSTRAINT invoice_requests_order_count_check CHECK (order_count BETWEEN 1 AND 100),
    CONSTRAINT invoice_requests_revision_check CHECK (revision > 0),
    CONSTRAINT invoice_requests_invoice_date_check CHECK (
        status NOT IN ('ISSUED', 'VOIDED') OR invoice_date IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_invoice_requests_user_created
    ON invoice_requests(user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_invoice_requests_status_created
    ON invoice_requests(status, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_invoice_requests_created
    ON invoice_requests(created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS invoice_request_orders (
    id BIGSERIAL PRIMARY KEY,
    invoice_request_id BIGINT NOT NULL REFERENCES invoice_requests(id) ON DELETE RESTRICT,
    payment_order_id BIGINT NOT NULL,
    reservation_active BOOLEAN NOT NULL DEFAULT TRUE,
    out_trade_no_snapshot VARCHAR(64) NOT NULL,
    order_type_snapshot VARCHAR(20) NOT NULL,
    payment_type_snapshot VARCHAR(30) NOT NULL,
    pay_amount_snapshot NUMERIC(20,2) NOT NULL,
    currency_snapshot VARCHAR(3) NOT NULL,
    paid_at_snapshot TIMESTAMPTZ,
    completed_at_snapshot TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    CONSTRAINT invoice_request_orders_request_order_unique UNIQUE (invoice_request_id, payment_order_id),
    CONSTRAINT invoice_request_orders_amount_check CHECK (pay_amount_snapshot > 0),
    CONSTRAINT invoice_request_orders_currency_check CHECK (currency_snapshot = 'CNY'),
    CONSTRAINT invoice_request_orders_reservation_check CHECK (
        (reservation_active AND released_at IS NULL) OR
        (NOT reservation_active AND released_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_invoice_request_orders_active_payment
    ON invoice_request_orders(payment_order_id)
    WHERE reservation_active IS TRUE;

CREATE INDEX IF NOT EXISTS idx_invoice_request_orders_request
    ON invoice_request_orders(invoice_request_id, id);

CREATE TABLE IF NOT EXISTS invoice_files (
    id BIGSERIAL PRIMARY KEY,
    invoice_request_id BIGINT NOT NULL REFERENCES invoice_requests(id) ON DELETE RESTRICT,
    storage_key VARCHAR(512) NOT NULL UNIQUE,
    original_filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    file_extension VARCHAR(10) NOT NULL,
    file_size BIGINT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT TRUE,
    uploaded_by BIGINT NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    replaced_at TIMESTAMPTZ,
    CONSTRAINT invoice_files_size_check CHECK (file_size > 0 AND file_size <= 20971520),
    CONSTRAINT invoice_files_type_check CHECK (
        (content_type = 'application/pdf' AND file_extension = 'pdf') OR
        (content_type = 'application/ofd' AND file_extension = 'ofd')
    ),
    CONSTRAINT invoice_files_current_check CHECK (
        (is_current AND replaced_at IS NULL) OR
        (NOT is_current AND replaced_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_invoice_files_current_request
    ON invoice_files(invoice_request_id)
    WHERE is_current IS TRUE;

CREATE INDEX IF NOT EXISTS idx_invoice_files_request_uploaded
    ON invoice_files(invoice_request_id, uploaded_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS invoice_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    invoice_request_id BIGINT NOT NULL REFERENCES invoice_requests(id) ON DELETE RESTRICT,
    request_no_snapshot VARCHAR(64) NOT NULL,
    actor_type VARCHAR(20) NOT NULL,
    actor_id BIGINT,
    action VARCHAR(50) NOT NULL,
    from_status VARCHAR(20) NOT NULL DEFAULT '',
    to_status VARCHAR(20) NOT NULL DEFAULT '',
    request_revision INTEGER NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    user_agent_hash CHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoice_audit_logs_actor_type_check CHECK (actor_type IN ('user', 'admin', 'system')),
    CONSTRAINT invoice_audit_logs_revision_check CHECK (request_revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_invoice_audit_logs_request_created
    ON invoice_audit_logs(invoice_request_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_invoice_audit_logs_request_no
    ON invoice_audit_logs(request_no_snapshot, created_at ASC, id ASC);
