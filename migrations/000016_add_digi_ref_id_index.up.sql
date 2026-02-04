-- ============================================
-- Migration 000019: Add missing index on digi_ref_id for callback lookup
-- ============================================

-- Add index on transactions.digi_ref_id for fast callback matching
CREATE INDEX IF NOT EXISTS idx_transactions_digi_ref_id ON transactions(digi_ref_id) WHERE digi_ref_id IS NOT NULL;

-- Add index on transaction_logs.digi_ref_id (already exists in create, but ensure)
-- This is for auditing and debugging purposes
