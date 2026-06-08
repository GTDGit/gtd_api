-- Migration 000048: Fix VA providers to match spec
-- Pakailink is the primary provider for all common VA banks.
-- Banks only on Xendit (BJB=110, BSSB=120) and Dana (BTPN=213) stay as-is.

-- Fix VA names to match Pakailink bank code format and set pakailink as provider
-- for banks that are supported on Pakailink.
UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink',
    name = 'BRI Virtual Account',
    is_active = true
WHERE type = 'VA' AND code = '002';

UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink',
    name = 'BNI Virtual Account',
    is_active = true
WHERE type = 'VA' AND code = '009';

UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink',
    name = 'Mandiri Virtual Account',
    is_active = true
WHERE type = 'VA' AND code = '008';

UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink',
    name = 'Bank Neo Virtual Account',
    is_active = true
WHERE type = 'VA' AND code = '490';

-- Ensure BCA (014) is active with pakailink
UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink',
    name = 'BCA Virtual Account',
    is_active = true
WHERE type = 'VA' AND code = '014';

-- QRIS MPM: set default provider to pakailink (switchable via admin)
UPDATE payment_methods
SET provider = 'pakailink',
    provider_display_name = 'Pakailink'
WHERE type = 'QRIS' AND code = 'MPM';
