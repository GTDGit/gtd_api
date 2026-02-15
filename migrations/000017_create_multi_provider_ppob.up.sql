-- ============================================
-- Migration 000017: Multi-Provider PPOB System
-- Supports: KIOSBANK, ALTERRA, DIGIFLAZZ (backup)
-- ============================================

-- Provider list table
CREATE TABLE ppob_providers (
    id SERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL UNIQUE,      -- 'kiosbank', 'alterra', 'digiflazz'
    name VARCHAR(50) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_backup BOOLEAN NOT NULL DEFAULT false, -- Digiflazz = backup only
    priority INT NOT NULL DEFAULT 100,        -- Lower = tried first (for same price)
    config JSONB DEFAULT '{}',                -- Provider-specific config
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ppob_providers_code ON ppob_providers(code);
CREATE INDEX idx_ppob_providers_is_active ON ppob_providers(is_active);
CREATE INDEX idx_ppob_providers_is_backup ON ppob_providers(is_backup);

-- Insert default providers
INSERT INTO ppob_providers (code, name, is_active, is_backup, priority, config) VALUES
    ('kiosbank', 'KIOSBANK', true, false, 1, '{"auth_type": "digest"}'),
    ('alterra', 'Alterra', true, false, 2, '{"auth_type": "rsa_sha256"}'),
    ('digiflazz', 'Digiflazz', true, true, 100, '{"auth_type": "md5_signature"}');

-- Provider SKU mapping table (links our products to provider's SKUs)
CREATE TABLE ppob_provider_skus (
    id SERIAL PRIMARY KEY,
    provider_id INT NOT NULL REFERENCES ppob_providers(id) ON DELETE CASCADE,
    product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    provider_sku_code VARCHAR(100) NOT NULL,  -- Provider's SKU code
    provider_product_name VARCHAR(200),        -- Provider's product name
    price INT NOT NULL DEFAULT 0,              -- Current price from provider
    admin INT NOT NULL DEFAULT 0,              -- Admin fee from provider
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_available BOOLEAN NOT NULL DEFAULT true, -- Based on provider sync
    stock INT DEFAULT NULL,                     -- NULL = unlimited
    last_sync_at TIMESTAMPTZ,
    sync_error VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider_id, provider_sku_code),
    UNIQUE(provider_id, product_id)            -- One mapping per product per provider
);

CREATE INDEX idx_ppob_provider_skus_provider_id ON ppob_provider_skus(provider_id);
CREATE INDEX idx_ppob_provider_skus_product_id ON ppob_provider_skus(product_id);
CREATE INDEX idx_ppob_provider_skus_provider_sku_code ON ppob_provider_skus(provider_sku_code);
CREATE INDEX idx_ppob_provider_skus_is_active ON ppob_provider_skus(is_active);
CREATE INDEX idx_ppob_provider_skus_is_available ON ppob_provider_skus(is_available);
CREATE INDEX idx_ppob_provider_skus_price ON ppob_provider_skus(price);

-- Provider health tracking
CREATE TABLE ppob_provider_health (
    id SERIAL PRIMARY KEY,
    provider_id INT NOT NULL REFERENCES ppob_providers(id) ON DELETE CASCADE,
    total_requests INT NOT NULL DEFAULT 0,
    success_count INT NOT NULL DEFAULT 0,
    failed_count INT NOT NULL DEFAULT 0,
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    last_failure_reason VARCHAR(255),
    avg_response_time_ms INT DEFAULT 0,
    health_score DECIMAL(5,2) DEFAULT 100.00,  -- 0-100%
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider_id, date)
);

CREATE INDEX idx_ppob_provider_health_provider_id ON ppob_provider_health(provider_id);
CREATE INDEX idx_ppob_provider_health_date ON ppob_provider_health(date);
CREATE INDEX idx_ppob_provider_health_health_score ON ppob_provider_health(health_score);

