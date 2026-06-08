-- Add BCA Virtual Account via Pakailink provider
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, fee_percent, fee_min, fee_max, min_amount, max_amount, expired_duration, is_active, display_order)
VALUES ('VA', '014', 'BCA Virtual Account', 'pakailink', 'flat', 4000, 0, 0, 0, 10000, 100000000, 86400, true, 10)
ON CONFLICT (type, code, provider) DO NOTHING;
