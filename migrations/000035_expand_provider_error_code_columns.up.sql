ALTER TABLE transactions
    ALTER COLUMN failed_code TYPE VARCHAR(64);

ALTER TABLE transaction_logs
    ALTER COLUMN rc TYPE VARCHAR(64);

ALTER TABLE digiflazz_callbacks
    ALTER COLUMN rc TYPE VARCHAR(64);
