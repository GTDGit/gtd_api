-- Migration 000056: dedicated webhook signing key per client
-- webhook_key is the HMAC-SHA256 secret used to sign outbound payment webhooks
-- delivered to the client's callback URL. It is independent of api_key/callback_secret
-- so it can be rotated without affecting authentication.

ALTER TABLE clients ADD COLUMN IF NOT EXISTS webhook_key TEXT;

-- Backfill existing clients with a generated key (gb_whsec_<64 hex>).
UPDATE clients
SET webhook_key = 'gb_whsec_' || encode(gen_random_bytes(32), 'hex')
WHERE webhook_key IS NULL OR webhook_key = '';
