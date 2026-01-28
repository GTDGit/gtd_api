-- ============================================
-- Migration 000005: Payment Tables
-- ============================================

CREATE TABLE payment_methods (
    id SERIAL PRIMARY KEY,
    type payment_type NOT NULL,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(50) NOT NULL,
    provider payment_provider NOT NULL,
    fee_type fee_type NOT NULL DEFAULT 'flat',
    fee_flat INT NOT NULL DEFAULT 0,
    fee_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    fee_min INT NOT NULL DEFAULT 0,
    fee_max INT NOT NULL DEFAULT 0,
    min_amount INT NOT NULL DEFAULT 10000,
    max_amount INT NOT NULL DEFAULT 50000000,
    expired_duration INT NOT NULL DEFAULT 86400,
    logo_url VARCHAR(255),
    display_order INT NOT NULL DEFAULT 0,
    payment_instruction JSONB,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_maintenance BOOLEAN NOT NULL DEFAULT false,
    maintenance_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(type, code)
);

CREATE INDEX idx_payment_methods_type_code ON payment_methods(type, code);
CREATE INDEX idx_payment_methods_type ON payment_methods(type);
CREATE INDEX idx_payment_methods_provider ON payment_methods(provider);
CREATE INDEX idx_payment_methods_is_active ON payment_methods(is_active);

CREATE TABLE payments (
    id SERIAL PRIMARY KEY,
    payment_id VARCHAR(50) NOT NULL UNIQUE,
    reference_id VARCHAR(50) NOT NULL,
    client_id INT NOT NULL REFERENCES clients(id),
    payment_method_id INT NOT NULL REFERENCES payment_methods(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,
    payment_type payment_type NOT NULL,
    payment_code VARCHAR(20) NOT NULL,
    provider payment_provider NOT NULL,
    amount BIGINT NOT NULL,
    fee BIGINT NOT NULL DEFAULT 0,
    total_amount BIGINT NOT NULL,
    customer_name VARCHAR(100),
    customer_email VARCHAR(100),
    customer_phone VARCHAR(20),
    status payment_status NOT NULL DEFAULT 'Pending',
    payment_detail JSONB NOT NULL DEFAULT '{}',
    payment_instruction JSONB,
    sender_bank VARCHAR(50),
    sender_name VARCHAR(100),
    sender_account VARCHAR(50),
    provider_ref VARCHAR(100),
    provider_data JSONB,
    callback_type VARCHAR(20),
    description TEXT,
    metadata JSONB,
    callback_sent BOOLEAN NOT NULL DEFAULT false,
    callback_sent_at TIMESTAMPTZ,
    callback_attempts INT NOT NULL DEFAULT 0,
    expired_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(client_id, reference_id)
);

CREATE INDEX idx_payments_payment_id ON payments(payment_id);
CREATE INDEX idx_payments_reference_id ON payments(reference_id);
CREATE INDEX idx_payments_client_id ON payments(client_id);
CREATE INDEX idx_payments_payment_type_code ON payments(payment_type, payment_code);
CREATE INDEX idx_payments_payment_type ON payments(payment_type);
CREATE INDEX idx_payments_provider ON payments(provider);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_is_sandbox ON payments(is_sandbox);
CREATE INDEX idx_payments_created_at ON payments(created_at);
CREATE INDEX idx_payments_expired_at ON payments(expired_at) WHERE status = 'Pending';
CREATE INDEX idx_payments_provider_ref ON payments(provider_ref);
CREATE INDEX idx_payments_callback_pending ON payments(callback_sent) WHERE callback_sent = false AND status IN ('Paid', 'Expired', 'Cancelled', 'Failed');

CREATE TABLE payment_logs (
    id SERIAL PRIMARY KEY,
    payment_id INT NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL,
    provider payment_provider NOT NULL,
    request JSONB,
    response JSONB,
    is_success BOOLEAN NOT NULL DEFAULT false,
    error_code VARCHAR(50),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_at TIMESTAMPTZ,
    response_time_ms INT
);

CREATE INDEX idx_payment_logs_payment_id ON payment_logs(payment_id);
CREATE INDEX idx_payment_logs_action ON payment_logs(action);
CREATE INDEX idx_payment_logs_created_at ON payment_logs(created_at);

CREATE TABLE payment_callbacks (
    id SERIAL PRIMARY KEY,
    provider payment_provider NOT NULL,
    provider_ref VARCHAR(100),
    headers JSONB,
    payload JSONB NOT NULL,
    signature VARCHAR(255),
    is_valid_signature BOOLEAN NOT NULL DEFAULT false,
    payment_id VARCHAR(50),
    status VARCHAR(20),
    paid_amount BIGINT,
    is_processed BOOLEAN NOT NULL DEFAULT false,
    processed_at TIMESTAMPTZ,
    process_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_callbacks_provider ON payment_callbacks(provider);
CREATE INDEX idx_payment_callbacks_provider_ref ON payment_callbacks(provider_ref);
CREATE INDEX idx_payment_callbacks_payment_id ON payment_callbacks(payment_id);
CREATE INDEX idx_payment_callbacks_is_processed ON payment_callbacks(is_processed) WHERE is_processed = false;
CREATE INDEX idx_payment_callbacks_created_at ON payment_callbacks(created_at);

CREATE TABLE refunds (
    id SERIAL PRIMARY KEY,
    refund_id VARCHAR(50) NOT NULL UNIQUE,
    payment_id INT NOT NULL REFERENCES payments(id),
    amount BIGINT NOT NULL,
    status refund_status NOT NULL DEFAULT 'Pending',
    reason TEXT NOT NULL,
    provider_ref VARCHAR(100),
    provider_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refunds_refund_id ON refunds(refund_id);
CREATE INDEX idx_refunds_payment_id ON refunds(payment_id);
CREATE INDEX idx_refunds_status ON refunds(status);
CREATE INDEX idx_refunds_created_at ON refunds(created_at);