-- Add provider tracking to transactions
ALTER TABLE transactions 
    ADD COLUMN provider_id INT REFERENCES ppob_providers(id),
    ADD COLUMN provider_sku_id INT REFERENCES ppob_provider_skus(id),
    ADD COLUMN provider_ref_id VARCHAR(100),
    ADD COLUMN provider_response JSONB;

CREATE INDEX idx_transactions_provider_id ON transactions(provider_id);
CREATE INDEX idx_transactions_provider_sku_id ON transactions(provider_sku_id);
CREATE INDEX idx_transactions_provider_ref_id ON transactions(provider_ref_id);

-- Add provider tracking to transaction logs
ALTER TABLE transaction_logs
    ADD COLUMN provider_id INT REFERENCES ppob_providers(id),
    ADD COLUMN provider_sku_id INT REFERENCES ppob_provider_skus(id);

CREATE INDEX idx_transaction_logs_provider_id ON transaction_logs(provider_id);

-- Provider callback logs (unified for all providers)
CREATE TABLE ppob_provider_callbacks (
    id SERIAL PRIMARY KEY,
    provider_id INT NOT NULL REFERENCES ppob_providers(id),
    provider_ref_id VARCHAR(100) NOT NULL,
    transaction_id INT REFERENCES transactions(id),
    payload JSONB NOT NULL,
    status VARCHAR(20),
    message TEXT,
    is_processed BOOLEAN NOT NULL DEFAULT false,
    processed_at TIMESTAMPTZ,
    process_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ppob_provider_callbacks_provider_id ON ppob_provider_callbacks(provider_id);
CREATE INDEX idx_ppob_provider_callbacks_provider_ref_id ON ppob_provider_callbacks(provider_ref_id);
CREATE INDEX idx_ppob_provider_callbacks_transaction_id ON ppob_provider_callbacks(transaction_id);
CREATE INDEX idx_ppob_provider_callbacks_is_processed ON ppob_provider_callbacks(is_processed) WHERE is_processed = false;

-- View for getting best price per product (excluding backup providers)
CREATE OR REPLACE VIEW v_product_best_price AS
SELECT 
    p.id AS product_id,
    p.sku_code,
    p.name AS product_name,
    p.category,
    p.brand,
    p.type,
    p.admin AS product_admin,
    MIN(ps.price) AS best_price,
    MIN(ps.admin) FILTER (WHERE ps.price = (
        SELECT MIN(ps2.price) 
        FROM ppob_provider_skus ps2 
        JOIN ppob_providers pr2 ON ps2.provider_id = pr2.id
        WHERE ps2.product_id = p.id 
        AND ps2.is_active = true 
        AND ps2.is_available = true
        AND pr2.is_active = true
        AND pr2.is_backup = false
    )) AS best_admin,
    p.is_active AS product_status
FROM products p
LEFT JOIN ppob_provider_skus ps ON p.id = ps.product_id AND ps.is_active = true AND ps.is_available = true
LEFT JOIN ppob_providers pr ON ps.provider_id = pr.id AND pr.is_active = true AND pr.is_backup = false
GROUP BY p.id, p.sku_code, p.name, p.category, p.brand, p.type, p.admin, p.is_active;

-- Function to get providers sorted by price for a product
CREATE OR REPLACE FUNCTION get_providers_by_price(p_product_id INT)
RETURNS TABLE (
    provider_id INT,
    provider_code VARCHAR(20),
    provider_sku_id INT,
    provider_sku_code VARCHAR(100),
    price INT,
    admin INT,
    is_backup BOOLEAN
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        pr.id,
        pr.code,
        ps.id,
        ps.provider_sku_code,
        ps.price,
        ps.admin,
        pr.is_backup
    FROM ppob_provider_skus ps
    JOIN ppob_providers pr ON ps.provider_id = pr.id
    WHERE ps.product_id = p_product_id
    AND ps.is_active = true
    AND ps.is_available = true
    AND pr.is_active = true
    ORDER BY pr.is_backup ASC, ps.price ASC, pr.priority ASC;
END;
$$ LANGUAGE plpgsql;
