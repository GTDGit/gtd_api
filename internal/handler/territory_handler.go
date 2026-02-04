package handler

import (
	"database/sql"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// TerritoryHandler handles territory-related HTTP requests
type TerritoryHandler struct {
	repo *repository.TerritoryRepository
}

// NewTerritoryHandler creates a new TerritoryHandler
func NewTerritoryHandler(repo *repository.TerritoryRepository) *TerritoryHandler {
	return &TerritoryHandler{repo: repo}
}

// TerritoryResponse is the standard response structure for territory endpoints
type TerritoryResponse struct {
	Success bool                `json:"success"`
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    interface{}         `json:"data,omitempty"`
	Error   *TerritoryErrorInfo `json:"error,omitempty"`
	Meta    TerritoryMeta       `json:"meta"`
}

// TerritoryErrorInfo contains error details
type TerritoryErrorInfo struct {
	Type    string `json:"type"`
	Details string `json:"details"`
}

// TerritoryMeta contains metadata for the response
type TerritoryMeta struct {
	Total        int    `json:"total,omitempty"`
	ProvinceCode string `json:"provinceCode,omitempty"`
	ProvinceName string `json:"provinceName,omitempty"`
	CityCode     string `json:"cityCode,omitempty"`
	CityName     string `json:"cityName,omitempty"`
	DistrictCode string `json:"districtCode,omitempty"`
	DistrictName string `json:"districtName,omitempty"`
	RequestID    string `json:"requestId"`
	Timestamp    string `json:"timestamp"`
}

// GetProvinces returns all provinces
// GET /v1/territory/province
func (h *TerritoryHandler) GetProvinces(c *gin.Context) {
	ctx := c.Request.Context()

	provinces, err := h.repo.GetAllProvinces(ctx)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve provinces")
		return
	}

	// Convert to response format
	var response []models.ProvinceResponse
	for _, p := range provinces {
		response = append(response, models.ProvinceResponse{
			Name: p.Name,
			Code: p.Code,
		})
	}

	count, _ := h.repo.CountProvinces(ctx)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved provinces",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetAllCities returns all cities in Indonesia
// GET /v1/territory/city
func (h *TerritoryHandler) GetAllCities(c *gin.Context) {
	ctx := c.Request.Context()

	cities, err := h.repo.GetAllCities(ctx)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve cities")
		return
	}

	// Convert to response format
	var response []models.CityResponse
	for _, city := range cities {
		response = append(response, models.CityResponse{
			Name:         city.Name,
			ProvinceCode: city.ProvinceCode,
			Code:         city.FullCode,
		})
	}

	count, _ := h.repo.CountCities(ctx)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved cities",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetCitiesByProvince returns all cities for a given province
// GET /v1/territory/city/:province_code
func (h *TerritoryHandler) GetCitiesByProvince(c *gin.Context) {
	ctx := c.Request.Context()
	provinceCode := c.Param("province_code")

	// Validate province code format (2 digits)
	if !h.isValidProvinceCode(provinceCode) {
		h.errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Province code must be 2 digits")
		return
	}

	// Check if province exists
	province, err := h.repo.GetProvinceByCode(ctx, provinceCode)
	if err != nil {
		if err == sql.ErrNoRows {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Province with code '"+provinceCode+"' does not exist")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve province")
		return
	}

	cities, err := h.repo.GetCitiesByProvinceCode(ctx, provinceCode)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve cities")
		return
	}

	// Convert to response format
	var response []models.CityResponse
	for _, city := range cities {
		response = append(response, models.CityResponse{
			Name:         city.Name,
			ProvinceCode: city.ProvinceCode,
			Code:         city.FullCode,
		})
	}

	count, _ := h.repo.CountCitiesByProvince(ctx, provinceCode)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved cities",
		Data:    response,
		Meta: TerritoryMeta{
			Total:        count,
			ProvinceCode: provinceCode,
			ProvinceName: province.Name,
			RequestID:    h.generateRequestID(),
			Timestamp:    time.Now().Format(time.RFC3339),
		},
	})
}

// GetAllDistricts returns all districts in Indonesia
// GET /v1/territory/district
func (h *TerritoryHandler) GetAllDistricts(c *gin.Context) {
	ctx := c.Request.Context()

	districts, err := h.repo.GetAllDistricts(ctx)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve districts")
		return
	}

	// Convert to response format
	var response []models.DistrictResponse
	for _, district := range districts {
		response = append(response, models.DistrictResponse{
			Name:     district.Name,
			CityCode: district.CityCode,
			Code:     district.FullCode,
		})
	}

	count, _ := h.repo.CountDistricts(ctx)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved districts",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetDistrictsByCity returns all districts for a given city
