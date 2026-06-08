-- Migration 000047: Remove legacy ewallet codes (DANA, OVO, GOPAY, SHOPEEPAY, LINKAJA)
-- that conflict with the new PAY-prefixed codes (PAYDANA, PAYOVO, etc.)
-- The PAY-prefixed codes were seeded in 000044 and are the canonical codes going forward.

DELETE FROM payment_methods
WHERE type = 'EWALLET'
  AND code IN ('DANA', 'OVO', 'GOPAY', 'SHOPEEPAY', 'LINKAJA');

-- Ensure all PAY-prefixed ewallet methods are active
UPDATE payment_methods
SET is_active = true
WHERE type = 'EWALLET'
  AND code IN ('PAYDANA', 'PAYGOPAY', 'PAYOVO', 'PAYLINKAJA', 'PAYSHOPEE');

-- AstraPay remains inactive (no live integration yet)
UPDATE payment_methods
SET is_active = false
WHERE type = 'EWALLET' AND code = 'PAYASTRAPAY';
