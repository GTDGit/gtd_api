ALTER TABLE transactions
    ADD COLUMN provider_initial_response JSONB,
    ADD COLUMN provider_initial_http_status INT,
    ADD COLUMN provider_http_status INT;
