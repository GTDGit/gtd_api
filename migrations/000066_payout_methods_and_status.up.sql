-- ============================================
-- Migration 000066: payout_methods catalog + payout_status enum
-- ============================================
-- Standardizes the payout system with the payment system:
--   * payout_methods: per-channel catalog (BANK/EWALLET) with name + min/max
--     amount + fee config, mirroring payment_methods. Source of per-channel
--     minimum amounts (e.g. DANA 1000, other e-wallets 10000, banks 10000).
--   * payout_status: collapse the lifecycle to Processing/Success/Failed
--     (the legacy transfer_status enum carried an unused 'Pending' value).

-- --------------------------------------------
-- 1. payout_methods catalog
-- --------------------------------------------
CREATE TABLE payout_methods (
    id SERIAL PRIMARY KEY,
    method_type payout_method_type NOT NULL,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(50) NOT NULL,
    fee_type fee_type NOT NULL DEFAULT 'flat',
    fee_flat INT NOT NULL DEFAULT 0,
    fee_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    fee_min INT NOT NULL DEFAULT 0,
    fee_max INT NOT NULL DEFAULT 0,
    min_amount INT NOT NULL DEFAULT 10000,
    max_amount INT NOT NULL DEFAULT 50000000,
    logo_url VARCHAR(255),
    display_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_maintenance BOOLEAN NOT NULL DEFAULT false,
    maintenance_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(method_type, code)
);

CREATE INDEX idx_payout_methods_method_type ON payout_methods(method_type);
CREATE INDEX idx_payout_methods_is_active ON payout_methods(is_active);

-- Seed: BANK uses a single DEFAULT row as the floor for all disbursement banks
-- (individual banks live in bank_codes). E-wallets are catalogued per channel
-- so their minimums can differ (DANA accepts payouts from 1000).
INSERT INTO payout_methods (method_type, code, name, min_amount, max_amount, display_order) VALUES
    ('BANK',    'DEFAULT', 'Bank Transfer', 10000, 500000000, 1),
    ('EWALLET', 'DANA',    'DANA',           1000,  10000000, 1),
    ('EWALLET', 'OVO',     'OVO',           10000,  10000000, 2)
ON CONFLICT (method_type, code) DO NOTHING;

-- --------------------------------------------
-- 2. payout_status enum (Processing/Success/Failed)
-- --------------------------------------------
CREATE TYPE payout_status AS ENUM ('Processing', 'Success', 'Failed');

-- Convert payouts.status from transfer_status to payout_status, folding any
-- legacy 'Pending' rows into 'Processing'. transfer_status itself is left intact
-- (it is a shared type and is never dropped per the migration rules).
ALTER TABLE payouts ALTER COLUMN status DROP DEFAULT;
ALTER TABLE payouts
    ALTER COLUMN status TYPE payout_status
    USING (CASE WHEN status::text = 'Pending' THEN 'Processing' ELSE status::text END)::payout_status;
ALTER TABLE payouts ALTER COLUMN status SET DEFAULT 'Processing';
