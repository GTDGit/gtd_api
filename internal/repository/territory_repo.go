package repository

import (
	"context"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/jmoiron/sqlx"
)

// TerritoryRepository handles database operations for territory data
type TerritoryRepository struct {
	db *sqlx.DB
}

// NewTerritoryRepository creates a new TerritoryRepository
func NewTerritoryRepository(db *sqlx.DB) *TerritoryRepository {
	return &TerritoryRepository{db: db}
}

// GetAllProvinces returns all provinces
func (r *TerritoryRepository) GetAllProvinces(ctx context.Context) ([]models.Province, error) {
	query := `SELECT code, name FROM provinces ORDER BY code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var provinces []models.Province
	for rows.Next() {
		var p models.Province
		if err := rows.Scan(&p.Code, &p.Name); err != nil {
			return nil, err
		}
		provinces = append(provinces, p)
	}
	return provinces, rows.Err()
}

// GetProvinceByCode returns a province by its code
func (r *TerritoryRepository) GetProvinceByCode(ctx context.Context, code string) (*models.Province, error) {
	query := `SELECT code, name FROM provinces WHERE code = $1`

	var p models.Province
	err := r.db.QueryRowContext(ctx, query, code).Scan(&p.Code, &p.Name)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetAllCities returns all cities in Indonesia
func (r *TerritoryRepository) GetAllCities(ctx context.Context) ([]models.City, error) {
	query := `SELECT code, province_code, full_code, name
	          FROM cities ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cities []models.City
	for rows.Next() {
		var c models.City
		if err := rows.Scan(&c.Code, &c.ProvinceCode, &c.FullCode, &c.Name); err != nil {
			return nil, err
		}
		cities = append(cities, c)
	}
	return cities, rows.Err()
}

// GetCitiesByProvinceCode returns all cities for a given province code
func (r *TerritoryRepository) GetCitiesByProvinceCode(ctx context.Context, provinceCode string) ([]models.City, error) {
	query := `SELECT code, province_code, full_code, name
	          FROM cities WHERE province_code = $1 ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query, provinceCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cities []models.City
	for rows.Next() {
		var c models.City
		if err := rows.Scan(&c.Code, &c.ProvinceCode, &c.FullCode, &c.Name); err != nil {
			return nil, err
		}
		cities = append(cities, c)
	}
	return cities, rows.Err()
}

// GetCityByCode returns a city by its full code
func (r *TerritoryRepository) GetCityByCode(ctx context.Context, code string) (*models.City, error) {
	query := `SELECT code, province_code, full_code, name FROM cities WHERE full_code = $1`

	var c models.City
	err := r.db.QueryRowContext(ctx, query, code).Scan(&c.Code, &c.ProvinceCode, &c.FullCode, &c.Name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetAllDistricts returns all districts in Indonesia
func (r *TerritoryRepository) GetAllDistricts(ctx context.Context) ([]models.District, error) {
	query := `SELECT code, city_code, full_code, name
	          FROM districts ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var districts []models.District
	for rows.Next() {
		var d models.District
		if err := rows.Scan(&d.Code, &d.CityCode, &d.FullCode, &d.Name); err != nil {
			return nil, err
		}
		districts = append(districts, d)
	}
	return districts, rows.Err()
}

// GetDistrictsByCityCode returns all districts for a given city code
func (r *TerritoryRepository) GetDistrictsByCityCode(ctx context.Context, cityCode string) ([]models.District, error) {
	query := `SELECT code, city_code, full_code, name
	          FROM districts WHERE city_code = $1 ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query, cityCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var districts []models.District
	for rows.Next() {
		var d models.District
		if err := rows.Scan(&d.Code, &d.CityCode, &d.FullCode, &d.Name); err != nil {
			return nil, err
		}
		districts = append(districts, d)
	}
	return districts, rows.Err()
}

// GetDistrictByCode returns a district by its full code
func (r *TerritoryRepository) GetDistrictByCode(ctx context.Context, code string) (*models.District, error) {
	query := `SELECT code, city_code, full_code, name FROM districts WHERE full_code = $1`

	var d models.District
	err := r.db.QueryRowContext(ctx, query, code).Scan(&d.Code, &d.CityCode, &d.FullCode, &d.Name)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetAllSubDistricts returns all sub-districts in Indonesia
func (r *TerritoryRepository) GetAllSubDistricts(ctx context.Context) ([]models.SubDistrict, error) {
	query := `SELECT code, district_code, full_code, name
	          FROM sub_districts ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subDistricts []models.SubDistrict
	for rows.Next() {
		var s models.SubDistrict
		if err := rows.Scan(&s.Code, &s.DistrictCode, &s.FullCode, &s.Name); err != nil {
			return nil, err
		}
		subDistricts = append(subDistricts, s)
	}
	return subDistricts, rows.Err()
}

