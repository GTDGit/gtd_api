-- Preserve additional legacy / dummy Alterra UAT product IDs that are still
-- referenced by the vendor workbook but are not present in the live price list.
-- These aliases are testing-only and intentionally use 99-prefixed GTD sku codes.

INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active) VALUES
    ('9900128', 'PDAM UAT Product 128', 'PDAM', 'PDAM', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 128 (closed product scenario)', true),
    ('9900131', 'TV Prepaid UAT Product 131', 'TV Kabel', 'TV Prabayar', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 131 (closed product scenario)', true),
    ('9900205', 'TV Prepaid UAT Product 205', 'TV Kabel', 'TV Prabayar', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 205', true),
    ('9900242', 'Streaming Voucher UAT Product 242', 'Voucher', 'Streaming Voucher', 'prepaid', 0, 0, 'Alterra UAT alias for legacy product_id 242', true),
    ('9900244', 'Gold Voucher UAT Product 244', 'Voucher', 'Gold Voucher', 'prepaid', 0, 0, 'Alterra UAT alias for legacy product_id 244', true),
    ('9900246', 'Education Voucher UAT Product 246', 'Voucher', 'Education Voucher', 'prepaid', 0, 0, 'Alterra UAT alias for legacy product_id 246', true),
    ('9900248', 'Education UAT Product 248', 'Edukasi', 'Education', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 248', true),
    ('9900351', 'PGN UAT Product 351', 'Gas PGN', 'PGN', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 351', true)
ON CONFLICT (sku_code) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    brand = EXCLUDED.brand,
    type = EXCLUDED.type,
    admin = EXCLUDED.admin,
    commission = EXCLUDED.commission,
    description = EXCLUDED.description,
    is_active = true,
    updated_at = NOW();

DELETE FROM ppob_provider_skus
WHERE provider_id = (SELECT id FROM ppob_providers WHERE code = 'alterra')
  AND (
      provider_sku_code IN ('128', '131', '205', '242', '244', '246', '248', '351')
      OR product_id IN (
          SELECT id FROM products WHERE sku_code IN (
              '9900128',
              '9900131',
              '9900205',
              '9900242',
              '9900244',
              '9900246',
              '9900248',
              '9900351'
          )
      )
  );

INSERT INTO ppob_provider_skus (
    provider_id,
    product_id,
    provider_sku_code,
    provider_product_name,
    price,
    admin,
    commission,
    is_active,
    is_available,
    sync_error
)
SELECT
    (SELECT id FROM ppob_providers WHERE code = 'alterra'),
    p.id,
    v.provider_sku_code,
    v.provider_product_name,
    v.price,
    v.admin,
    v.commission,
    true,
    true,
    NULL
FROM (
    VALUES
        ('9900128', '128', 'PDAM UAT Product 128', 0, 0, 0),
        ('9900131', '131', 'TV Prepaid UAT Product 131', 0, 0, 0),
        ('9900205', '205', 'TV Prepaid UAT Product 205', 0, 0, 0),
        ('9900242', '242', 'Streaming Voucher UAT Product 242', 0, 0, 0),
        ('9900244', '244', 'Gold Voucher UAT Product 244', 0, 0, 0),
        ('9900246', '246', 'Education Voucher UAT Product 246', 0, 0, 0),
        ('9900248', '248', 'Education UAT Product 248', 0, 0, 0),
        ('9900351', '351', 'PGN UAT Product 351', 0, 0, 0)
) AS v(sku_code, provider_sku_code, provider_product_name, price, admin, commission)
JOIN products p ON p.sku_code = v.sku_code
ON CONFLICT (provider_id, product_id) DO UPDATE SET
    provider_sku_code = EXCLUDED.provider_sku_code,
    product_id = EXCLUDED.product_id,
    provider_product_name = EXCLUDED.provider_product_name,
    price = EXCLUDED.price,
    admin = EXCLUDED.admin,
    commission = EXCLUDED.commission,
    is_active = true,
    is_available = true,
    sync_error = NULL,
    updated_at = NOW();
