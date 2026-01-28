-- ============================================
-- Migration 000013: Functions & Triggers
-- ============================================

-- ============================================
-- SECTION 1: UTILITY FUNCTIONS
-- ============================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- SECTION 2: TRIGGERS FOR updated_at
-- ============================================

CREATE TRIGGER update_clients_updated_at
    BEFORE UPDATE ON clients
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_admin_users_updated_at
    BEFORE UPDATE ON admin_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_skus_updated_at
    BEFORE UPDATE ON skus
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_transactions_updated_at
    BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payment_methods_updated_at
    BEFORE UPDATE ON payment_methods
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_refunds_updated_at
    BEFORE UPDATE ON refunds
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_bank_codes_updated_at
    BEFORE UPDATE ON bank_codes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- SECTION 3: PRODUCT STATUS FUNCTION
-- ============================================

-- Function to update product status based on SKUs
CREATE OR REPLACE FUNCTION update_product_status()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE products
    SET is_active = EXISTS (
        SELECT 1 FROM skus
        WHERE product_id = COALESCE(NEW.product_id, OLD.product_id)
        AND is_active = true
    )
    WHERE id = COALESCE(NEW.product_id, OLD.product_id);

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_product_status_on_sku_change
    AFTER INSERT OR UPDATE OR DELETE ON skus
    FOR EACH ROW EXECUTE FUNCTION update_product_status();

-- ============================================
-- SECTION 4: ID GENERATOR FUNCTIONS
-- ============================================

-- Function to generate transaction_id
-- Format: GRB-YYYYMMDD-NNNNNN
CREATE OR REPLACE FUNCTION generate_transaction_id()
RETURNS VARCHAR(50) AS $$
DECLARE
    today_date VARCHAR(8);
    seq_num INT;
    new_id VARCHAR(50);
BEGIN
    today_date := TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD');

    SELECT COALESCE(MAX(
        CAST(SUBSTRING(transaction_id FROM 14) AS INT)
    ), 0) + 1 INTO seq_num
    FROM transactions
    WHERE transaction_id LIKE 'GRB-' || today_date || '-%';

    new_id := 'GRB-' || today_date || '-' || LPAD(seq_num::TEXT, 6, '0');

    RETURN new_id;
END;
$$ LANGUAGE plpgsql;

-- Function to generate payment_id
-- Format: PAY-YYYYMMDD-NNNNNN
CREATE OR REPLACE FUNCTION generate_payment_id()
RETURNS VARCHAR(50) AS $$
DECLARE
    today_date VARCHAR(8);
    seq_num INT;
    new_id VARCHAR(50);
BEGIN
    today_date := TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD');

    SELECT COALESCE(MAX(
        CAST(SUBSTRING(payment_id FROM 14) AS INT)
    ), 0) + 1 INTO seq_num
    FROM payments
    WHERE payment_id LIKE 'PAY-' || today_date || '-%';

    new_id := 'PAY-' || today_date || '-' || LPAD(seq_num::TEXT, 6, '0');

    RETURN new_id;
END;
$$ LANGUAGE plpgsql;

-- Function to generate refund_id
-- Format: REF-YYYYMMDD-NNNNNN
CREATE OR REPLACE FUNCTION generate_refund_id()
RETURNS VARCHAR(50) AS $$
DECLARE
    today_date VARCHAR(8);
    seq_num INT;
    new_id VARCHAR(50);
BEGIN
    today_date := TO_CHAR(NOW() AT TIME ZONE 'Asia/Jakarta', 'YYYYMMDD');

    SELECT COALESCE(MAX(
        CAST(SUBSTRING(refund_id FROM 14) AS INT)
    ), 0) + 1 INTO seq_num
    FROM refunds
    WHERE refund_id LIKE 'REF-' || today_date || '-%';

    new_id := 'REF-' || today_date || '-' || LPAD(seq_num::TEXT, 6, '0');

    RETURN new_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- SECTION 5: BUSINESS LOGIC FUNCTIONS
-- ============================================

