-- ============================================
-- Migration 000018: Postal Codes Table
-- ============================================
-- Kode pos (5 digit) dikaitkan dengan kelurahan/desa (sub_district).
-- Query by district: join sub_districts dan filter district_code.
-- Query by sub_district: filter sub_district_code.

CREATE TABLE postal_codes (
    sub_district_code VARCHAR(10) NOT NULL,
    postal_code VARCHAR(5) NOT NULL,
    PRIMARY KEY (sub_district_code, postal_code),
    CONSTRAINT fk_postal_codes_sub_district
        FOREIGN KEY (sub_district_code)
        REFERENCES sub_districts(full_code)
);

CREATE INDEX idx_postal_codes_sub_district
ON postal_codes(sub_district_code);

CREATE INDEX idx_postal_codes_postal_code
ON postal_codes(postal_code);
