-- ============================================
-- Migration 000052: Seed Method <-> Provider matrix
-- ============================================
-- Idempotently ensures exactly one canonical payment_methods row per (type, code)
-- and seeds payment_method_providers (Method_Provider_Mapping) with the priority
-- order from design.md "Method <-> Provider Matrix (seed target)".
--
-- priority: lower = preferred (1 = highest). Within a method, providers are
-- numbered in the order listed for that method, starting at 1.
--
-- All inserts are idempotent (WHERE NOT EXISTS / ON CONFLICT DO NOTHING) so the
-- migration can be re-run without producing duplicates (Req 14.5).
--
-- VA bank codes (cross-checked against migrations 000043/000044):
--   BCA=014, BRI=002, BNI=009, MANDIRI=008, NEO=490, BSI=451, CIMB NIAGA=022,
--   PERMATA=013, MUAMALAT=147, DANAMON=011, MAYBANK=016, OCBC=028, PANIN=019,
--   SINARMAS=153, BJB=110, BSS (Bank Sumsel Babel)=120, BTPN=213.
-- ============================================

-- --------------------------------------------
-- 1. Ensure canonical payment_methods rows exist (one per type/code)
--    payment_methods.provider is a legacy NOT NULL column; it is no longer the
--    selection source of truth (payment_method_providers is). We set it to a
--    sensible default for rows we may need to create.
-- --------------------------------------------

-- QRIS MPM
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'QRIS', 'MPM', 'QRIS', 'pakailink', 'flat', 0, 1000, 10000000, 1800, true, 1, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM');

-- QRIS CPM
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'QRIS', 'CPM', 'QRIS CPM (Consumer Presented)', 'dana_direct', 'flat', 0, 1000, 10000000, 1800, true, 2, 'Dana'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'QRIS' AND code = 'CPM');

-- RETAIL ALFAMART
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'RETAIL', 'ALFAMART', 'Alfamart', 'xendit', 'flat', 2500, 10000, 5000000, 86400, true, 20, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'RETAIL' AND code = 'ALFAMART');

-- RETAIL INDOMARET
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'RETAIL', 'INDOMARET', 'Indomaret', 'xendit', 'flat', 2500, 10000, 5000000, 86400, true, 21, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'RETAIL' AND code = 'INDOMARET');

-- EWALLET DANA
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'DANA', 'Dana', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 30, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'DANA');

-- EWALLET GOPAY
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'GOPAY', 'GoPay', 'midtrans', 'flat', 0, 10000, 10000000, 900, true, 31, 'Midtrans'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'GOPAY');

-- EWALLET OVO
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'OVO', 'OVO', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 32, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO');

-- EWALLET LINKAJA
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'LINKAJA', 'LinkAja', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 33, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'LINKAJA');

-- EWALLET SHOPEEPAY
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'SHOPEEPAY', 'ShopeePay', 'pakailink', 'flat', 0, 10000, 10000000, 900, true, 34, 'Pakailink'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY');

-- EWALLET ASTRAPAY
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT 'EWALLET', 'ASTRAPAY', 'AstraPay', 'xendit', 'flat', 0, 10000, 10000000, 900, true, 35, 'Xendit'
WHERE NOT EXISTS (SELECT 1 FROM payment_methods WHERE type = 'EWALLET' AND code = 'ASTRAPAY');

-- VA banks (one canonical row per bank code)
INSERT INTO payment_methods (type, code, name, provider, fee_type, fee_flat, min_amount, max_amount, expired_duration, is_active, display_order, provider_display_name)
SELECT v.type, v.code, v.name, 'pakailink', 'flat', 4000, 10000, 100000000, 86400, true, v.display_order, 'Pakailink'
FROM (VALUES
    ('VA'::payment_type, '014', 'BCA Virtual Account',        44),
    ('VA'::payment_type, '002', 'BRI Virtual Account',        40),
    ('VA'::payment_type, '009', 'BNI Virtual Account',        41),
    ('VA'::payment_type, '008', 'Mandiri Virtual Account',    43),
    ('VA'::payment_type, '490', 'Bank Neo Virtual Account',   42),
    ('VA'::payment_type, '451', 'BSI Virtual Account',        45),
    ('VA'::payment_type, '022', 'CIMB Niaga Virtual Account', 46),
    ('VA'::payment_type, '013', 'Permata Virtual Account',    47),
    ('VA'::payment_type, '147', 'Muamalat Virtual Account',   48),
    ('VA'::payment_type, '011', 'Danamon Virtual Account',    49),
    ('VA'::payment_type, '016', 'Maybank Virtual Account',    50),
    ('VA'::payment_type, '028', 'OCBC Virtual Account',       51),
    ('VA'::payment_type, '019', 'Panin Virtual Account',      52),
    ('VA'::payment_type, '153', 'Sinarmas Virtual Account',   53),
    ('VA'::payment_type, '110', 'BJB Virtual Account',        54),
    ('VA'::payment_type, '120', 'Bank Sumsel Babel Virtual Account', 55),
    ('VA'::payment_type, '213', 'BTPN Virtual Account',       56)
) AS v(type, code, name, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM payment_methods pm WHERE pm.type = v.type AND pm.code = v.code
);

