-- ============================================
-- Migration 000020: Product Master Tables (categories, brands, types)
-- CRUD managed by admin; products validate against these
-- ============================================

-- 1. Change products.type from enum to VARCHAR to allow new types (reguler, pulsa_transfer)
ALTER TABLE products ALTER COLUMN type TYPE VARCHAR(50) USING type::text;

-- 2. Product categories (master)
CREATE TABLE product_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_product_categories_name ON product_categories(name);

-- 3. Product brands (master)
CREATE TABLE product_brands (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_product_brands_name ON product_brands(name);

-- 4. Product types (master) - prepaid, postpaid, reguler, pulsa_transfer
CREATE TABLE product_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) NOT NULL UNIQUE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_product_types_code ON product_types(code);

-- 5. Seed product_types
INSERT INTO product_types (name, code, display_order) VALUES
    ('Prepaid', 'prepaid', 1),
    ('Postpaid', 'postpaid', 2),
    ('Reguler', 'reguler', 3),
    ('Pulsa Transfer', 'pulsa_transfer', 4)
ON CONFLICT (code) DO NOTHING;

-- 6. Seed product_categories from existing products
INSERT INTO product_categories (name, display_order)
SELECT DISTINCT category, 0 FROM products
WHERE category IS NOT NULL AND category != ''
ON CONFLICT (name) DO NOTHING;

-- 7. Seed product_brands from existing products
INSERT INTO product_brands (name, display_order)
SELECT DISTINCT brand, 0 FROM products
WHERE brand IS NOT NULL AND brand != ''
ON CONFLICT (name) DO NOTHING;
