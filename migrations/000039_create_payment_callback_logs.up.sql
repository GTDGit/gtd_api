-- ============================================
-- Migration 000039: Client webhook delivery audit for Payment events
-- ============================================

CREATE TABLE IF NOT EXISTS payment_callback_logs (
    id SERIAL PRIMARY KEY,
    payment_id INT NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    client_id INT NOT NULL REFERENCES clients(id),
    event VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    attempt INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 5,
    http_status INT,
    response_body TEXT,
    response_time_ms INT,
    is_delivered BOOLEAN NOT NULL DEFAULT false,
    error_message TEXT,
    next_retry_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pcb_logs_payment ON payment_callback_logs(payment_id);
CREATE INDEX IF NOT EXISTS idx_pcb_logs_client ON payment_callback_logs(client_id);
CREATE INDEX IF NOT EXISTS idx_pcb_logs_pending
    ON payment_callback_logs(next_retry_at)
 WHERE is_delivered = false;
