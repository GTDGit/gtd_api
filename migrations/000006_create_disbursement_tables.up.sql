-- ============================================
-- Migration 000006: Disbursement Tables
-- Transfer dana ke rekening bank (intrabank & interbank)
-- ============================================

-- Create transfer_inquiries FIRST (before transfers table that references it)
CREATE TABLE transfer_inquiries (
    id SERIAL PRIMARY KEY,
    inquiry_id VARCHAR(50) NOT NULL UNIQUE,             -- 'INQ-20250128-000001'
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,

    -- Bank info
    bank_code VARCHAR(10) NOT NULL,
    bank_name VARCHAR(100),
    account_number VARCHAR(34) NOT NULL,
    account_name VARCHAR(100),                          -- Nama dari bank

    -- Routing
    transfer_type transfer_type NOT NULL,               -- Determined by bank_code
    provider disbursement_provider NOT NULL,

    -- Provider response
    provider_ref VARCHAR(100),
    provider_data JSONB,

    -- Expiry
    expired_at TIMESTAMPTZ NOT NULL,                    -- 30 minutes from creation

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transfer_inquiries_inquiry_id ON transfer_inquiries(inquiry_id);
CREATE INDEX idx_transfer_inquiries_client_id ON transfer_inquiries(client_id);
CREATE INDEX idx_transfer_inquiries_expired_at ON transfer_inquiries(expired_at);

-- Now create transfers table (references transfer_inquiries)
CREATE TABLE transfers (
    id SERIAL PRIMARY KEY,
    transfer_id VARCHAR(50) NOT NULL UNIQUE,
    reference_id VARCHAR(50) NOT NULL,
    client_id INT NOT NULL REFERENCES clients(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,

    transfer_type transfer_type NOT NULL,
    provider disbursement_provider NOT NULL,

    bank_code VARCHAR(10) NOT NULL,
    bank_name VARCHAR(100),
    account_number VARCHAR(34) NOT NULL,
    account_name VARCHAR(100),

    source_bank_code VARCHAR(10) NOT NULL,
    source_account_number VARCHAR(34) NOT NULL,

    amount BIGINT NOT NULL,
    fee BIGINT NOT NULL DEFAULT 0,
    total_amount BIGINT NOT NULL,

    status transfer_status NOT NULL DEFAULT 'Processing',
    failed_reason VARCHAR(255),
    failed_code VARCHAR(50),

    purpose_code VARCHAR(2),
    remark VARCHAR(50),

    inquiry_id INT REFERENCES transfer_inquiries(id),

    provider_ref VARCHAR(100),
    provider_data JSONB,

    callback_sent BOOLEAN NOT NULL DEFAULT false,
    callback_sent_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(client_id, reference_id)
);

CREATE INDEX idx_transfers_transfer_id ON transfers(transfer_id);
CREATE INDEX idx_transfers_reference_id ON transfers(reference_id);
CREATE INDEX idx_transfers_client_id ON transfers(client_id);
CREATE INDEX idx_transfers_status ON transfers(status);
CREATE INDEX idx_transfers_transfer_type ON transfers(transfer_type);
CREATE INDEX idx_transfers_provider ON transfers(provider);
CREATE INDEX idx_transfers_is_sandbox ON transfers(is_sandbox);
CREATE INDEX idx_transfers_created_at ON transfers(created_at);
CREATE INDEX idx_transfers_callback_pending ON transfers(callback_sent) WHERE callback_sent = false AND status IN ('Success', 'Failed');

-- Transfer logs (setiap attempt ke bank API)
CREATE TABLE transfer_logs (
    id SERIAL PRIMARY KEY,
    transfer_id INT NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
    inquiry_id INT REFERENCES transfer_inquiries(id),

    action VARCHAR(50) NOT NULL,                        -- 'inquiry', 'transfer', 'status_check'
    provider disbursement_provider NOT NULL,

    -- Request/Response
    request JSONB,
    response JSONB,

    -- Result
    is_success BOOLEAN NOT NULL DEFAULT false,
    response_code VARCHAR(20),                          -- Bank response code
    response_message TEXT,

    -- Timing
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_at TIMESTAMPTZ,
    response_time_ms INT
);

CREATE INDEX idx_transfer_logs_transfer_id ON transfer_logs(transfer_id);
CREATE INDEX idx_transfer_logs_action ON transfer_logs(action);
CREATE INDEX idx_transfer_logs_created_at ON transfer_logs(created_at);

-- Transfer callbacks (webhook dari bank, jika ada)
CREATE TABLE transfer_callbacks (
    id SERIAL PRIMARY KEY,
    provider disbursement_provider NOT NULL,
    provider_ref VARCHAR(100),

    headers JSONB,
    payload JSONB NOT NULL,
    signature VARCHAR(255),
    is_valid_signature BOOLEAN NOT NULL DEFAULT false,

    transfer_id VARCHAR(50),                            -- Our transfer_id (parsed)
    status VARCHAR(20),

    is_processed BOOLEAN NOT NULL DEFAULT false,
    processed_at TIMESTAMPTZ,
    process_error TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transfer_callbacks_provider ON transfer_callbacks(provider);
CREATE INDEX idx_transfer_callbacks_provider_ref ON transfer_callbacks(provider_ref);
CREATE INDEX idx_transfer_callbacks_transfer_id ON transfer_callbacks(transfer_id);
CREATE INDEX idx_transfer_callbacks_is_processed ON transfer_callbacks(is_processed) WHERE is_processed = false;
CREATE INDEX idx_transfer_callbacks_created_at ON transfer_callbacks(created_at);
