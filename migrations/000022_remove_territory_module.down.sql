CREATE TABLE provinces (
    code VARCHAR(2) PRIMARY KEY,
    name VARCHAR(100) NOT NULL
);

CREATE TABLE cities (
    code VARCHAR(2) NOT NULL,
    province_code VARCHAR(2) NOT NULL,
    full_code VARCHAR(4) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    CONSTRAINT fk_cities_province
        FOREIGN KEY (province_code)
        REFERENCES provinces(code)
);

CREATE INDEX idx_cities_province
ON cities(province_code);

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
