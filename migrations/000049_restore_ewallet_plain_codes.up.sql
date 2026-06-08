-- Migration 000049: Restore plain ewallet codes (DANA, GOPAY, OVO, LINKAJA, SHOPEEPAY)
-- Migration 047 incorrectly deleted these. Our internal codes use plain names.
-- The PAY-prefixed mapping to Pakailink is done in application code, not DB.

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'DANA', 'Dana', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 30, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'DANA');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'GOPAY', 'GoPay', 'midtrans', 'flat', 0, 10000, 10000000, 900, true, 31, 'Midtrans'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'GOPAY');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'OVO', 'OVO', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 32, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'LINKAJA', 'LinkAja', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 33, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'LINKAJA');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'SHOPEEPAY', 'ShopeePay', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 34, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'ASTRAPAY', 'AstraPay', 'xendit', 'flat', 0, 10000, 10000000, 900, false, 35, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'ASTRAPAY');

-- Remove the PAY-prefixed duplicates that were added in 000044 (those were wrong)
DELETE FROM payment_methods WHERE type = 'EWALLET' AND code IN ('PAYDANA','PAYGOPAY','PAYOVO','PAYLINKAJA','PAYSHOPEE','PAYASTRAPAY');
