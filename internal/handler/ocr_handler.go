package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// OCRHandler handles OCR endpoints
type OCRHandler struct {
	ocrService *service.OCRService
}

// NewOCRHandler creates a new OCR handler
func NewOCRHandler(ocrService *service.OCRService) *OCRHandler {
	return &OCRHandler{ocrService: ocrService}
}

// OCRResponse represents the standard OCR API response
type OCRResponse struct {
	Success    bool                  `json:"success"`
	Code       int                   `json:"code"`
	Message    string                `json:"message"`
	Data       interface{}           `json:"data"`
	Validation *models.OCRValidation `json:"validation,omitempty"`
	Meta       models.OCRMeta        `json:"meta"`
}

// OCRErrorResponse represents error response with quality details
type OCRErrorResponse struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Error   interface{} `json:"error"`
	Meta    struct {
		RequestID string `json:"requestId"`
		Timestamp string `json:"timestamp"`
	} `json:"meta"`
}

// KTPOCR handles POST /v1/identity/ocr/ktp
// @Summary Extract data from KTP image
// @Tags Identity OCR
// @Accept json,multipart/form-data
// @Produce json
// @Param image body string true "Base64 encoded image or file upload"
// @Param validateQuality query bool false "Validate image quality"
// @Param validateNik query bool false "Validate NIK against database"
// @Success 200 {object} OCRResponse
// @Failure 400 {object} OCRErrorResponse
// @Failure 422 {object} OCRErrorResponse
// @Router /v1/identity/ocr/ktp [post]
func (h *OCRHandler) KTPOCR(c *gin.Context) {
	requestID := h.generateRequestID("ktp")
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, 401, "AUTHENTICATION_ERROR", "Invalid authentication")
		return
	}

	// Parse request
	imageData, validateQuality, validateNik, err := h.parseOCRRequest(c)
	if err != nil {
		h.errorResponse(c, requestID, 400, "VALIDATION_ERROR", err.Error())
		return
	}

	// Process OCR
	result, err := h.ocrService.ProcessKTPOCR(c.Request.Context(), client.ID, imageData, validateQuality, validateNik)
	if err != nil {
		h.handleOCRError(c, requestID, err)
		return
	}

	// Success response
	c.JSON(http.StatusOK, OCRResponse{
		Success:    true,
		Code:       200,
		Message:    "Successfully extracted KTP data",
		Data:       result.Data,
		Validation: result.Validation,
		Meta: models.OCRMeta{
			RequestID:          requestID,
			Timestamp:          time.Now().Format(time.RFC3339),
			ProcessingTimeMs:   result.ProcessingTime,
			ExtractionEngine:   "google-vision",
			VerificationEngine: "groq-llama-3.2-90b-text",
		},
	})
}

// NPWPOCR handles POST /v1/identity/ocr/npwp
// @Summary Extract data from NPWP image
// @Tags Identity OCR
// @Accept json,multipart/form-data
// @Produce json
// @Param image body string true "Base64 encoded image or file upload"
// @Param validateQuality query bool false "Validate image quality"
// @Success 200 {object} OCRResponse
// @Failure 400 {object} OCRErrorResponse
// @Failure 422 {object} OCRErrorResponse
// @Router /v1/identity/ocr/npwp [post]
func (h *OCRHandler) NPWPOCR(c *gin.Context) {
	requestID := h.generateRequestID("npwp")
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, 401, "AUTHENTICATION_ERROR", "Invalid authentication")
		return
	}

	// Parse request
	imageData, validateQuality, _, err := h.parseOCRRequest(c)
	if err != nil {
		h.errorResponse(c, requestID, 400, "VALIDATION_ERROR", err.Error())
		return
	}

	// Process OCR
	result, err := h.ocrService.ProcessNPWPOCR(c.Request.Context(), client.ID, imageData, validateQuality)
	if err != nil {
		h.handleOCRError(c, requestID, err)
		return
	}

	// Success response
	c.JSON(http.StatusOK, OCRResponse{
		Success:    true,
		Code:       200,
		Message:    "Successfully extracted NPWP data",
		Data:       result.Data,
		Validation: nil,
		Meta: models.OCRMeta{
			RequestID:          requestID,
			Timestamp:          time.Now().Format(time.RFC3339),
			ProcessingTimeMs:   result.ProcessingTime,
			ExtractionEngine:   "google-vision",
			VerificationEngine: "groq-llama-3.2-90b-text",
		},
	})
}

