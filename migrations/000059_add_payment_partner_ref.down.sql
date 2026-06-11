DROP INDEX IF EXISTS idx_payments_partner_ref;
ALTER TABLE payments DROP COLUMN IF EXISTS partner_ref;
