-- ============================================
-- Migration 000066 (down)
-- ============================================

-- Revert payouts.status back to the shared transfer_status enum.
ALTER TABLE payouts ALTER COLUMN status DROP DEFAULT;
ALTER TABLE payouts
    ALTER COLUMN status TYPE transfer_status
    USING status::text::transfer_status;
ALTER TABLE payouts ALTER COLUMN status SET DEFAULT 'Processing';

DROP TYPE IF EXISTS payout_status;

DROP TABLE IF EXISTS payout_methods;
