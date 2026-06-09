-- ============================================
-- Rollback Migration 000018: Recreate payment_callbacks table
-- ============================================

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