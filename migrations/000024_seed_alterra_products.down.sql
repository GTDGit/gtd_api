-- Remove Alterra provider SKU mappings
DELETE FROM ppob_provider_skus WHERE provider_id = 2;

-- Remove Alterra-only products (products that have no other provider SKUs)
DELETE FROM products WHERE sku_code LIKE 'ALT-%' AND id NOT IN (
    SELECT DISTINCT product_id FROM ppob_provider_skus
);