// SIMOCR handles POST /v1/identity/ocr/sim
// @Summary Extract data from SIM image
// @Tags Identity OCR
// @Accept json,multipart/form-data
// @Produce json
// @Param image body string true "Base64 encoded image or file upload"
// @Param validateQuality query bool false "Validate image quality"
// @Success 200 {object} OCRResponse
// @Failure 400 {object} OCRErrorResponse
// @Failure 422 {object} OCRErrorResponse
// @Router /v1/identity/ocr/sim [post]
func (h *OCRHandler) SIMOCR(c *gin.Context) {
	requestID := h.generateRequestID("sim")
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, 401, "AUTHENTICATION_ERROR", "Invalid authentication")
		return
	}

	// Parse request
	imageData, validateQuality, _, err := h.parseOCRRequest(c)
	if err != nil {
		h.errorResponse(c, requestID, 400, "VALIDATION_ERROR", err.Error())
		return
	}

	// Process OCR
	result, err := h.ocrService.ProcessSIMOCR(c.Request.Context(), client.ID, imageData, validateQuality)
	if err != nil {
		h.handleOCRError(c, requestID, err)
		return
	}

	// Success response
	c.JSON(http.StatusOK, OCRResponse{
		Success:    true,
		Code:       200,
		Message:    "Successfully extracted SIM data",
		Data:       result.Data,
		Validation: nil,
		Meta: models.OCRMeta{
			RequestID:          requestID,
			Timestamp:          time.Now().Format(time.RFC3339),
			ProcessingTimeMs:   result.ProcessingTime,
			ExtractionEngine:   "google-vision",
			VerificationEngine: "groq-llama-3.2-90b-text",
		},
	})
}

// GetOCRByID handles GET /v1/identity/ocr/:id
// @Summary Get OCR result by ID
// @Tags Identity OCR
// @Produce json
// @Param id path string true "OCR record ID (UUID)"
// @Success 200 {object} OCRResponse
// @Failure 400 {object} OCRErrorResponse
// @Failure 404 {object} OCRErrorResponse
// @Router /v1/identity/ocr/{id} [get]
func (h *OCRHandler) GetOCRByID(c *gin.Context) {
	requestID := h.generateRequestID("get_ocr")
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, 401, "AUTHENTICATION_ERROR", "Invalid authentication")
		return
	}

	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		h.errorResponse(c, requestID, 400, "VALIDATION_ERROR", "ID must be a valid UUID format")
		return
	}

	// Get record
	record, err := h.ocrService.GetOCRByID(c.Request.Context(), id, client.ID)
	if err != nil {
		h.errorResponse(c, requestID, 500, "INTERNAL_ERROR", "Failed to retrieve OCR record")
		return
	}

	if record == nil {
		h.errorResponse(c, requestID, 404, "NOT_FOUND", "No OCR record found with id: "+id)
		return
	}

	// Build response based on document type
	var data interface{}
	switch record.DocType {
	case models.DocTypeKTP:
		data = h.buildKTPResponse(record)
	case models.DocTypeNPWP:
		data = h.buildNPWPResponse(record)
	case models.DocTypeSIM:
		data = h.buildSIMResponse(record)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Successfully retrieved OCR data",
		"data":    data,
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
			"createdAt": record.CreatedAt.Format(time.RFC3339),
		},
	})
}

