package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

// LivenessHandler handles liveness detection endpoints
type LivenessHandler struct {
	svc *service.LivenessService
}

// NewLivenessHandler creates a new liveness handler
func NewLivenessHandler(svc *service.LivenessService) *LivenessHandler {
	return &LivenessHandler{svc: svc}
}

// CreateSession creates a new liveness session
// @Summary Create liveness session
// @Description Create a new liveness detection session
// @Tags Identity - Liveness
// @Accept json
// @Produce json
// @Param request body models.CreateSessionRequest true "Create session request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /v1/identity/liveness/session [post]
func (h *LivenessHandler) CreateSession(c *gin.Context) {
	requestID := "req_sess_" + utils.GenerateRandomString(12)
	startTime := time.Now()

	// Get client from context
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, http.StatusUnauthorized, "UNAUTHORIZED", "Client not authenticated")
		return
	}

	// Parse request
	var req models.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "NIK is required and must be 16 digits")
		return
	}

	// Validate NIK format
	if len(req.NIK) != 16 {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "NIK must be 16 digits")
		return
	}

	// Validate method if provided
	method := req.Method
	// Default to generic passive logic (handled by AWS)
	if method == "" {
		method = models.LivenessMethodPassive
	}

	// Create session
	result, err := h.svc.CreateSession(c.Request.Context(), client.ID, req.NIK, method, req.RedirectURL)
	if err != nil {
		h.errorResponse(c, requestID, http.StatusInternalServerError, "SESSION_CREATE_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Liveness session created",
		"data":    result,
		"meta": gin.H{
			"requestId":        requestID,
			"timestamp":        time.Now().Format(time.RFC3339),
			"processingTimeMs": time.Since(startTime).Milliseconds(),
		},
	})
}

// VerifyLiveness verifies liveness from submitted frames
// @Summary Verify liveness
// @Description Submit frames for liveness verification
// @Tags Identity - Liveness
// @Accept json
// @Produce json
// @Param request body models.VerifyLivenessRequest true "Verify liveness request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /v1/identity/liveness/verify [post]
func (h *LivenessHandler) VerifyLiveness(c *gin.Context) {
	requestID := "req_liv_" + utils.GenerateRandomString(12)
	startTime := time.Now()

	// Get client from context
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, http.StatusUnauthorized, "UNAUTHORIZED", "Client not authenticated")
		return
	}

	// Parse request
	var req models.VerifyLivenessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	// Validate session ID
	if req.SessionID == "" {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "sessionId is required")
		return
	}

	// For AWS flow, we utilize the SessionID to retrieve results.
	// The frames are processed by AWS Rekognition.

	// Verify liveness
	result, err := h.svc.VerifyLiveness(c.Request.Context(), client.ID, &req)
	if err != nil {
		// Check if it's a liveness error
		if livErr, ok := err.(*service.LivenessError); ok {
			// Get session info for response
			session, _ := h.svc.GetSession(c.Request.Context(), req.SessionID, client.ID)
			h.livenessFailedResponse(c, requestID, session, livErr.Code, livErr.Message, time.Since(startTime).Milliseconds())
			return
		}
		h.errorResponse(c, requestID, http.StatusInternalServerError, "LIVENESS_ERROR", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Liveness verification successful",
		"data":    result,
		"meta": gin.H{
			"requestId":        requestID,
			"timestamp":        time.Now().Format(time.RFC3339),
			"processingTimeMs": time.Since(startTime).Milliseconds(),
		},
	})
}

// GetSession gets liveness session details
// @Summary Get liveness session
// @Description Get liveness session details by session ID
// @Tags Identity - Liveness
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /v1/identity/liveness/session/{sessionId} [get]
func (h *LivenessHandler) GetSession(c *gin.Context) {
	requestID := "req_get_sess_" + utils.GenerateRandomString(12)

	// Get client from context
	client := middleware.GetClient(c)
	if client == nil {
		h.errorResponse(c, requestID, http.StatusUnauthorized, "UNAUTHORIZED", "Client not authenticated")
		return
	}

	sessionID := c.Param("sessionId")
	if sessionID == "" {
		h.errorResponse(c, requestID, http.StatusBadRequest, "VALIDATION_ERROR", "Session ID is required")
		return
	}

	// Get session
	session, err := h.svc.GetSession(c.Request.Context(), sessionID, client.ID)
	if err != nil {
		h.errorResponse(c, requestID, http.StatusNotFound, "NOT_FOUND", "Session not found")
		return
	}

	// Build response
	response := gin.H{
		"sessionId": session.SessionID,
		"nik":       session.NIK,
		"method":    session.Method,
		"status":    session.Status,
		"expiresAt": session.ExpiresAt.Format(time.RFC3339),
		"createdAt": session.CreatedAt.Format(time.RFC3339),
	}

	if len(session.Challenges) > 0 {
		response["challenges"] = session.Challenges
	}

	if session.IsLive != nil {
		response["isLive"] = *session.IsLive
	}

	if session.Confidence != nil {
		response["confidence"] = *session.Confidence
	}

	if session.FaceURL != "" {
		response["file"] = gin.H{"face": session.FaceURL}
	}

	if session.CompletedAt != nil {
		response["completedAt"] = session.CompletedAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Session retrieved successfully",
		"data":    response,
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// errorResponse sends an error response
func (h *LivenessHandler) errorResponse(c *gin.Context, requestID string, code int, errType, message string) {
	c.JSON(code, gin.H{
		"success": false,
		"code":    code,
		"message": message,
		"error": gin.H{
			"code":   errType,
			"detail": message,
		},
		"meta": gin.H{
			"requestId": requestID,
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// livenessFailedResponse sends a liveness failed response
func (h *LivenessHandler) livenessFailedResponse(c *gin.Context, requestID string, session *models.LivenessSession, errorCode, reason string, processingTime int64) {
	response := gin.H{
		"sessionId":     "",
		"nik":           "",
		"method":        "passive",
		"isLive":        false,
		"confidence":    0,
		"failureReason": reason,
		"errorCode":     errorCode,
	}

	if session != nil {
		response["sessionId"] = session.SessionID
		response["nik"] = session.NIK
		response["method"] = session.Method
		if session.Confidence != nil {
			response["confidence"] = *session.Confidence
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"code":    200,
		"message": "Liveness verification failed",
		"data":    response,
		"meta": gin.H{
			"requestId":        requestID,
			"timestamp":        time.Now().Format(time.RFC3339),
			"processingTimeMs": processingTime,
		},
	})
}
