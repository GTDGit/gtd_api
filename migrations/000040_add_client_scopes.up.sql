-- ============================================
-- Migration 000040: Add scopes column to clients
-- ============================================
-- Single API key with multi-scope flag. Existing clients backfilled
-- with all three scopes to preserve current behavior. New clients
-- can be issued with restricted scopes (e.g. ppob-only sub-brands).

ALTER TABLE clients
    ADD COLUMN IF NOT EXISTS scopes TEXT[] NOT NULL
    DEFAULT ARRAY['ppob','payment','disbursement']::TEXT[];

CREATE INDEX IF NOT EXISTS idx_clients_scopes ON clients USING GIN (scopes);
