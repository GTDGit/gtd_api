-- Migration 000058 (down): re-add the webhook_key column
-- Restores the structure from 000056 (without backfill).

ALTER TABLE clients ADD COLUMN IF NOT EXISTS webhook_key TEXT;
