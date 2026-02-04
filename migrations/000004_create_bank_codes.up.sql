-- ============================================
-- Migration 000004: Bank Codes Table
-- ============================================

CREATE TABLE bank_codes (
    id SERIAL PRIMARY KEY,
    code VARCHAR(10) NOT NULL UNIQUE,
    short_name VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    swift_code VARCHAR(20),
    support_va BOOLEAN NOT NULL DEFAULT false,
    support_disbursement BOOLEAN NOT NULL DEFAULT true,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bank_codes_code ON bank_codes(code);
CREATE INDEX idx_bank_codes_short_name ON bank_codes(short_name);
CREATE INDEX idx_bank_codes_is_active ON bank_codes(is_active);
