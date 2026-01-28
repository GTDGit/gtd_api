-- ============================================
-- Migration 000003: PPOB Tables (products, skus, transactions, logs, callbacks)
-- ============================================

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    sku_code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    category VARCHAR(50) NOT NULL,
    brand VARCHAR(50) NOT NULL,
    type product_type NOT NULL,
    admin INT DEFAULT 0,
    commission INT DEFAULT 0,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_sku_code ON products(sku_code);
CREATE INDEX idx_products_type ON products(type);
CREATE INDEX idx_products_category ON products(category);
CREATE INDEX idx_products_brand ON products(brand);
CREATE INDEX idx_products_is_active ON products(is_active);

CREATE TABLE skus (
    id SERIAL PRIMARY KEY,
    product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    digi_sku_code VARCHAR(50) NOT NULL,
    seller_name VARCHAR(100),
    priority SMALLINT NOT NULL CHECK (priority IN (1, 2, 3)),
    price INT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    support_multi BOOLEAN NOT NULL DEFAULT true,
    unlimited_stock BOOLEAN NOT NULL DEFAULT true,
    stock INT NOT NULL DEFAULT 0,
    cut_off_start TIME NOT NULL DEFAULT '00:00:00',
    cut_off_end TIME NOT NULL DEFAULT '00:00:00',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(product_id, priority),
    UNIQUE(digi_sku_code)
);

CREATE INDEX idx_skus_product_id ON skus(product_id);
CREATE INDEX idx_skus_digi_sku_code ON skus(digi_sku_code);
CREATE INDEX idx_skus_priority ON skus(priority);
CREATE INDEX idx_skus_is_active ON skus(is_active);

CREATE TABLE transactions (
    id SERIAL PRIMARY KEY,
    transaction_id VARCHAR(50) NOT NULL UNIQUE,
    reference_id VARCHAR(50) NOT NULL,
    client_id INT NOT NULL REFERENCES clients(id),
    product_id INT NOT NULL REFERENCES products(id),
    sku_id INT REFERENCES skus(id),
    is_sandbox BOOLEAN NOT NULL DEFAULT false,
    customer_no VARCHAR(50) NOT NULL,
    customer_name VARCHAR(100),
    type transaction_type NOT NULL,
    status transaction_status NOT NULL DEFAULT 'Processing',
    serial_number VARCHAR(100),
    amount INT,
    admin INT DEFAULT 0,
    period VARCHAR(50),
    description JSONB,
    failed_reason VARCHAR(255),
    failed_code VARCHAR(10),
    retry_count SMALLINT NOT NULL DEFAULT 0,
    max_retry SMALLINT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    inquiry_id INT REFERENCES transactions(id),
    digi_ref_id VARCHAR(100),
    callback_sent BOOLEAN NOT NULL DEFAULT false,
    callback_sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(client_id, reference_id)
);

CREATE INDEX idx_transactions_transaction_id ON transactions(transaction_id);
CREATE INDEX idx_transactions_reference_id ON transactions(reference_id);
CREATE INDEX idx_transactions_client_id ON transactions(client_id);
CREATE INDEX idx_transactions_product_id ON transactions(product_id);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_type ON transactions(type);
CREATE INDEX idx_transactions_is_sandbox ON transactions(is_sandbox);
CREATE INDEX idx_transactions_created_at ON transactions(created_at);
CREATE INDEX idx_transactions_next_retry_at ON transactions(next_retry_at) WHERE status = 'Pending';
CREATE INDEX idx_transactions_inquiry_pending ON transactions(expired_at) WHERE type = 'inquiry' AND status = 'Success';
CREATE INDEX idx_transactions_callback_pending ON transactions(callback_sent) WHERE callback_sent = false AND status IN ('Success', 'Failed');

CREATE TABLE transaction_logs (
    id SERIAL PRIMARY KEY,
    transaction_id INT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    sku_id INT NOT NULL REFERENCES skus(id),
    digi_ref_id VARCHAR(100) NOT NULL,
    request JSONB NOT NULL,
    response JSONB,
    rc VARCHAR(10),
    status VARCHAR(20),
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_at TIMESTAMPTZ,
    response_time_ms INT
);

CREATE INDEX idx_transaction_logs_transaction_id ON transaction_logs(transaction_id);
CREATE INDEX idx_transaction_logs_digi_ref_id ON transaction_logs(digi_ref_id);
CREATE INDEX idx_transaction_logs_rc ON transaction_logs(rc);
CREATE INDEX idx_transaction_logs_created_at ON transaction_logs(created_at);

CREATE TABLE digiflazz_callbacks (
    id SERIAL PRIMARY KEY,
    digi_ref_id VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    rc VARCHAR(10),
    status VARCHAR(20),
    serial_number VARCHAR(100),
    message TEXT,
    is_processed BOOLEAN NOT NULL DEFAULT false,
    processed_at TIMESTAMPTZ,
    process_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_digiflazz_callbacks_digi_ref_id ON digiflazz_callbacks(digi_ref_id);
CREATE INDEX idx_digiflazz_callbacks_is_processed ON digiflazz_callbacks(is_processed) WHERE is_processed = false;
CREATE INDEX idx_digiflazz_callbacks_created_at ON digiflazz_callbacks(created_at);
