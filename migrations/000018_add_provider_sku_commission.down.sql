-- ============================================
-- Migration 000018 DOWN: Remove commission column from ppob_provider_skus
-- ============================================

DROP INDEX IF EXISTS idx_ppob_provider_skus_effective_admin;

ALTER TABLE ppob_provider_skus
DROP COLUMN IF EXISTS commission;
