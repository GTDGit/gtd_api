-- ============================================
-- Migration 000032 DOWN: Remove workbook-based PPOB seed
-- WARNING: destructive reset for PPOB testing environments
-- ============================================

BEGIN;

DELETE FROM ppob_provider_callbacks;
DELETE FROM digiflazz_callbacks;
DELETE FROM transaction_logs;
DELETE FROM transactions;
DELETE FROM ppob_provider_health;
DELETE FROM ppob_provider_skus;
DELETE FROM skus;
DELETE FROM products;
DELETE FROM product_categories;
DELETE FROM product_brands;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ppob_provider_skus_provider_id_provider_sku_code_key'
    ) THEN
        ALTER TABLE ppob_provider_skus
            ADD CONSTRAINT ppob_provider_skus_provider_id_provider_sku_code_key
            UNIQUE (provider_id, provider_sku_code);
    END IF;
END $$;

COMMIT;
