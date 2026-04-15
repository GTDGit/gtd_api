DELETE FROM ppob_provider_skus
WHERE product_id IN (
    SELECT id FROM products WHERE sku_code IN (
        '9900011',
        '9900027',
        '9900112',
        '9900446',
        '9900447',
        '9900686',
        '9900687'
    )
);

DELETE FROM products
WHERE sku_code IN (
    '9900011',
    '9900027',
    '9900112',
    '9900446',
    '9900447',
    '9900686',
    '9900687'
);
