-- Reverse 000069: drop the public UUID id and revert qris_type to Indonesian.

UPDATE qris_registrations SET qris_type = 'statis' WHERE qris_type = 'static';
ALTER TABLE qris_registrations ALTER COLUMN qris_type SET DEFAULT 'statis';

DROP INDEX IF EXISTS uq_qris_registration_id;
ALTER TABLE qris_registrations DROP COLUMN IF EXISTS registration_id;