// GetSubDistrictsByDistrictCode returns all sub-districts for a given district code
func (r *TerritoryRepository) GetSubDistrictsByDistrictCode(ctx context.Context, districtCode string) ([]models.SubDistrict, error) {
	query := `SELECT code, district_code, full_code, name
	          FROM sub_districts WHERE district_code = $1 ORDER BY full_code`

	rows, err := r.db.QueryContext(ctx, query, districtCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subDistricts []models.SubDistrict
	for rows.Next() {
		var s models.SubDistrict
		if err := rows.Scan(&s.Code, &s.DistrictCode, &s.FullCode, &s.Name); err != nil {
			return nil, err
		}
		subDistricts = append(subDistricts, s)
	}
	return subDistricts, rows.Err()
}

// GetSubDistrictByCode returns a sub-district by its full code
func (r *TerritoryRepository) GetSubDistrictByCode(ctx context.Context, code string) (*models.SubDistrict, error) {
	query := `SELECT code, district_code, full_code, name FROM sub_districts WHERE full_code = $1`

	var s models.SubDistrict
	err := r.db.QueryRowContext(ctx, query, code).Scan(&s.Code, &s.DistrictCode, &s.FullCode, &s.Name)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CountProvinces returns the total count of provinces
func (r *TerritoryRepository) CountProvinces(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM provinces`).Scan(&count)
	return count, err
}

// CountCities returns the total count of all cities
func (r *TerritoryRepository) CountCities(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cities`).Scan(&count)
	return count, err
}

// CountCitiesByProvince returns the total count of cities in a province
func (r *TerritoryRepository) CountCitiesByProvince(ctx context.Context, provinceCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cities WHERE province_code = $1`, provinceCode).Scan(&count)
	return count, err
}

// CountDistricts returns the total count of all districts
func (r *TerritoryRepository) CountDistricts(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM districts`).Scan(&count)
	return count, err
}

// CountDistrictsByCity returns the total count of districts in a city
func (r *TerritoryRepository) CountDistrictsByCity(ctx context.Context, cityCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM districts WHERE city_code = $1`, cityCode).Scan(&count)
	return count, err
}

// CountSubDistricts returns the total count of all sub-districts
func (r *TerritoryRepository) CountSubDistricts(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sub_districts`).Scan(&count)
	return count, err
}

// CountSubDistrictsByDistrict returns the total count of sub-districts in a district
func (r *TerritoryRepository) CountSubDistrictsByDistrict(ctx context.Context, districtCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sub_districts WHERE district_code = $1`, districtCode).Scan(&count)
	return count, err
}

// CountPostalCodes returns the total count of all postal codes
func (r *TerritoryRepository) CountPostalCodes(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postal_codes`).Scan(&count)
	return count, err
}

// CountPostalCodesByDistrict returns the total count of postal codes in a district
func (r *TerritoryRepository) CountPostalCodesByDistrict(ctx context.Context, districtCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postal_codes WHERE SUBSTRING(sub_district_code, 1, 6) = $1`, districtCode).Scan(&count)
	return count, err
}

// CountPostalCodesBySubDistrict returns the total count of postal codes in a sub-district
func (r *TerritoryRepository) CountPostalCodesBySubDistrict(ctx context.Context, subDistrictCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postal_codes WHERE sub_district_code = $1`, subDistrictCode).Scan(&count)
	return count, err
}

