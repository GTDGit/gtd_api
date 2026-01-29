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
