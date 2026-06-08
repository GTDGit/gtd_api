-- Migration 000047: Deactivate legacy ewallet codes (DANA, OVO, GOPAY, SHOPEEPAY, LINKAJA)
-- These conflict with new plain codes (same meaning, different casing from old migrations).
-- We cannot DELETE because existing payments may reference them via payment_method_id FK.
-- Instead: deactivate them so they don't appear in the list endpoint.
-- Migration 049 will restore/ensure the correct plain codes exist.

UPDATE payment_methods
SET is_active = false
WHERE type = 'EWALLET'
  AND code IN ('DANA', 'OVO', 'GOPAY', 'SHOPEEPAY', 'LINKAJA')
  AND EXISTS (
    SELECT 1 FROM payments WHERE payment_method_id = payment_methods.id LIMIT 1
  );

-- For methods with NO existing payments, it's safe to delete
DELETE FROM payment_methods
WHERE type = 'EWALLET'
  AND code IN ('DANA', 'OVO', 'GOPAY', 'SHOPEEPAY', 'LINKAJA')
  AND NOT EXISTS (
    SELECT 1 FROM payments WHERE payment_method_id = payment_methods.id LIMIT 1
  );
