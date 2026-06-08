-- Rollback: recreate refunds table
CREATE TABLE IF NOT EXISTS refunds (
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
