-- Preserve legacy / dummy Alterra UAT product IDs that are still referenced by
-- the Alterra workbook, even when they are not present in the live price list.
-- These aliases are testing-only and intentionally use 99-prefixed GTD sku codes
-- so runtime and sync logic can distinguish them from the main catalog seed.

INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active) VALUES
    ('9900011', 'Mobile Prepaid UAT Product 11', 'Mobile Prepaid', 'ALTERRA UAT', 'prepaid', 0, 0, 'Alterra UAT alias for legacy product_id 11', true),
    ('9900027', 'PLN Prepaid UAT Product 27', 'Listrik', 'PLN', 'prepaid', 0, 0, 'Alterra UAT alias for legacy product_id 27 (closed product scenario)', true),
    ('9900112', 'PLN Postpaid UAT Product 112', 'Listrik', 'PLN', 'postpaid', 0, 0, 'Alterra UAT alias for legacy product_id 112 (closed product scenario)', true),
    ('9900446', 'BPJS Ketenagakerjaan - Iuran 1 bln (UAT 446)', 'BPJS Ketenagakerjaan', 'BPJS', 'postpaid', 1000, 0, 'Alterra UAT alias for legacy product_id 446', true),
    ('9900447', 'BPJS Ketenagakerjaan - Iuran 3 bln (UAT 447)', 'BPJS Ketenagakerjaan', 'BPJS', 'postpaid', 1000, 0, 'Alterra UAT alias for legacy product_id 447', true),
    ('9900686', 'BPJS Ketenagakerjaan - Iuran 6 bln (UAT 686)', 'BPJS Ketenagakerjaan', 'BPJS', 'postpaid', 1000, 0, 'Alterra UAT alias for legacy product_id 686', true),
    ('9900687', 'BPJS Ketenagakerjaan - Iuran 12 bln (UAT 687)', 'BPJS Ketenagakerjaan', 'BPJS', 'postpaid', 1000, 0, 'Alterra UAT alias for legacy product_id 687', true)
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
        ('9900011', '11', 'Alterra UAT Product 11', 0, 0, 0),
        ('9900027', '27', 'PLN Prepaid UAT Product 27', 0, 0, 0),
        ('9900112', '112', 'PLN Postpaid UAT Product 112', 0, 0, 0),
        ('9900446', '446', 'BPJS Ketenagakerjaan - Iuran 1 bln', 0, 1000, 0),
        ('9900447', '447', 'BPJS Ketenagakerjaan - Iuran 3 bln', 0, 1000, 0),
        ('9900686', '686', 'BPJS Ketenagakerjaan - Iuran 6 bln', 0, 1000, 0),
        ('9900687', '687', 'BPJS Ketenagakerjaan - Iuran 12 bln', 0, 1000, 0)
) AS v(sku_code, provider_sku_code, provider_product_name, price, admin, commission)
JOIN products p ON p.sku_code = v.sku_code
ON CONFLICT (provider_id, provider_sku_code) DO UPDATE SET
    product_id = EXCLUDED.product_id,
    provider_product_name = EXCLUDED.provider_product_name,
    price = EXCLUDED.price,
    admin = EXCLUDED.admin,
    commission = EXCLUDED.commission,
    is_active = true,
    is_available = true,
    sync_error = NULL,
    updated_at = NOW();
