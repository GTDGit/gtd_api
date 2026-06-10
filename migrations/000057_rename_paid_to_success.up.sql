-- Migration 000057: rename payment status 'Paid' -> 'Success'
-- Renames the enum label in place; all existing rows with 'Paid' become 'Success'
-- automatically. Transaction-safe on PostgreSQL 12+.
--
-- Note: legacy enum values 'Refunded' and 'Partial_Refund' are intentionally left
-- in the type. Removing an enum value requires rebuilding the type; the application
-- code no longer writes these values, so they remain as inert historical labels.

ALTER TYPE payment_status RENAME VALUE 'Paid' TO 'Success';
