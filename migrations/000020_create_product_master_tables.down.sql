-- Remove variant_id from products
ALTER TABLE products DROP COLUMN IF EXISTS variant_id;

DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS product_brands;
DROP TABLE IF EXISTS product_categories;
