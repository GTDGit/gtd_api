-- ============================================
-- Migration 000062: Payout enums
-- ============================================
-- The payout (disbursement) system is refactored to mirror the payment system:
-- a per-type routing table (BANK/EWALLET) with priority-ordered providers and
-- automatic fallback. This migration only introduces the enum types/values so
-- they are committed before later migrations reference the new 'dana_direct'
-- value (Postgres forbids using a freshly-added enum value in the same tx).

-- Method type for payouts: bank transfer vs e-wallet top-up.
CREATE TYPE payout_method_type AS ENUM ('BANK', 'EWALLET');

-- DANA disbursement is added as a valid provider. The other providers used by
-- payouts (bri_direct, bnc_direct, pakailink) already exist on the enum.
ALTER TYPE disbursement_provider ADD VALUE IF NOT EXISTS 'dana_direct';
