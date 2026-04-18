-- ============================================
-- Migration 000037: Add payment callback columns to clients
-- ============================================
-- Clients get a dedicated webhook URL and secret for Payment events.
-- When NULL, PaymentCallbackService falls back to callback_url / callback_secret.

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS payment_callback_url    VARCHAR(255),
    ADD COLUMN IF NOT EXISTS payment_callback_secret VARCHAR(100);