// GET /v1/territory/district/:city_code
func (h *TerritoryHandler) GetDistrictsByCity(c *gin.Context) {
	ctx := c.Request.Context()
	cityCode := c.Param("city_code")

	// Validate city code format (4 digits)
	if !h.isValidCityCode(cityCode) {
		h.errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "City code must be 4 digits")
		return
	}

	// Check if city exists
	city, err := h.repo.GetCityByCode(ctx, cityCode)
	if err != nil {
		if err == sql.ErrNoRows {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "City with code '"+cityCode+"' does not exist")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve city")
		return
	}

	districts, err := h.repo.GetDistrictsByCityCode(ctx, cityCode)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve districts")
		return
	}

	// Convert to response format
	var response []models.DistrictResponse
	for _, district := range districts {
		response = append(response, models.DistrictResponse{
			Name:     district.Name,
			CityCode: district.CityCode,
			Code:     district.FullCode,
		})
	}

	count, _ := h.repo.CountDistrictsByCity(ctx, cityCode)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved districts",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			CityCode:  cityCode,
			CityName:  city.Name,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetAllSubDistricts returns all sub-districts in Indonesia
// GET /v1/territory/sub-district
func (h *TerritoryHandler) GetAllSubDistricts(c *gin.Context) {
	ctx := c.Request.Context()

	subDistricts, err := h.repo.GetAllSubDistricts(ctx)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve sub-districts")
		return
	}

	// Convert to response format
	var response []models.SubDistrictResponse
	for _, subDistrict := range subDistricts {
		response = append(response, models.SubDistrictResponse{
			Name:         subDistrict.Name,
			DistrictCode: subDistrict.DistrictCode,
			Code:         subDistrict.FullCode,
		})
	}

	count, _ := h.repo.CountSubDistricts(ctx)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved sub-districts",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetSubDistrictsByDistrict returns all sub-districts for a given district
// GET /v1/territory/sub-district/:district_code
func (h *TerritoryHandler) GetSubDistrictsByDistrict(c *gin.Context) {
	ctx := c.Request.Context()
	districtCode := c.Param("district_code")

	// Validate district code format (6 digits)
	if !h.isValidDistrictCode(districtCode) {
		h.errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "District code must be 6 digits")
		return
	}

	// Check if district exists
	district, err := h.repo.GetDistrictByCode(ctx, districtCode)
	if err != nil {
		if err == sql.ErrNoRows {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "District with code '"+districtCode+"' does not exist")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve district")
		return
	}

	subDistricts, err := h.repo.GetSubDistrictsByDistrictCode(ctx, districtCode)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve sub-districts")
		return
	}

	// Convert to response format
	var response []models.SubDistrictResponse
	for _, subDistrict := range subDistricts {
		response = append(response, models.SubDistrictResponse{
			Name:         subDistrict.Name,
			DistrictCode: subDistrict.DistrictCode,
			Code:         subDistrict.FullCode,
		})
	}

	count, _ := h.repo.CountSubDistrictsByDistrict(ctx, districtCode)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved sub-districts",
		Data:    response,
		Meta: TerritoryMeta{
			Total:        count,
			DistrictCode: districtCode,
			DistrictName: district.Name,
			RequestID:    h.generateRequestID(),
			Timestamp:    time.Now().Format(time.RFC3339),
		},
	})
}

// GetAllPostalCodes returns all postal codes in Indonesia
// GET /v1/territory/postal-code
func (h *TerritoryHandler) GetAllPostalCodes(c *gin.Context) {
	ctx := c.Request.Context()

	postalCodes, err := h.repo.GetAllPostalCodes(ctx)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve postal codes")
		return
	}

	// Convert to response format
	var response []models.PostalCodeResponse
	for _, pc := range postalCodes {
		response = append(response, models.PostalCodeResponse{
			PostalCode:      pc.PostalCode,
			SubDistrictCode: pc.SubDistrictCode,
			DistrictCode:    pc.DistrictCode,
		})
	}

	count, _ := h.repo.CountPostalCodes(ctx)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved postal codes",
		Data:    response,
		Meta: TerritoryMeta{
			Total:     count,
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}

