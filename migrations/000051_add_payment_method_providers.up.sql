-- ============================================
-- Migration 000051: Method <-> Provider mapping (Method_Provider_Mapping)
-- One row binds one canonical payment_methods entry to one provider with a
-- priority. Fully-typed columns, no generic metadata blob.
-- ============================================

CREATE TABLE IF NOT EXISTS payment_method_providers (
    id                  SERIAL PRIMARY KEY,
    payment_method_id   INT NOT NULL REFERENCES payment_methods(id) ON DELETE CASCADE,
    provider            payment_provider NOT NULL,
    priority            INT NOT NULL DEFAULT 100,   -- lower = preferred
    is_active           BOOLEAN NOT NULL DEFAULT true,
    is_maintenance      BOOLEAN NOT NULL DEFAULT false,
    maintenance_message TEXT,
    -- provider-specific routing hints, fully typed (no generic metadata blob)
    provider_bank_code  VARCHAR(20),                -- e.g. Xendit channel for a bank
    provider_channel    VARCHAR(40),                -- e.g. "OVO", "ALFAMART", "QRIS"
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (payment_method_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_pmp_method   ON payment_method_providers(payment_method_id);
CREATE INDEX IF NOT EXISTS idx_pmp_priority ON payment_method_providers(payment_method_id, priority);
CREATE INDEX IF NOT EXISTS idx_pmp_provider ON payment_method_providers(provider);
