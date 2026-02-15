ALTER TABLE transactions
    DROP COLUMN IF EXISTS buy_price,
    DROP COLUMN IF EXISTS sell_price;
