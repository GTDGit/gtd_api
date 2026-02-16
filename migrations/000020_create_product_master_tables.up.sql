-- ============================================
-- Migration 000020: Product Master Tables (categories, brands, types)
-- CRUD managed by admin; products validate against these
-- ============================================

-- 1. Drop the view that depends on products.type before ALTER
DROP VIEW IF EXISTS v_product_best_price;

-- 2. Change products.type from enum to VARCHAR to allow new types (reguler, pulsa_transfer)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'products' AND column_name = 'type' AND udt_name = 'product_type'
    ) THEN
        ALTER TABLE products ALTER COLUMN type TYPE VARCHAR(50) USING type::text;
    END IF;
END $$;

-- 3. Recreate the view (now products.type is VARCHAR)
CREATE OR REPLACE VIEW v_product_best_price AS
SELECT
    p.id AS product_id,
    p.sku_code,
    p.name AS product_name,
    p.category,
    p.brand,
    p.type,
    p.admin AS product_admin,
    MIN(ps.price) AS best_price,
    MIN(ps.admin) FILTER (WHERE ps.price = (
        SELECT MIN(ps2.price)
        FROM ppob_provider_skus ps2
        JOIN ppob_providers pr2 ON ps2.provider_id = pr2.id
        WHERE ps2.product_id = p.id
        AND ps2.is_active = true
        AND ps2.is_available = true
        AND pr2.is_active = true
        AND pr2.is_backup = false
    )) AS best_admin,
    p.is_active AS product_status
FROM products p
LEFT JOIN ppob_provider_skus ps ON p.id = ps.product_id AND ps.is_active = true AND ps.is_available = true
LEFT JOIN ppob_providers pr ON ps.provider_id = pr.id AND pr.is_active = true AND pr.is_backup = false
GROUP BY p.id, p.sku_code, p.name, p.category, p.brand, p.type, p.admin, p.is_active;

-- 4. Product categories (master)
CREATE TABLE IF NOT EXISTS product_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_categories_name ON product_categories(name);

-- 5. Product brands (master)
CREATE TABLE IF NOT EXISTS product_brands (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_brands_name ON product_brands(name);

-- 6. Product types (master)
CREATE TABLE IF NOT EXISTS product_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_types_code ON product_types(code);

-- 7. Seed product_types
INSERT INTO product_types (name, code, display_order) VALUES
    ('Prepaid', 'prepaid', 1),
    ('Postpaid', 'postpaid', 2),
    ('Reguler', 'reguler', 3),
    ('Pulsa Transfer', 'pulsa_transfer', 4)
ON CONFLICT (code) DO NOTHING;

-- 8. Seed product_categories from existing products
INSERT INTO product_categories (name, display_order)
SELECT DISTINCT category, 0 FROM products
WHERE category IS NOT NULL AND category != ''
ON CONFLICT (name) DO NOTHING;

-- 9. Seed product_brands from existing products
INSERT INTO product_brands (name, display_order)
SELECT DISTINCT brand, 0 FROM products
WHERE brand IS NOT NULL AND brand != ''
ON CONFLICT (name) DO NOTHING;
