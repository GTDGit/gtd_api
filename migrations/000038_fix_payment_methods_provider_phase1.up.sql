-- ============================================
-- Migration 000038: Phase 1 payment method routing
-- ============================================
-- Phase 1 routes BRI/BNI/Mandiri/Bank Neo VA through Pakailink until
-- direct integrations are built. OVO remains disabled (no provider wired).

UPDATE payment_methods
   SET provider = 'pakailink'
 WHERE type = 'VA'
   AND code IN ('002','009','008','490');

UPDATE payment_methods
   SET is_active = false
 WHERE type = 'EWALLET'
   AND code = 'OVO';