// GetProvinceByName returns a province by name (case-insensitive, fuzzy match)
func (r *TerritoryRepository) GetProvinceByName(ctx context.Context, name string) (*models.Province, error) {
	query := `SELECT code, name FROM provinces WHERE UPPER(name) LIKE '%' || $1 || '%' ORDER BY 
		CASE WHEN UPPER(name) = $1 THEN 0 ELSE 1 END, 
		LENGTH(name) LIMIT 1`

	var p models.Province
	err := r.db.QueryRowContext(ctx, query, name).Scan(&p.Code, &p.Name)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetCityByNameAndProvince returns a city by name within a specific province
func (r *TerritoryRepository) GetCityByNameAndProvince(ctx context.Context, name, provinceCode string) (*models.City, error) {
	query := `SELECT code, province_code, full_code, name 
		FROM cities 
		WHERE province_code = $1 AND UPPER(name) LIKE '%' || $2 || '%' 
		ORDER BY 
			CASE WHEN UPPER(name) = $2 THEN 0 ELSE 1 END,
			LENGTH(name) 
		LIMIT 1`

	var c models.City
	err := r.db.QueryRowContext(ctx, query, provinceCode, name).Scan(&c.Code, &c.ProvinceCode, &c.FullCode, &c.Name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetDistrictByNameAndCity returns a district by name within a specific city
func (r *TerritoryRepository) GetDistrictByNameAndCity(ctx context.Context, name, cityCode string) (*models.District, error) {
	// Query with fuzzy match that also handles concatenated names (e.g., BEKASIUTARA matches BEKASI UTARA)
	query := `SELECT code, city_code, full_code, name 
		FROM districts 
		WHERE city_code = $1 AND (
			UPPER(name) LIKE '%' || $2 || '%' 
			OR UPPER(REPLACE(name, ' ', '')) LIKE '%' || $2 || '%'
			OR UPPER(name) LIKE '%' || REPLACE($2, ' ', '') || '%'
			OR UPPER(REPLACE(name, ' ', '')) = UPPER(REPLACE($2, ' ', ''))
		)
		ORDER BY 
			CASE 
				WHEN UPPER(name) = $2 THEN 0 
				WHEN UPPER(REPLACE(name, ' ', '')) = UPPER(REPLACE($2, ' ', '')) THEN 1
				ELSE 2 
			END,
			LENGTH(name) 
		LIMIT 1`

	var d models.District
	err := r.db.QueryRowContext(ctx, query, cityCode, name).Scan(&d.Code, &d.CityCode, &d.FullCode, &d.Name)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// GetSubDistrictByNameAndDistrict returns a sub-district by name within a specific district
func (r *TerritoryRepository) GetSubDistrictByNameAndDistrict(ctx context.Context, name, districtCode string) (*models.SubDistrict, error) {
	// Query with fuzzy match that also handles concatenated names
	query := `SELECT code, district_code, full_code, name
		FROM sub_districts
		WHERE district_code = $1 AND (
			UPPER(name) LIKE '%' || $2 || '%'
			OR UPPER(REPLACE(name, ' ', '')) LIKE '%' || $2 || '%'
			OR UPPER(name) LIKE '%' || REPLACE($2, ' ', '') || '%'
			OR UPPER(REPLACE(name, ' ', '')) = UPPER(REPLACE($2, ' ', ''))
		)
		ORDER BY
			CASE
				WHEN UPPER(name) = $2 THEN 0
				WHEN UPPER(REPLACE(name, ' ', '')) = UPPER(REPLACE($2, ' ', '')) THEN 1
				ELSE 2
			END,
			LENGTH(name)
		LIMIT 1`

	var s models.SubDistrict
	err := r.db.QueryRowContext(ctx, query, districtCode, name).Scan(&s.Code, &s.DistrictCode, &s.FullCode, &s.Name)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAllPostalCodes returns all postal codes
func (r *TerritoryRepository) GetAllPostalCodes(ctx context.Context) ([]models.PostalCode, error) {
	query := `SELECT
		pc.sub_district_code,
		pc.postal_code,
		SUBSTRING(pc.sub_district_code, 1, 6) as district_code
	FROM postal_codes pc
	ORDER BY pc.postal_code, pc.sub_district_code`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postalCodes []models.PostalCode
	for rows.Next() {
		var pc models.PostalCode
		if err := rows.Scan(&pc.SubDistrictCode, &pc.PostalCode, &pc.DistrictCode); err != nil {
			return nil, err
		}
		postalCodes = append(postalCodes, pc)
	}
	return postalCodes, rows.Err()
}

// GetPostalCodesByDistrict returns all postal codes for a given district code
func (r *TerritoryRepository) GetPostalCodesByDistrict(ctx context.Context, districtCode string) ([]models.PostalCode, error) {
	query := `SELECT
		pc.sub_district_code,
		pc.postal_code,
		SUBSTRING(pc.sub_district_code, 1, 6) as district_code
	FROM postal_codes pc
	WHERE SUBSTRING(pc.sub_district_code, 1, 6) = $1
	ORDER BY pc.postal_code, pc.sub_district_code`

	rows, err := r.db.QueryContext(ctx, query, districtCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postalCodes []models.PostalCode
	for rows.Next() {
		var pc models.PostalCode
		if err := rows.Scan(&pc.SubDistrictCode, &pc.PostalCode, &pc.DistrictCode); err != nil {
			return nil, err
		}
		postalCodes = append(postalCodes, pc)
	}
	return postalCodes, rows.Err()
}

// GetPostalCodesBySubDistrict returns all postal codes for a given sub-district code
func (r *TerritoryRepository) GetPostalCodesBySubDistrict(ctx context.Context, subDistrictCode string) ([]models.PostalCode, error) {
	query := `SELECT
		pc.sub_district_code,
		pc.postal_code,
		SUBSTRING(pc.sub_district_code, 1, 6) as district_code
	FROM postal_codes pc
	WHERE pc.sub_district_code = $1
	ORDER BY pc.postal_code`

	rows, err := r.db.QueryContext(ctx, query, subDistrictCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var postalCodes []models.PostalCode
	for rows.Next() {
		var pc models.PostalCode
		if err := rows.Scan(&pc.SubDistrictCode, &pc.PostalCode, &pc.DistrictCode); err != nil {
			return nil, err
		}
		postalCodes = append(postalCodes, pc)
	}
	return postalCodes, rows.Err()
}
