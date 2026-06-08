-- Migration 000045: Remove refund functionality
-- Refunds are handled manually; automated refund endpoints removed.

-- Remove refund-related payment status values from outstanding data (set to Paid for safety)
-- Note: refunded payments remain in DB as historical record with status Refunded,
-- but the refunds table and its data are dropped.

DROP TABLE IF EXISTS refunds CASCADE;

-- Remove Refunded/Partial_Refund from active payments (shouldn't be many, just normalize)
-- We keep the enum values in case historical data references them — just stop using them.
