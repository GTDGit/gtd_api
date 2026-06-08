-- Rollback: restore legacy ewallet codes (best-effort, provider may differ)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order)
VALUES
  ('EWALLET', 'DANA',      'Dana',      'pakailink',  'flat', 0, 10000, 10000000, 900, true, 30),
  ('EWALLET', 'OVO',       'OVO',       'ovo_direct', 'flat', 0, 10000, 10000000, 900, false, 32),
  ('EWALLET', 'GOPAY',     'GoPay',     'pakailink',  'flat', 0, 10000, 10000000, 900, false, 31),
  ('EWALLET', 'SHOPEEPAY', 'ShopeePay', 'xendit',     'flat', 0, 10000, 10000000, 900, false, 34)
ON CONFLICT (type, code) DO NOTHING;
