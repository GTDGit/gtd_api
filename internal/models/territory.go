package models

// Province represents a province in Indonesia
type Province struct {
	Code string `json:"code" db:"code"`
	Name string `json:"name" db:"name"`
}

// City represents a city/regency in Indonesia
type City struct {
	Code         string `json:"code" db:"code"`
	ProvinceCode string `json:"provinceCode" db:"province_code"`
	FullCode     string `json:"fullCode" db:"full_code"`
	Name         string `json:"name" db:"name"`
}

// District represents a district in Indonesia
type District struct {
	Code     string `json:"code" db:"code"`
	CityCode string `json:"cityCode" db:"city_code"`
	FullCode string `json:"fullCode" db:"full_code"`
	Name     string `json:"name" db:"name"`
}

// SubDistrict represents a sub-district (kelurahan/desa) in Indonesia
type SubDistrict struct {
	Code         string `json:"code" db:"code"`
	DistrictCode string `json:"districtCode" db:"district_code"`
	FullCode     string `json:"fullCode" db:"full_code"`
	Name         string `json:"name" db:"name"`
}

// PostalCode represents a postal code in Indonesia
type PostalCode struct {
	SubDistrictCode string `json:"subDistrictCode" db:"sub_district_code"`
	PostalCode      string `json:"postalCode" db:"postal_code"`
	DistrictCode    string `json:"districtCode" db:"district_code"`
}

// ProvinceResponse represents the API response for a province
type ProvinceResponse struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// CityResponse represents the API response for a city
type CityResponse struct {
	Name         string `json:"name"`
	ProvinceCode string `json:"provinceCode"`
	Code         string `json:"code"`
}

// DistrictResponse represents the API response for a district
type DistrictResponse struct {
	Name     string `json:"name"`
	CityCode string `json:"cityCode"`
	Code     string `json:"code"`
}

// SubDistrictResponse represents the API response for a sub-district
type SubDistrictResponse struct {
	Name         string `json:"name"`
	DistrictCode string `json:"districtCode"`
	Code         string `json:"code"`
}

// PostalCodeResponse represents the API response for a postal code
type PostalCodeResponse struct {
	PostalCode      string `json:"postalCode"`
	SubDistrictCode string `json:"subDistrictCode"`
	DistrictCode    string `json:"districtCode"`
}
