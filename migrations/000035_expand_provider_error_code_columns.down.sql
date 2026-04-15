ALTER TABLE digiflazz_callbacks
    ALTER COLUMN rc TYPE VARCHAR(10)
    USING LEFT(rc, 10);

ALTER TABLE transaction_logs
    ALTER COLUMN rc TYPE VARCHAR(10)
    USING LEFT(rc, 10);

ALTER TABLE transactions
    ALTER COLUMN failed_code TYPE VARCHAR(10)
    USING LEFT(failed_code, 10);
