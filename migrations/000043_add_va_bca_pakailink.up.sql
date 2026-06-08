-- Add BCA Virtual Account via Pakailink provider
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, fee_min, fee_max, min_amount, max_amount, expired_duration, is_active, display_order)
SELECT 'VA', '014', 'BCA Virtual Account', 'pakailink', 'flat', 4000, 0, 0, 0, 10000, 100000000, 86400, true, 10
WHERE NOT EXISTS (
    SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '014' AND provider = 'pakailink'
);
