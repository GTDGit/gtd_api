-- ============================================
-- Migration 000053: Add missing Midtrans binding for EWALLET/SHOPEEPAY
-- ============================================
-- Per the Method <-> Provider matrix in Fixing.md, SHOPEEPAY is served by
-- Pakailink, Dana Direct, Xendit, AND Midtrans. The seed in 000052 only added
-- pakailink(1), dana_direct(2), xendit(3) and missed midtrans. This forward-only
-- migration adds the midtrans binding at the next priority (4) so it acts as the
-- last fallback. Idempotent via ON CONFLICT DO NOTHING (Req 14.5).
-- ============================================

INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'midtrans', 4, 'SHOPEEPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
