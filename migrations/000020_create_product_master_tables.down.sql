DROP TABLE IF EXISTS product_types;
DROP TABLE IF EXISTS product_brands;
DROP TABLE IF EXISTS product_categories;

-- Note: products.type remains VARCHAR(50). To restore enum, first update
-- any 'reguler'/'pulsa_transfer' to 'prepaid'/'postpaid', then:
-- ALTER TABLE products ALTER COLUMN type TYPE product_type USING type::product_type;
