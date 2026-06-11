-- partner_ref is the reference WE generate and send to the upstream provider on
-- every payment (create/inquiry/cancel/refund). It is distinct from:
--   payment_id   - public UUIDv4 returned to the API client
--   reference_id - the client's own idempotency key
--   provider_ref - the id the provider returns to us
-- Providers like DANA cap partnerReferenceNo at 25 chars for QRIS, so partner_ref
-- is generated short (<=25) and collision-safe; using one stable value for both
-- create and inquiry prevents "transaction not found" mismatches.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS partner_ref TEXT;

-- Backfill existing rows with their payment_id so the value is non-null and
-- unique. Historical rows are mostly expired/failed test data and are not
-- re-queried; new rows receive a freshly generated short ref from the app.
UPDATE payments SET partner_ref = payment_id WHERE partner_ref IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_partner_ref ON payments (partner_ref);