// GetPostalCodesByDistrict returns all postal codes for a given district
// GET /v1/territory/postal-code/district/:district_code
func (h *TerritoryHandler) GetPostalCodesByDistrict(c *gin.Context) {
	ctx := c.Request.Context()
	districtCode := c.Param("district_code")

	// Validate district code format (6 digits)
	if !h.isValidDistrictCode(districtCode) {
		h.errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "District code must be 6 digits")
		return
	}

	// Check if district exists
	district, err := h.repo.GetDistrictByCode(ctx, districtCode)
	if err != nil {
		if err == sql.ErrNoRows {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "District with code '"+districtCode+"' does not exist")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve district")
		return
	}

	postalCodes, err := h.repo.GetPostalCodesByDistrict(ctx, districtCode)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve postal codes")
		return
	}

	if len(postalCodes) == 0 {
		h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "No postal code found for district code '"+districtCode+"'")
		return
	}

	// Convert to response format
	var response []models.PostalCodeResponse
	for _, pc := range postalCodes {
		response = append(response, models.PostalCodeResponse{
			PostalCode:      pc.PostalCode,
			SubDistrictCode: pc.SubDistrictCode,
			DistrictCode:    pc.DistrictCode,
		})
	}

	count, _ := h.repo.CountPostalCodesByDistrict(ctx, districtCode)

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved postal codes",
		Data:    response,
		Meta: TerritoryMeta{
			Total:        count,
			DistrictCode: districtCode,
			DistrictName: district.Name,
			RequestID:    h.generateRequestID(),
			Timestamp:    time.Now().Format(time.RFC3339),
		},
	})
}

// GetPostalCodesBySubDistrict returns all postal codes for a given sub-district
// GET /v1/territory/postal-code/sub-district/:sub_district_code
func (h *TerritoryHandler) GetPostalCodesBySubDistrict(c *gin.Context) {
	ctx := c.Request.Context()
	subDistrictCode := c.Param("sub_district_code")

	// Validate sub-district code format (10 digits)
	if !h.isValidSubDistrictCode(subDistrictCode) {
		h.errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Sub-district code must be 10 digits")
		return
	}

	// Check if sub-district exists
	_, err := h.repo.GetSubDistrictByCode(ctx, subDistrictCode)
	if err != nil {
		if err == sql.ErrNoRows {
			h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Sub-district with code '"+subDistrictCode+"' does not exist")
			return
		}
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve sub-district")
		return
	}

	postalCodes, err := h.repo.GetPostalCodesBySubDistrict(ctx, subDistrictCode)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve postal codes")
		return
	}

	if len(postalCodes) == 0 {
		h.errorResponse(c, http.StatusNotFound, "NOT_FOUND", "No postal code found for sub-district code '"+subDistrictCode+"'")
		return
	}

	// Convert to response format
	var response []models.PostalCodeResponse
	for _, pc := range postalCodes {
		response = append(response, models.PostalCodeResponse{
			PostalCode:      pc.PostalCode,
			SubDistrictCode: pc.SubDistrictCode,
			DistrictCode:    pc.DistrictCode,
		})
	}

	count, _ := h.repo.CountPostalCodesBySubDistrict(ctx, subDistrictCode)

	// Get district code from sub-district code (first 6 digits)
	districtCode := subDistrictCode[:6]

	c.JSON(http.StatusOK, TerritoryResponse{
		Success: true,
		Code:    http.StatusOK,
		Message: "Successfully retrieved postal codes",
		Data:    response,
		Meta: TerritoryMeta{
			Total:        count,
			DistrictCode: districtCode,
			RequestID:    h.generateRequestID(),
			Timestamp:    time.Now().Format(time.RFC3339),
		},
	})
}

// Helper functions

func (h *TerritoryHandler) isValidProvinceCode(code string) bool {
	match, _ := regexp.MatchString(`^\d{2}$`, code)
	return match
}

func (h *TerritoryHandler) isValidCityCode(code string) bool {
	match, _ := regexp.MatchString(`^\d{4}$`, code)
	return match
}

func (h *TerritoryHandler) isValidDistrictCode(code string) bool {
	match, _ := regexp.MatchString(`^\d{6}$`, code)
	return match
}

func (h *TerritoryHandler) isValidSubDistrictCode(code string) bool {
	match, _ := regexp.MatchString(`^\d{10}$`, code)
	return match
}

func (h *TerritoryHandler) generateRequestID() string {
	return "req_ter_" + uuid.New().String()[:8]
}

func (h *TerritoryHandler) errorResponse(c *gin.Context, statusCode int, errorType, details string) {
	c.JSON(statusCode, TerritoryResponse{
		Success: false,
		Code:    statusCode,
		Message: details,
		Error: &TerritoryErrorInfo{
			Type:    errorType,
			Details: details,
		},
		Meta: TerritoryMeta{
			RequestID: h.generateRequestID(),
			Timestamp: time.Now().Format(time.RFC3339),
		},
	})
}
