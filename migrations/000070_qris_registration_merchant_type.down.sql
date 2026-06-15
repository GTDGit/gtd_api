-- Reverse 000070: drop merchant_type.

ALTER TABLE qris_registrations DROP COLUMN IF EXISTS merchant_type;
