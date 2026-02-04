package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// FaceCompareHandler handles face comparison endpoints
type FaceCompareHandler struct {
	svc *service.FaceCompareService
}

// NewFaceCompareHandler creates a new face compare handler
func NewFaceCompareHandler(svc *service.FaceCompareService) *FaceCompareHandler {
	return &FaceCompareHandler{svc: svc}
}

// CompareFaces handles POST /v1/identity/compare
// @Summary Compare two faces
// @Description Compare two faces using AWS Rekognition CompareFaces
// @Tags Identity - Face Compare
// @Accept json,multipart/form-data
// @Produce json
// @Param request body models.FaceCompareRequest true "Face comparison request (JSON)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /v1/identity/compare [post]
func (h *FaceCompareHandler) CompareFaces(c *gin.Context) {
	requestID := "req_cmp_" + utils.GenerateRandomString(12)
	startTime := time.Now()

	// Get client from context
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, http.StatusUnauthorized, "UNAUTHORIZED", "Client not authenticated", "")
		return
	}

	// Parse request based on content type
	var sourceData, targetData []byte
	var sourceType, targetType, sourceURL, targetURL string
	var threshold *float64
	var err error

	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Multipart form data - file upload
		sourceData, targetData, threshold, err = h.parseMultipartRequest(c)
		if err != nil {
			h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), "")
			return
		}
		sourceType = "upload"
		targetType = "upload"
		sourceURL = "uploaded"
		targetURL = "uploaded"
	} else {
		// JSON request - S3 URLs
		var req models.FaceCompareRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: source and target are required", "")
			return
		}

		// Download images from S3 URLs
		sourceData, err = h.svc.DownloadFromS3URL(c.Request.Context(), req.Source)
		if err != nil {
			if fcErr, ok := err.(*service.FaceCompareError); ok {
				h.errorResponse(c, requestID, http.StatusBadRequest, fcErr.Code, fcErr.Message, "source")
				return
			}
			h.errorResponse(c, requestID, http.StatusBadRequest, models.FaceCompareErrS3AccessError, "Cannot access source URL", "source")
			return
		}

		targetData, err = h.svc.DownloadFromS3URL(c.Request.Context(), req.Target)
		if err != nil {
			if fcErr, ok := err.(*service.FaceCompareError); ok {
				h.errorResponse(c, requestID, http.StatusBadRequest, fcErr.Code, fcErr.Message, "target")
				return
			}
			h.errorResponse(c, requestID, http.StatusBadRequest, models.FaceCompareErrS3AccessError, "Cannot access target URL", "target")
			return
		}

		sourceType = "url"
		targetType = "url"
		sourceURL = req.Source
		targetURL = req.Target
		threshold = req.SimilarityThreshold
	}

	// Validate image data
	if len(sourceData) == 0 {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "Source image is required", "source")
		return
	}
	if len(targetData) == 0 {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "Target image is required", "target")
		return
	}

	// Validate threshold if provided
	if threshold != nil {
		if *threshold < 0 || *threshold > 100 {
			h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "similarityThreshold must be between 0 and 100", "similarityThreshold")
			return
		}
	}

	// Compare faces
	result, err := h.svc.CompareFaces(
		c.Request.Context(),
		client.ID,
		sourceData,
		targetData,
		sourceType,
		targetType,
		sourceURL,
		targetURL,
		threshold,
	)
	if err != nil {
		// Check if it's a FaceCompareError
		if fcErr, ok := err.(*service.FaceCompareError); ok {
			field := ""
			if strings.Contains(fcErr.Message, "source") || strings.Contains(fcErr.Message, "Source") {
				field = "source"
			} else if strings.Contains(fcErr.Message, "target") || strings.Contains(fcErr.Message, "Target") {
				field = "target"
			}
			h.errorResponse(c, requestID, http.StatusBadRequest, fcErr.Code, fcErr.Message, field)
			return
		}
		h.errorResponse(c, requestID, http.StatusInternalServerError, models.FaceCompareErrAWSServiceError, err.Error(), "")
		return
	}

	// Success response
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Face comparison successful",
		"data":    result,
		"meta": gin.H{
			"requestId":        requestID,
			"timestamp":        time.Now().Format(time.RFC3339),
			"processingTimeMs": time.Since(startTime).Milliseconds(),
		},
	})
}

// GetCompareByID handles GET /v1/identity/compare/:id
// @Summary Get face comparison result by ID
// @Description Retrieve a face comparison result by its ID
// @Tags Identity - Face Compare
// @Produce json
// @Param id path string true "Face comparison ID (UUID)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/identity/compare/{id} [get]
func (h *FaceCompareHandler) GetCompareByID(c *gin.Context) {
	requestID := "req_get_cmp_" + utils.GenerateRandomString(12)

	// Get client from context
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, http.StatusUnauthorized, "UNAUTHORIZED", "Client not authenticated", "")
		return
	}

	id := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(id); err != nil {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "ID must be a valid UUID format", "id")
		return
	}

	// Get record
	record, err := h.svc.GetByID(c.Request.Context(), id, client.ID)
	if err != nil {
		h.errorResponse(c, requestID, http.StatusNotFound, "NOT_FOUND", "Face comparison record not found", "")
		return
	}

	if record == nil {
		h.errorResponse(c, requestID, http.StatusNotFound, "NOT_FOUND", "No face comparison record found with id: "+id, "")
		return
	}

	// Build response
	response := models.FaceCompareResponse{
		ID:         record.ID,
		Matched:    record.Matched,
		Similarity: record.Similarity,
		Threshold:  record.Threshold,
		Source: models.FaceDetailResp{
			Detected:    record.SourceDetected,
			Confidence:  getFloatPtrValue(record.SourceConfidence),
			BoundingBox: record.SourceBoundingBox,
		},
		Target: models.FaceDetailResp{
			Detected:    record.TargetDetected,
			Confidence:  getFloatPtrValue(record.TargetConfidence),
			BoundingBox: record.TargetBoundingBox,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Successfully retrieved face comparison data",
		"data":    response,
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
			"createdAt": record.CreatedAt.Format(time.RFC3339),
		},
	})
}

// parseMultipartRequest parses multipart form data request
func (h *FaceCompareHandler) parseMultipartRequest(c *gin.Context) ([]byte, []byte, *float64, error) {
	// Get source file
	sourceFile, _, err := c.Request.FormFile("source")
	if err != nil {
		return nil, nil, nil, err
	}
	defer sourceFile.Close()

	sourceData, err := io.ReadAll(sourceFile)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get target file
	targetFile, _, err := c.Request.FormFile("target")
	if err != nil {
		return nil, nil, nil, err
	}
	defer targetFile.Close()

	targetData, err := io.ReadAll(targetFile)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get optional threshold
	var threshold *float64
	if thresholdStr := c.PostForm("similarityThreshold"); thresholdStr != "" {
		t, err := strconv.ParseFloat(thresholdStr, 64)
		if err == nil {
			threshold = &t
		}
	}

	return sourceData, targetData, threshold, nil
}

// errorResponse sends an error response
func (h *FaceCompareHandler) errorResponse(c *gin.Context, requestID string, code int, errCode, message, field string) {
	response := gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"error": gin.H{
			"code":   errCode,
			"detail": message,
		},
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	if field != "" {
		response["error"].(gin.H)["field"] = field
	}

	c.JSON(code, response)
}

// getFloatPtrValue returns the value of a float64 pointer or 0 if nil
func getFloatPtrValue(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
