-- ============================================
-- Migration 000019: Add buy_price and sell_price to transactions
-- buy_price  = actual price paid to provider (from successful provider)
-- sell_price = price shown to client (cheapest provider price at time of transaction)
-- ============================================

ALTER TABLE transactions
    ADD COLUMN buy_price INT,
    ADD COLUMN sell_price INT;

COMMENT ON COLUMN transactions.buy_price IS 'Actual price paid to provider (may differ from sell_price on failover)';
COMMENT ON COLUMN transactions.sell_price IS 'Price shown to client (cheapest provider price at transaction time)';
