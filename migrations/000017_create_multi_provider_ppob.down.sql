-- ============================================
-- Migration 000017: Multi-Provider PPOB System (DOWN)
-- ============================================

-- Drop function
DROP FUNCTION IF EXISTS get_providers_by_price(INT);

-- Drop view
DROP VIEW IF EXISTS v_product_best_price;

-- Drop callback logs
DROP TABLE IF EXISTS ppob_provider_callbacks;

-- Remove columns from transaction_logs
ALTER TABLE transaction_logs
    DROP COLUMN IF EXISTS provider_id,
    DROP COLUMN IF EXISTS provider_sku_id;

-- Remove columns from transactions
ALTER TABLE transactions 
    DROP COLUMN IF EXISTS provider_id,
    DROP COLUMN IF EXISTS provider_sku_id,
    DROP COLUMN IF EXISTS provider_ref_id,
    DROP COLUMN IF EXISTS provider_response;

-- Drop tables
DROP TABLE IF EXISTS ppob_provider_health;
DROP TABLE IF EXISTS ppob_provider_skus;
DROP TABLE IF EXISTS ppob_providers;
