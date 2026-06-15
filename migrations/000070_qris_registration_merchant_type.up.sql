-- QRIS registration: add merchant_type (perorangan | perusahaan).
--
-- The Nobu form's "JENIS MERCHANT (PERORANGAN / BADAN USAHA)" column drives the
-- required onboarding documents: perorangan needs KTP + selfie + business photo;
-- perusahaan additionally needs akta, SK, NPWP, NIB. Existing rows default to
-- 'perorangan' (the common case) so the NOT NULL constraint holds.

ALTER TABLE qris_registrations ADD COLUMN IF NOT EXISTS merchant_type VARCHAR(20) NOT NULL DEFAULT 'perorangan';
