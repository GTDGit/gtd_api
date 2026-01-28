package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/GTDGit/gtd_api/internal/models"
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

// CountCitiesByProvince returns the total count of cities in a province
func (r *TerritoryRepository) CountCitiesByProvince(ctx context.Context, provinceCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cities WHERE province_code = $1`, provinceCode).Scan(&count)
	return count, err
}

// CountDistrictsByCity returns the total count of districts in a city
func (r *TerritoryRepository) CountDistrictsByCity(ctx context.Context, cityCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM districts WHERE city_code = $1`, cityCode).Scan(&count)
	return count, err
}

// CountSubDistrictsByDistrict returns the total count of sub-districts in a district
func (r *TerritoryRepository) CountSubDistrictsByDistrict(ctx context.Context, districtCode string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sub_districts WHERE district_code = $1`, districtCode).Scan(&count)
	return count, err
}
