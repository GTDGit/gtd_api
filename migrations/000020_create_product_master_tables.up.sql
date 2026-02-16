-- ============================================
-- Migration 000020: Product Master (categories, brands) + Product Variants
-- products.type TETAP enum product_type (prepaid/postpaid)
-- product_variants = Reguler, Pulsa Transfer, dll (BUKAN prepaid/postpaid)
-- ============================================

-- 1. Drop product_types if exists (from previous attempt)
DROP TABLE IF EXISTS product_types;

-- 2. Product categories (master)
CREATE TABLE IF NOT EXISTS product_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_categories_name ON product_categories(name);

-- 3. Product brands (master)
CREATE TABLE IF NOT EXISTS product_brands (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_brands_name ON product_brands(name);

-- 4. Product variants (Reguler, Pulsa Transfer, dll - BUKAN prepaid/postpaid)
CREATE TABLE IF NOT EXISTS product_variants (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_variants_name ON product_variants(name);

-- 5. Seed product_variants (Reguler, Pulsa Transfer only)
INSERT INTO product_variants (name, display_order) VALUES
    ('Reguler', 1),
    ('Pulsa Transfer', 2)
ON CONFLICT (name) DO NOTHING;

-- 6. Add variant_id to products (optional)
ALTER TABLE products ADD COLUMN IF NOT EXISTS variant_id INT REFERENCES product_variants(id);

-- 7. Seed product_categories from existing products
INSERT INTO product_categories (name, display_order)
SELECT DISTINCT category, 0 FROM products
WHERE category IS NOT NULL AND category != ''
ON CONFLICT (name) DO NOTHING;

-- 8. Seed product_brands from existing products
INSERT INTO product_brands (name, display_order)
SELECT DISTINCT brand, 0 FROM products
WHERE brand IS NOT NULL AND brand != ''
ON CONFLICT (name) DO NOTHING;
