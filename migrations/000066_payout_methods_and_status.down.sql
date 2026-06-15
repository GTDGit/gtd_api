-- ============================================
-- Migration 000066 (down)
-- ============================================

-- Revert payouts.status back to the shared transfer_status enum.
-- Drop the partial index first (its status predicate is bound to the current
-- enum and would block ALTER COLUMN TYPE), then recreate it afterwards.
DROP INDEX IF EXISTS idx_transfers_callback_pending;

ALTER TABLE payouts ALTER COLUMN status DROP DEFAULT;
ALTER TABLE payouts
    ALTER COLUMN status TYPE transfer_status
    USING status::text::transfer_status;
ALTER TABLE payouts ALTER COLUMN status SET DEFAULT 'Processing';

CREATE INDEX idx_transfers_callback_pending ON payouts(callback_sent)
    WHERE callback_sent = false AND status IN ('Success', 'Failed');

DROP TYPE IF EXISTS payout_status;

DROP TABLE IF EXISTS payout_methods;
