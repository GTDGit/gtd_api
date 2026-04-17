DELETE FROM ppob_provider_skus
WHERE provider_id = (SELECT id FROM ppob_providers WHERE code = 'alterra')
  AND provider_sku_code IN ('128', '131', '205', '242', '244', '246', '248', '351');

DELETE FROM products
WHERE sku_code IN (
    '9900128',
    '9900131',
    '9900205',
    '9900242',
    '9900244',
    '9900246',
    '9900248',
    '9900351'
);
