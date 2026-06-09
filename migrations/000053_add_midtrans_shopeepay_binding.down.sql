-- ============================================
-- Migration 000053 (down): Remove Midtrans binding for EWALLET/SHOPEEPAY
-- ============================================

DELETE FROM payment_method_providers
WHERE provider = 'midtrans'
  AND payment_method_id IN (
    SELECT id FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY'
  );
