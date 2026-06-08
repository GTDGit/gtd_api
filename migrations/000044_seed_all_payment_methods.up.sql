-- ============================================
-- Migration 000044: Seed all payment methods + add provider_display_name
-- ============================================

-- Add provider_display_name to payment_methods for pretty display in admin
ALTER TABLE payment_methods
    ADD COLUMN IF NOT EXISTS provider_display_name VARCHAR(50);

-- Seed display names for existing enum values
UPDATE payment_methods SET provider_display_name = CASE provider
    WHEN 'pakailink'      THEN 'Pakailink'
    WHEN 'dana_direct'    THEN 'Dana'
    WHEN 'midtrans'       THEN 'Midtrans'
    WHEN 'xendit'         THEN 'Xendit'
    WHEN 'ovo_direct'     THEN 'OVO'
    WHEN 'bca_direct'     THEN 'BCA Direct'
    WHEN 'bni_direct'     THEN 'BNI Direct'
    WHEN 'bri_direct'     THEN 'BRI Direct'
    WHEN 'mandiri_direct' THEN 'Mandiri Direct'
    WHEN 'bnc_direct'     THEN 'BNC Direct'
    ELSE provider::text
END
WHERE provider_display_name IS NULL;

-- ============================================
-- QRIS (type=QRIS, code=MPM)
-- Already seeded in 000042 — ensure it exists
-- ============================================
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'QRIS', 'MPM', 'QRIS', 'pakailink', 'flat', 0, 1000, 10000000, 1800, true, 1, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM');

-- ============================================
-- RETAIL
-- ============================================
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'RETAIL', 'ALFAMART', 'Alfamart', 'xendit', 'flat', 2500, 10000, 5000000, 86400, true, 20, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'RETAIL' AND code = 'ALFAMART');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'RETAIL', 'INDOMARET', 'Indomaret', 'xendit', 'flat', 2500, 10000, 5000000, 86400, true, 21, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'RETAIL' AND code = 'INDOMARET');

-- ============================================
-- EWALLET
-- ============================================
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYDANA', 'Dana', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 30, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYDANA');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYGOPAY', 'GoPay', 'midtrans', 'flat', 0, 10000, 10000000, 900, true, 31, 'Midtrans'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYGOPAY');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYOVO', 'OVO', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 32, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYOVO');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYLINKAJA', 'LinkAja', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 33, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYLINKAJA');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYSHOPEE', 'ShopeePay', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 34, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYSHOPEE');

INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'PAYASTRAPAY', 'AstraPay', 'xendit', 'flat', 0, 10000, 10000000, 900, false, 35, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'PAYASTRAPAY');

-- ============================================
-- VA (Virtual Account)
-- Pakailink: MUAMALAT, NEO, BNI, BRI, BCA, BSI, CIMB, DANAMON, MANDIRI, MAYBANK, OCBC, PANIN, PERMATA, SINARMAS
-- Xendit:    BRI, BNI, MANDIRI, BSI, BSSB, BJB, CIMB, PERMATA
-- Midtrans:  BRI, BNI, MANDIRI, CIMB, PERMATA
-- Dana:      BNI, BRI, MANDIRI, BTPN, CIMB, PERMATA, PANIN, BSI
--
-- Strategy: one row per bank code, provider = most capable/available
-- BRI    → pakailink (available across all 4, pakailink preferred)
-- BNI    → pakailink
-- MANDIRI→ pakailink
-- CIMB   → pakailink (available in all)
-- PERMATA→ pakailink (available in all)
-- BSI    → pakailink (pakailink + xendit + dana)
-- BCA    → pakailink (only pakailink)
-- MUAMALAT→ pakailink
-- NEO    → pakailink
-- DANAMON→ pakailink
-- MAYBANK→ pakailink
-- OCBC   → pakailink
-- PANIN  → pakailink (pakailink + dana)
-- SINARMAS→ pakailink
-- BJB    → xendit (only xendit)
-- BSSB   → xendit (only xendit — Bank Sumsel Babel)
-- BTPN   → dana_direct (only dana)
-- ============================================

-- BRI (002)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '002', 'BRI Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 40, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '002');

-- BNI (009)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '009', 'BNI Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 41, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '009');

-- Bank Neo Commerce (490) 
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '490', 'Bank Neo Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 42, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '490');

-- Mandiri (008)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '008', 'Mandiri Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 43, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '008');

-- BCA (014) - already in 000043, update display_name
UPDATE payment_methods SET provider_display_name = 'Pakailink', display_order = 44
WHERE type = 'VA' AND code = '014' AND provider_display_name IS NULL;

-- BSI (451)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '451', 'BSI Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 45, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '451');

-- CIMB Niaga (022)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '022', 'CIMB Niaga Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 46, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '022');

-- Permata (013)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '013', 'Permata Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 47, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '013');

-- Muamalat (147)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '147', 'Muamalat Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 48, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '147');

-- Danamon (011)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '011', 'Danamon Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 49, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '011');

-- Maybank (016)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '016', 'Maybank Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 50, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '016');

-- OCBC (028)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '028', 'OCBC Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 51, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '028');

-- Panin (019)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '019', 'Panin Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 52, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '019');

-- Sinarmas (153)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '153', 'Sinarmas Virtual Account', 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, 53, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '153');

-- BJB (110) - Xendit only
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '110', 'BJB Virtual Account', 'xendit', 'flat', 4000, 10000, 100000000, 86400, false, 54, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '110');

-- BSSB / Bank Sumsel Babel (120) - Xendit only
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '120', 'Bank Sumsel Babel Virtual Account', 'xendit', 'flat', 4000, 10000, 100000000, 86400, false, 55, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '120');

-- BTPN (213) - Dana only
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'VA', '213', 'BTPN Virtual Account', 'dana_direct', 'flat', 4000, 10000, 100000000, 86400, false, 56, 'Dana'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'VA' AND code = '213');
