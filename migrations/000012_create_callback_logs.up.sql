-- ============================================
-- Migration 000012: Callback Logs Table
-- Log setiap callback yang dikirim ke client
-- ============================================

CREATE TABLE callback_logs (
    id SERIAL PRIMARY KEY,
    client_id INT NOT NULL REFERENCES clients(id),

    -- Source (either transaction or payment)
    transaction_id INT REFERENCES transactions(id) ON DELETE CASCADE,
    payment_id INT REFERENCES payments(id) ON DELETE CASCADE,

    -- Callback info
    event VARCHAR(50) NOT NULL,                      -- 'transaction.success', 'payment.paid', etc
    payload JSONB NOT NULL,                          -- Full callback payload

    -- Delivery status
    attempt SMALLINT NOT NULL DEFAULT 1,             -- Attempt number (1-6)
    max_attempts SMALLINT NOT NULL DEFAULT 6,
    http_status INT,                                 -- Response HTTP status
    response_body TEXT,                              -- Response from client
    response_time_ms INT,                            -- Response time
    is_delivered BOOLEAN NOT NULL DEFAULT false,

    -- Error
    error_message TEXT,

    -- Timing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_retry_at TIMESTAMPTZ,                       -- For retry: 30s, 1m, 5m, 30m, 2h
    delivered_at TIMESTAMPTZ,

    -- Constraint: must have either transaction_id or payment_id
    CONSTRAINT callback_logs_source_check CHECK (
        (transaction_id IS NOT NULL AND payment_id IS NULL) OR
        (transaction_id IS NULL AND payment_id IS NOT NULL)
    )
);

-- Indexes
CREATE INDEX idx_callback_logs_client_id ON callback_logs(client_id);
CREATE INDEX idx_callback_logs_transaction_id ON callback_logs(transaction_id);
CREATE INDEX idx_callback_logs_payment_id ON callback_logs(payment_id);
CREATE INDEX idx_callback_logs_event ON callback_logs(event);
CREATE INDEX idx_callback_logs_is_delivered ON callback_logs(is_delivered) WHERE is_delivered = false;
CREATE INDEX idx_callback_logs_next_retry_at ON callback_logs(next_retry_at) WHERE is_delivered = false;
CREATE INDEX idx_callback_logs_created_at ON callback_logs(created_at);

-- Comments
COMMENT ON TABLE callback_logs IS 'Log callback yang dikirim ke client (unified for PPOB and Payment)';
COMMENT ON COLUMN callback_logs.event IS 'Event type: transaction.success, transaction.failed, payment.paid, payment.expired, etc';
COMMENT ON COLUMN callback_logs.attempt IS 'Current attempt number, starts at 1';
COMMENT ON COLUMN callback_logs.next_retry_at IS 'Next retry time with exponential backoff: 30s, 1m, 5m, 30m, 2h';
