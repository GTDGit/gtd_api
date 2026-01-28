-- ============================================
-- Migration 000001: Extensions & Enum Types
-- ============================================

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- PPOB Enums
CREATE TYPE product_type AS ENUM ('prepaid', 'postpaid');
CREATE TYPE transaction_type AS ENUM ('prepaid', 'inquiry', 'payment');
CREATE TYPE transaction_status AS ENUM ('Processing', 'Success', 'Pending', 'Failed');

-- Payment Enums
CREATE TYPE payment_type AS ENUM ('VA', 'EWALLET', 'QRIS', 'RETAIL');
CREATE TYPE payment_status AS ENUM ('Pending', 'Paid', 'Expired', 'Cancelled', 'Failed', 'Refunded', 'Partial_Refund');
CREATE TYPE payment_provider AS ENUM (
    'bca_direct', 'bri_direct', 'bni_direct', 'mandiri_direct', 'bnc_direct',
    'dana_direct', 'ovo_direct', 'midtrans', 'pakailink', 'xendit'
);
CREATE TYPE fee_type AS ENUM ('flat', 'percent');
CREATE TYPE refund_status AS ENUM ('Pending', 'Processing', 'Success', 'Failed');
CREATE TYPE callback_type AS ENUM ('payment', 'settlement');

-- Disbursement Enums
CREATE TYPE transfer_type AS ENUM ('INTRABANK', 'INTERBANK');
CREATE TYPE transfer_status AS ENUM ('Processing', 'Success', 'Pending', 'Failed');
CREATE TYPE disbursement_provider AS ENUM (
    'bca_direct', 'bri_direct', 'bni_direct', 'mandiri_direct', 'bnc_direct'
);

-- Identity Enums
CREATE TYPE identity_doc_type AS ENUM ('KTP', 'NPWP', 'SIM');
CREATE TYPE liveness_status AS ENUM ('Pending', 'Processing', 'Passed', 'Failed', 'Expired');
CREATE TYPE ocr_status AS ENUM ('Pending', 'Processing', 'Success', 'Failed');