// parseOCRRequest parses OCR request from JSON or multipart
func (h *OCRHandler) parseOCRRequest(c *gin.Context) ([]byte, bool, bool, error) {
	contentType := c.GetHeader("Content-Type")

	var imageData []byte
	var validateQuality, validateNik bool

	if contentType == "application/json" || contentType == "" {
		// JSON request
		var req models.OCRRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return nil, false, false, err
		}

		// Decode base64 image
		decoded, err := service.DecodeBase64Image(req.Image)
		if err != nil {
			return nil, false, false, err
		}
		imageData = decoded
		validateQuality = req.ValidateQuality
		validateNik = req.ValidateNik

	} else {
		// Multipart request
		file, _, err := c.Request.FormFile("image")
		if err != nil {
			return nil, false, false, err
		}
		defer file.Close()

		imageData, err = io.ReadAll(file)
		if err != nil {
			return nil, false, false, err
		}

		validateQuality = c.PostForm("validateQuality") == "true"
		validateNik = c.PostForm("validateNik") == "true"
	}

	if len(imageData) == 0 {
		return nil, false, false, errImageRequired
	}

	return imageData, validateQuality, validateNik, nil
}

var errImageRequired = &validationError{message: "image is required"}

type validationError struct {
	message string
}

func (e *validationError) Error() string {
	return e.message
}

// handleOCRError handles OCR processing errors
func (h *OCRHandler) handleOCRError(c *gin.Context, requestID string, err error) {
	// Check if it's a quality error
	if qErr, ok := err.(*service.QualityError); ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"code":    422,
			"message": "Image quality validation failed",
			"error": gin.H{
				"type":    "QUALITY_VALIDATION_ERROR",
				"details": qErr.Details,
				"quality": qErr.Quality,
			},
			"meta": gin.H{
				"requestId": requestID,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		})
		return
	}

	// Check if it's a document type error
	if dtErr, ok := err.(*service.DocumentTypeError); ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    400,
			"message": dtErr.Message,
			"error": gin.H{
				"type":     "INVALID_DOCUMENT_TYPE",
				"expected": dtErr.Expected,
				"detected": dtErr.Detected,
				"details":  []string{dtErr.Message},
			},
			"meta": gin.H{
				"requestId": requestID,
				"timestamp": time.Now().Format(time.RFC3339),
			},
		})
		return
	}

	// Check error message for specific error types
	errMsg := err.Error()
	var code int
	var errType string

	switch {
	case contains(errMsg, "invalid image format"):
		code = 400
		errType = "INVALID_IMAGE_FORMAT"
	case contains(errMsg, "size exceeds"):
		code = 413
		errType = "IMAGE_TOO_LARGE"
	case contains(errMsg, "no text detected"):
		code = 422
		errType = "DOCUMENT_NOT_DETECTED"
	case contains(errMsg, "Invalid document type"):
		code = 400
		errType = "INVALID_DOCUMENT_TYPE"
	case contains(errMsg, "text extraction failed"):
		code = 503
		errType = "AI_SERVICE_ERROR"
	case contains(errMsg, "text parsing failed"):
		code = 503
		errType = "AI_SERVICE_ERROR"
	default:
		code = 500
		errType = "INTERNAL_ERROR"
	}

	h.errorResponse(c, requestID, code, errType, errMsg)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// errorResponse sends error response
