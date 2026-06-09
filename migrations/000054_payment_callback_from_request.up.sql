-- ============================================
-- Migration 000054: Callback URL comes from the request, not the client
-- ============================================
-- Per Fixing.md #2 + explicit request: url.callback is mandatory on create and
-- the webhook is delivered to that per-payment URL. The dedicated client-level
-- payment_callback_url / payment_callback_secret columns are removed. The HMAC
-- signing secret falls back to clients.callback_secret.
--
-- - payments.callback_url stores the request-provided callback (delivery is async).
-- - clients.payment_callback_url / payment_callback_secret dropped.
-- ============================================

ALTER TABLE payments ADD COLUMN IF NOT EXISTS callback_url TEXT;

ALTER TABLE clients DROP COLUMN IF EXISTS payment_callback_url;
ALTER TABLE clients DROP COLUMN IF EXISTS payment_callback_secret;
