-- Migration 000050: Add fee_paid_by column to payments
-- Promotes feePaidBy from metadata._feePaidBy to a first-class, typed column.
-- merchant (default) = total is subtotal; customer = total is subtotal + fee.

ALTER TABLE payments
    ADD COLUMN fee_paid_by VARCHAR(10) NOT NULL DEFAULT 'merchant'
        CHECK (fee_paid_by IN ('merchant', 'customer'));

CREATE INDEX idx_payments_fee_paid_by ON payments(fee_paid_by);

-- Backfill any historical value stored in metadata._feePaidBy into the new column.
UPDATE payments
SET fee_paid_by = metadata->>'_feePaidBy'
WHERE metadata->>'_feePaidBy' IN ('merchant', 'customer');
