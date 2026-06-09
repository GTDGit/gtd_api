-- ============================================
-- Migration 000018: Drop payment_callbacks table
-- Reason: Per Fixing.md #10 - callback handling is now real-time in-memory only
-- ============================================

DROP INDEX IF EXISTS idx_payment_callbacks_created_at;
DROP INDEX IF EXISTS idx_payment_callbacks_is_processed;
DROP INDEX IF EXISTS idx_payment_callbacks_payment_id;
DROP INDEX IF EXISTS idx_payment_callbacks_provider_ref;
DROP INDEX IF EXISTS idx_payment_callbacks_provider;

DROP TABLE IF EXISTS payment_callbacks;