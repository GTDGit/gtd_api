-- ============================================
-- Migration 000063: Refactor disbursement -> payout
-- ============================================
-- Refactors the in-development transfer/disbursement schema into the payout
-- model that mirrors the payment system:
--   * tables renamed transfers* -> payouts* (data preserved, no DROP)
--   * public id column transfer_id -> payout_id
--   * new request-shaped columns (method_type, channel_code, fee_paid_by,
--     send_amount, customer_*, description, callback_url, callback_attempts)
--   * legacy bank-only NOT NULL columns relaxed (e-wallet payouts omit them)
--   * payout_routes: per-type (BANK/EWALLET) priority-ordered provider routing
--     with automatic fallback (capability is checked in the adapter, so no
--     per-bank-code rows are needed).

-- --------------------------------------------
-- 1. Rename tables (preserve data)
-- --------------------------------------------
ALTER TABLE transfer_inquiries RENAME TO payout_inquiries;
ALTER TABLE transfers           RENAME TO payouts;
ALTER TABLE transfer_logs       RENAME TO payout_logs;
ALTER TABLE transfer_callbacks  RENAME TO payout_callbacks;

-- --------------------------------------------
-- 2. Rename public-id columns transfer_id -> payout_id
-- --------------------------------------------
ALTER TABLE payouts          RENAME COLUMN transfer_id TO payout_id;
ALTER TABLE payout_logs      RENAME COLUMN transfer_id TO payout_id;   -- int FK -> payouts.id
ALTER TABLE payout_callbacks RENAME COLUMN transfer_id TO payout_id;   -- parsed public id

-- --------------------------------------------
-- 3. payout_inquiries: new columns
-- --------------------------------------------
ALTER TABLE payout_inquiries
    ADD COLUMN method_type  payout_method_type NOT NULL DEFAULT 'BANK',
    ADD COLUMN channel_code VARCHAR(20) NOT NULL DEFAULT '';
UPDATE payout_inquiries SET channel_code = bank_code WHERE channel_code = '';
-- transfer_type is meaningful only for BANK payouts; relax for e-wallet.
ALTER TABLE payout_inquiries ALTER COLUMN transfer_type DROP NOT NULL;

CREATE INDEX idx_payout_inquiries_method_type ON payout_inquiries(method_type);

-- --------------------------------------------
-- 4. payouts: new columns + relax legacy NOT NULLs
-- --------------------------------------------
ALTER TABLE payouts
    ADD COLUMN method_type       payout_method_type NOT NULL DEFAULT 'BANK',
    ADD COLUMN channel_code      VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN fee_paid_by       VARCHAR(10) NOT NULL DEFAULT 'merchant'
        CHECK (fee_paid_by IN ('merchant', 'customer')),
    ADD COLUMN send_amount       BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN customer_name     VARCHAR(100),
    ADD COLUMN customer_email    VARCHAR(100),
    ADD COLUMN customer_phone    VARCHAR(20),
    ADD COLUMN description       TEXT,
    ADD COLUMN callback_url      TEXT,
    ADD COLUMN callback_attempts INT NOT NULL DEFAULT 0;

UPDATE payouts SET channel_code = bank_code WHERE channel_code = '';
UPDATE payouts SET send_amount  = amount     WHERE send_amount = 0;

-- Bank-only columns become optional so e-wallet payouts can omit them.
ALTER TABLE payouts ALTER COLUMN transfer_type         DROP NOT NULL;
ALTER TABLE payouts ALTER COLUMN source_bank_code      DROP NOT NULL;
ALTER TABLE payouts ALTER COLUMN source_account_number DROP NOT NULL;

CREATE INDEX idx_payouts_method_type ON payouts(method_type);
CREATE INDEX idx_payouts_fee_paid_by ON payouts(fee_paid_by);
CREATE INDEX idx_payouts_callback_attempts ON payouts(callback_attempts);

-- --------------------------------------------
-- 5. payout_routes: per-type provider routing (Method_Provider_Mapping analog)
-- --------------------------------------------
CREATE TABLE payout_routes (
    id                  SERIAL PRIMARY KEY,
    method_type         payout_method_type NOT NULL,
    provider            disbursement_provider NOT NULL,
    priority            INT NOT NULL DEFAULT 100,   -- lower = preferred
    is_active           BOOLEAN NOT NULL DEFAULT true,
    is_maintenance      BOOLEAN NOT NULL DEFAULT false,
    maintenance_message TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (method_type, provider)
);

CREATE INDEX idx_payout_routes_type_priority ON payout_routes(method_type, priority);
CREATE INDEX idx_payout_routes_provider ON payout_routes(provider);

-- Seed routing: BANK -> pakailink > bri > bnc; EWALLET -> dana > pakailink.
-- The adapter's capability check (Supports) skips providers that cannot serve a
-- given channel_code (e.g. dana_direct only serves the DANA e-wallet), so the
-- selector falls through to the next priority automatically.
INSERT INTO payout_routes (method_type, provider, priority) VALUES
    ('BANK',    'pakailink',   1),
    ('BANK',    'bri_direct',  2),
    ('BANK',    'bnc_direct',  3),
    ('EWALLET', 'dana_direct', 1),
    ('EWALLET', 'pakailink',   2)
ON CONFLICT (method_type, provider) DO NOTHING;
