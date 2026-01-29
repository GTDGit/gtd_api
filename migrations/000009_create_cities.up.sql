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
