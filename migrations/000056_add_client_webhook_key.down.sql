-- Migration 000056 (down): drop webhook signing key column.

ALTER TABLE clients DROP COLUMN IF EXISTS webhook_key;