-- --------------------------------------------
-- 2. Seed payment_method_providers (priority order from the design matrix)
-- --------------------------------------------

-- ===== QRIS MPM: pakailink, xendit, midtrans, dana_direct =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink',   1, 'QRIS' FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',      2, 'QRIS' FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'midtrans',    3, 'QRIS' FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 4, 'QRIS' FROM payment_methods WHERE type = 'QRIS' AND code = 'MPM'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== QRIS CPM: dana_direct =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 1, 'QRIS' FROM payment_methods WHERE type = 'QRIS' AND code = 'CPM'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== RETAIL ALFAMART: xendit, pakailink =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',    1, 'ALFAMART' FROM payment_methods WHERE type = 'RETAIL' AND code = 'ALFAMART'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink', 2, 'ALFAMART' FROM payment_methods WHERE type = 'RETAIL' AND code = 'ALFAMART'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== RETAIL INDOMARET: xendit, pakailink =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',    1, 'INDOMARET' FROM payment_methods WHERE type = 'RETAIL' AND code = 'INDOMARET'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink', 2, 'INDOMARET' FROM payment_methods WHERE type = 'RETAIL' AND code = 'INDOMARET'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET DANA: pakailink, dana_direct =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink',   1, 'DANA' FROM payment_methods WHERE type = 'EWALLET' AND code = 'DANA'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 2, 'DANA' FROM payment_methods WHERE type = 'EWALLET' AND code = 'DANA'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET GOPAY: midtrans, pakailink =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'midtrans',  1, 'GOPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'GOPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink', 2, 'GOPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'GOPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET OVO: pakailink, dana_direct, xendit, ovo_direct =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink',   1, 'OVO' FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 2, 'OVO' FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',      3, 'OVO' FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'ovo_direct',  4, 'OVO' FROM payment_methods WHERE type = 'EWALLET' AND code = 'OVO'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET LINKAJA: pakailink, dana_direct, xendit =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink',   1, 'LINKAJA' FROM payment_methods WHERE type = 'EWALLET' AND code = 'LINKAJA'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 2, 'LINKAJA' FROM payment_methods WHERE type = 'EWALLET' AND code = 'LINKAJA'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',      3, 'LINKAJA' FROM payment_methods WHERE type = 'EWALLET' AND code = 'LINKAJA'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET SHOPEEPAY: pakailink, dana_direct, xendit =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'pakailink',   1, 'SHOPEEPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'dana_direct', 2, 'SHOPEEPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit',      3, 'SHOPEEPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'SHOPEEPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== EWALLET ASTRAPAY: xendit =====
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_channel)
SELECT id, 'xendit', 1, 'ASTRAPAY' FROM payment_methods WHERE type = 'EWALLET' AND code = 'ASTRAPAY'
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== VA: pakailink (priority 1 for every supported bank) =====
-- Banks: MUAMALAT(147), NEO(490), BNI(009), BRI(002), BCA(014), BSI(451),
--        CIMB NIAGA(022), DANAMON(011), MANDIRI(008), MAYBANK(016), OCBC(028),
--        PANIN(019), PERMATA(013), SINARMAS(153)
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'pakailink', 1, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code IN ('147','490','009','002','014','451','022','011','008','016','028','019','013','153')
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== VA: xendit (priority 2 where pakailink also serves; 1 where xendit-only) =====
-- Banks: BRI(002), BNI(009), MANDIRI(008), BSI(451), BSS(120), BJB(110), CIMB(022), PERMATA(013)
-- pakailink-shared banks -> priority 2
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'xendit', 2, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code IN ('002','009','008','451','022','013')
ON CONFLICT (payment_method_id, provider) DO NOTHING;
-- xendit-only banks (BSS 120, BJB 110) -> priority 1
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'xendit', 1, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code IN ('120','110')
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== VA: midtrans (priority 3) =====
-- Banks: BRI(002), BNI(009), MANDIRI(008), CIMB(022), PERMATA(013)
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'midtrans', 3, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code IN ('002','009','008','022','013')
ON CONFLICT (payment_method_id, provider) DO NOTHING;

-- ===== VA: dana_direct =====
-- Banks: BNI(009), BRI(002), MANDIRI(008), BTPN(213), CIMB(022), PERMATA(013), PANIN(019), BSI(451)
-- For banks shared with pakailink+xendit+midtrans -> priority 4
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'dana_direct', 4, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code IN ('009','002','008','022','013')
ON CONFLICT (payment_method_id, provider) DO NOTHING;
-- BSI(451): pakailink(1), xendit(2) -> dana_direct priority 3
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'dana_direct', 3, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code = '451'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
-- PANIN(019): pakailink(1) -> dana_direct priority 2
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'dana_direct', 2, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code = '019'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
-- BTPN(213): dana_direct-only -> priority 1
INSERT INTO payment_method_providers (payment_method_id, provider, priority, provider_bank_code, provider_channel)
SELECT id, 'dana_direct', 1, code, 'VA' FROM payment_methods
WHERE type = 'VA' AND code = '213'
ON CONFLICT (payment_method_id, provider) DO NOTHING;
