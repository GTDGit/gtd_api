-- Reverse 000063: payout -> disbursement/transfer naming and drop new columns.

DROP TABLE IF EXISTS payout_routes;

-- payouts: drop new columns and restore legacy NOT NULLs.
DROP INDEX IF EXISTS idx_payouts_callback_attempts;
DROP INDEX IF EXISTS idx_payouts_fee_paid_by;
DROP INDEX IF EXISTS idx_payouts_method_type;

ALTER TABLE payouts ALTER COLUMN source_account_number SET NOT NULL;
ALTER TABLE payouts ALTER COLUMN source_bank_code      SET NOT NULL;
ALTER TABLE payouts ALTER COLUMN transfer_type         SET NOT NULL;

ALTER TABLE payouts
    DROP COLUMN IF EXISTS callback_attempts,
    DROP COLUMN IF EXISTS callback_url,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS customer_phone,
    DROP COLUMN IF EXISTS customer_email,
    DROP COLUMN IF EXISTS customer_name,
    DROP COLUMN IF EXISTS send_amount,
    DROP COLUMN IF EXISTS fee_paid_by,
    DROP COLUMN IF EXISTS channel_code,
    DROP COLUMN IF EXISTS method_type;

DROP INDEX IF EXISTS idx_payout_inquiries_method_type;
ALTER TABLE payout_inquiries ALTER COLUMN transfer_type SET NOT NULL;
ALTER TABLE payout_inquiries
    DROP COLUMN IF EXISTS channel_code,
    DROP COLUMN IF EXISTS method_type;

-- Rename columns back.
ALTER TABLE payout_callbacks RENAME COLUMN payout_id TO transfer_id;
ALTER TABLE payout_logs      RENAME COLUMN payout_id TO transfer_id;
ALTER TABLE payouts          RENAME COLUMN payout_id TO transfer_id;

-- Rename tables back.
ALTER TABLE payout_callbacks  RENAME TO transfer_callbacks;
ALTER TABLE payout_logs       RENAME TO transfer_logs;
ALTER TABLE payouts           RENAME TO transfers;
ALTER TABLE payout_inquiries  RENAME TO transfer_inquiries;