func (h *OCRHandler) errorResponse(c *gin.Context, requestID string, code int, errType, message string) {
	c.JSON(code, gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"error": gin.H{
			"type":    errType,
			"details": []string{message},
		},
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// generateRequestID generates a unique request ID
func (h *OCRHandler) generateRequestID(prefix string) string {
	return fmt.Sprintf("req_%s_%s", prefix, utils.GenerateRandomString(12))
}

// buildKTPResponse builds KTP response from record
func (h *OCRHandler) buildKTPResponse(record *models.OCRRecord) interface{} {
	resp := gin.H{
		"id":       record.ID,
		"docType":  record.DocType,
		"fullName": record.FullName,
	}

	if record.NIK != nil {
		resp["nik"] = *record.NIK
	}
	if record.PlaceOfBirth != nil {
		resp["placeOfBirth"] = *record.PlaceOfBirth
	}
	if record.DateOfBirth != nil {
		resp["dateOfBirth"] = *record.DateOfBirth
	}
	if record.Gender != nil {
		resp["gender"] = *record.Gender
	}
	if record.BloodType != nil {
		resp["bloodType"] = *record.BloodType
	}
	if record.Address != nil {
		resp["address"] = *record.Address
	}
	if record.AdministrativeCode != nil {
		resp["administrativeCode"] = *record.AdministrativeCode
	}
	if record.Religion != nil {
		resp["religion"] = *record.Religion
	}
	if record.MaritalStatus != nil {
		resp["maritalStatus"] = *record.MaritalStatus
	}
	if record.Occupation != nil {
		resp["occupation"] = *record.Occupation
	}
	if record.Nationality != nil {
		resp["nationality"] = *record.Nationality
	}
	if record.ValidUntil != nil {
		resp["validUntil"] = *record.ValidUntil
	}
	if record.PublishedIn != nil {
		resp["publishedIn"] = *record.PublishedIn
	}
	if record.PublishedOn != nil {
		resp["publishedOn"] = *record.PublishedOn
	}
	if record.FileURLs != nil {
		resp["file"] = *record.FileURLs
	}

	return resp
}

// buildNPWPResponse builds NPWP response from record
func (h *OCRHandler) buildNPWPResponse(record *models.OCRRecord) interface{} {
	resp := gin.H{
		"id":       record.ID,
		"docType":  record.DocType,
		"fullName": record.FullName,
	}

	if record.NPWP != nil {
		resp["npwp"] = *record.NPWP
	}
	if record.NPWPRaw != nil {
		resp["npwpRaw"] = *record.NPWPRaw
	}
	if record.NIK != nil {
		resp["nik"] = *record.NIK
	}
	if record.NPWPFormat != nil {
		resp["format"] = *record.NPWPFormat
	}
	if record.TaxPayerType != nil {
		resp["taxPayerType"] = *record.TaxPayerType
	}
	if record.Address != nil {
		resp["address"] = *record.Address
	}
	if record.AdministrativeCode != nil {
		resp["administrativeCode"] = *record.AdministrativeCode
	}
	if record.PublishedIn != nil {
		resp["publishedIn"] = *record.PublishedIn
	}
	if record.PublishedOn != nil {
		resp["publishedOn"] = *record.PublishedOn
	}
	if record.FileURLs != nil {
		resp["file"] = *record.FileURLs
	}

	return resp
}

// buildSIMResponse builds SIM response from record
func (h *OCRHandler) buildSIMResponse(record *models.OCRRecord) interface{} {
	resp := gin.H{
		"id":       record.ID,
		"docType":  record.DocType,
		"fullName": record.FullName,
	}

	if record.SIMNumber != nil {
		resp["simNumber"] = *record.SIMNumber
	}
	if record.PlaceOfBirth != nil {
		resp["placeOfBirth"] = *record.PlaceOfBirth
	}
	if record.DateOfBirth != nil {
		resp["dateOfBirth"] = *record.DateOfBirth
	}
	if record.Gender != nil {
		resp["gender"] = *record.Gender
	}
	if record.BloodType != nil {
		resp["bloodType"] = *record.BloodType
	}
	if record.Height != nil {
		resp["height"] = *record.Height
	}
	if record.Address != nil {
		resp["address"] = *record.Address
	}
	if record.AdministrativeCode != nil {
		resp["administrativeCode"] = *record.AdministrativeCode
	}
	if record.Occupation != nil {
		resp["occupation"] = *record.Occupation
	}
	if record.SIMType != nil {
		resp["type"] = *record.SIMType
	}
	if record.ValidFrom != nil {
		resp["validFrom"] = *record.ValidFrom
	}
	if record.ValidUntil != nil {
		resp["validUntil"] = *record.ValidUntil
	}
	if record.Publisher != nil {
		resp["publisher"] = *record.Publisher
	}
	if record.FileURLs != nil {
		resp["file"] = *record.FileURLs
	}

	return resp
}