-- Function to calculate payment fee
CREATE OR REPLACE FUNCTION calculate_payment_fee(
    p_amount BIGINT,
    p_fee_type fee_type,
    p_fee_flat INT,
    p_fee_percent DECIMAL(5,2),
    p_fee_min INT,
    p_fee_max INT
)
RETURNS BIGINT AS $$
DECLARE
    calculated_fee BIGINT;
BEGIN
    IF p_fee_type = 'flat' THEN
        calculated_fee := p_fee_flat;
    ELSE
        calculated_fee := CEIL(p_amount * p_fee_percent / 100);

        -- Apply min/max
        IF p_fee_min > 0 AND calculated_fee < p_fee_min THEN
            calculated_fee := p_fee_min;
        END IF;
        IF p_fee_max > 0 AND calculated_fee > p_fee_max THEN
            calculated_fee := p_fee_max;
        END IF;
    END IF;

    RETURN calculated_fee;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- SECTION 6: QUERY HELPER FUNCTIONS
-- ============================================

-- Get available SKUs for a product (not in cutoff)
CREATE OR REPLACE FUNCTION get_available_skus(p_product_id INT, p_current_time TIME DEFAULT LOCALTIME)
RETURNS TABLE (
    sku_id INT,
    digi_sku_code VARCHAR(50),
    priority SMALLINT,
    price INT,
    support_multi BOOLEAN
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        s.id,
        s.digi_sku_code,
        s.priority,
        s.price,
        s.support_multi
    FROM skus s
    WHERE s.product_id = p_product_id
      AND s.is_active = true
      AND (
          (s.cut_off_start = '00:00:00' AND s.cut_off_end = '00:00:00')
          OR
          (s.cut_off_start < s.cut_off_end AND NOT (p_current_time >= s.cut_off_start AND p_current_time <= s.cut_off_end))
          OR
          (s.cut_off_start > s.cut_off_end AND NOT (p_current_time >= s.cut_off_start OR p_current_time <= s.cut_off_end))
      )
    ORDER BY s.priority ASC;
END;
$$ LANGUAGE plpgsql;

-- Get pending transactions for retry
CREATE OR REPLACE FUNCTION get_pending_transactions_for_retry()
RETURNS TABLE (
    id INT,
    transaction_id VARCHAR(50),
    product_id INT,
    customer_no VARCHAR(50),
    type transaction_type,
    retry_count SMALLINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        t.id,
        t.transaction_id,
        t.product_id,
        t.customer_no,
        t.type,
        t.retry_count
    FROM transactions t
    WHERE t.status = 'Pending'
      AND t.next_retry_at <= NOW()
      AND t.retry_count < t.max_retry
    ORDER BY t.next_retry_at ASC
    FOR UPDATE SKIP LOCKED;
END;
$$ LANGUAGE plpgsql;

-- Get pending callbacks for retry
CREATE OR REPLACE FUNCTION get_pending_callbacks_for_retry()
RETURNS TABLE (
    id INT,
    client_id INT,
    transaction_id INT,
    payment_id INT,
    event VARCHAR(50),
    payload JSONB,
    attempt SMALLINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id,
        c.client_id,
        c.transaction_id,
        c.payment_id,
        c.event,
        c.payload,
        c.attempt
    FROM callback_logs c
    WHERE c.is_delivered = false
      AND c.next_retry_at <= NOW()
      AND c.attempt < c.max_attempts
    ORDER BY c.next_retry_at ASC
    FOR UPDATE SKIP LOCKED;
END;
$$ LANGUAGE plpgsql;

-- Get expired pending payments
CREATE OR REPLACE FUNCTION get_expired_pending_payments()
RETURNS TABLE (
    id INT,
    payment_id VARCHAR(50),
    client_id INT,
    expired_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        p.id,
        p.payment_id,
        p.client_id,
        p.expired_at
    FROM payments p
    WHERE p.status = 'pending'
      AND p.expired_at <= NOW()
    ORDER BY p.expired_at ASC
    FOR UPDATE SKIP LOCKED;
END;
$$ LANGUAGE plpgsql;
