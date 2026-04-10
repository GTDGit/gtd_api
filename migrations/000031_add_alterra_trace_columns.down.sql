ALTER TABLE transactions
    DROP COLUMN IF EXISTS provider_http_status,
    DROP COLUMN IF EXISTS provider_initial_http_status,
    DROP COLUMN IF EXISTS provider_initial_response;
