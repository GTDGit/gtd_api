-- Migration 000058: drop the per-client webhook_key column
-- Outbound payment webhooks are signed with callback_secret again, so the
-- dedicated webhook_key (added in 000056) is no longer used by any code path.

ALTER TABLE clients DROP COLUMN IF EXISTS webhook_key;
