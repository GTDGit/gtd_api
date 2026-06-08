-- Migration 000050 (down): Remove fee_paid_by column from payments

DROP INDEX IF EXISTS idx_payments_fee_paid_by;

ALTER TABLE payments
    DROP COLUMN IF EXISTS fee_paid_by;
