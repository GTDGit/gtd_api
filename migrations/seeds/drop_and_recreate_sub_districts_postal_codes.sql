-- Drop (postal_codes dulu karena FK ke sub_districts)
DROP TABLE IF EXISTS postal_codes;
DROP TABLE IF EXISTS sub_districts;
DROP TABLE IF EXISTS districts;

-- Recreate districts (000010)
CREATE TABLE districts (
    code VARCHAR(2) NOT NULL,
    city_code VARCHAR(4) NOT NULL,
    full_code VARCHAR(6) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    CONSTRAINT fk_districts_city
        FOREIGN KEY (city_code)
        REFERENCES cities(full_code)
);

CREATE INDEX idx_districts_city
ON districts(city_code);


-- Recreate sub_districts (000011)
CREATE TABLE sub_districts (
    code VARCHAR(4) NOT NULL,
    district_code VARCHAR(6) NOT NULL,
    full_code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    CONSTRAINT fk_sub_districts_district
        FOREIGN KEY (district_code)
        REFERENCES districts(full_code)
);

CREATE INDEX idx_sub_districts_district
ON sub_districts(district_code);

-- Recreate postal_codes (000018)
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