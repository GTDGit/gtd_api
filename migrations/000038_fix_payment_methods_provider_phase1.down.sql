-- Revert Phase 1 payment method routing.

UPDATE payment_methods SET provider = 'bri_direct'     WHERE type = 'VA' AND code = '002';
UPDATE payment_methods SET provider = 'bni_direct'     WHERE type = 'VA' AND code = '009';
UPDATE payment_methods SET provider = 'mandiri_direct' WHERE type = 'VA' AND code = '008';
UPDATE payment_methods SET provider = 'bnc_direct'     WHERE type = 'VA' AND code = '490';

UPDATE payment_methods SET is_active = true WHERE type = 'EWALLET' AND code = 'OVO';
