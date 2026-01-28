-- ============================================
-- Migration 000013 DOWN: Drop Functions & Triggers
-- ============================================

-- Drop triggers first
DROP TRIGGER IF EXISTS update_clients_updated_at ON clients;
DROP TRIGGER IF EXISTS update_admin_users_updated_at ON admin_users;
DROP TRIGGER IF EXISTS update_products_updated_at ON products;
DROP TRIGGER IF EXISTS update_skus_updated_at ON skus;
DROP TRIGGER IF EXISTS update_transactions_updated_at ON transactions;
DROP TRIGGER IF EXISTS update_payment_methods_updated_at ON payment_methods;
DROP TRIGGER IF EXISTS update_payments_updated_at ON payments;
DROP TRIGGER IF EXISTS update_refunds_updated_at ON refunds;
DROP TRIGGER IF EXISTS update_bank_codes_updated_at ON bank_codes;
DROP TRIGGER IF EXISTS update_product_status_on_sku_change ON skus;

-- Drop functions
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS update_product_status();
DROP FUNCTION IF EXISTS generate_transaction_id();
DROP FUNCTION IF EXISTS generate_payment_id();
DROP FUNCTION IF EXISTS generate_refund_id();
DROP FUNCTION IF EXISTS calculate_payment_fee(BIGINT, fee_type, INT, DECIMAL(5,2), INT, INT);
DROP FUNCTION IF EXISTS get_available_skus(INT, TIME);
DROP FUNCTION IF EXISTS get_pending_transactions_for_retry();
DROP FUNCTION IF EXISTS get_pending_callbacks_for_retry();
DROP FUNCTION IF EXISTS get_expired_pending_payments();
