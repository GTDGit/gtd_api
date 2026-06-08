-- Migration 000046: Add QRIS CPM payment method
-- CPM = Consumer Presented Mode: merchant scans customer's QR from DANA app.
-- Only dana_direct supports CPM currently.

INSERT INTO payment_methods (
    type, code, name, provider, fee_type, fee_flat,
    min_amount, max_amount, expired_duration,
    is_active, display_order, provider_display_name
)
SELECT
    'QRIS', 'CPM', 'QRIS CPM (Consumer Presented)', 'dana_direct',
    'flat', 0, 1000, 10000000, 1800,
    true, 2, 'Dana'
WHERE NOT EXISTS (
    SELECT 1 FROM payment_methods WHERE type = 'QRIS' AND code = 'CPM'
);
