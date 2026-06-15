-- QRIS registration: add a public UUID id + normalize qris_type to English enum.
--
-- Aligns the client-facing QRIS contract with the Payment API:
--   * registration_id (UUID v4) is the public identifier returned as `id` and
--     used in GET /v1/qris/merchants/{id}. The SERIAL `id` stays internal, and
--     `registration_ref` remains the per-client idempotency key.
--   * qris_type values move from Indonesian 'statis' to the English enum
--     {static, dynamic, both} that the API now exposes.

ALTER TABLE qris_registrations ADD COLUMN IF NOT EXISTS registration_id VARCHAR(50);

-- Backfill existing rows with a UUID; gen_random_uuid() is available via pgcrypto
-- (used elsewhere by the QRIS doc portal token default).
UPDATE qris_registrations SET registration_id = gen_random_uuid()::text WHERE registration_id IS NULL;

ALTER TABLE qris_registrations ALTER COLUMN registration_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_qris_registration_id
    ON qris_registrations (registration_id);

-- Normalize the existing default + data to the English enum.
ALTER TABLE qris_registrations ALTER COLUMN qris_type SET DEFAULT 'static';
UPDATE qris_registrations SET qris_type = 'static' WHERE qris_type = 'statis';
