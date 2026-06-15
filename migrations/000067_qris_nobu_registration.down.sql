-- Reverse 000067. Per project rule we DROP only objects this migration created.

DROP TABLE IF EXISTS qris_callbacks;

ALTER TABLE qris_merchants DROP COLUMN IF EXISTS registration_id;
ALTER TABLE qris_merchants DROP COLUMN IF EXISTS sub_merchant_id;

-- Remove the batch FK before dropping the batches table it points at.
ALTER TABLE qris_registrations DROP CONSTRAINT IF EXISTS fk_qris_registration_batch;

DROP TABLE IF EXISTS qris_nobu_batches;
DROP TABLE IF EXISTS qris_registrations;
