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
