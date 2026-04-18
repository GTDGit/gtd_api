ALTER TABLE clients
    DROP COLUMN IF EXISTS payment_callback_url,
    DROP COLUMN IF EXISTS payment_callback_secret;
