DELETE FROM transaction_logs
WHERE sku_id IS NULL;

ALTER TABLE transaction_logs
    ALTER COLUMN sku_id SET NOT NULL;
