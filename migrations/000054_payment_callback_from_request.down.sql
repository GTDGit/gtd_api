-- ============================================
-- Migration 000054 (down): Restore client-level payment callback columns
-- ============================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS payment_callback_url TEXT;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS payment_callback_secret TEXT;

ALTER TABLE payments DROP COLUMN IF EXISTS callback_url;
