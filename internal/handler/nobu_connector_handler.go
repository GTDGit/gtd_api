package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/service"
)

type NobuConnectorHandler struct {
	connectorService *service.NobuConnectorService
}

func NewNobuConnectorHandler(connectorService *service.NobuConnectorService) *NobuConnectorHandler {
	return &NobuConnectorHandler{connectorService: connectorService}
}

// CreateAccessToken issues a B2B token to Nobu (Service: get-token b2b).
func (h *NobuConnectorHandler) CreateAccessToken(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeNobuSNAPResponse(c, http.StatusBadRequest, "4007300", "Invalid request body")
		return
	}

	resp, err := h.connectorService.IssueAccessToken(c.Request.Context(), c.Request.Header, body)
	if err != nil {
		h.handleError(c, err, "5007300", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, resp)
}

// HandleNotify receives Service 52 QRIS payment notifications from Nobu.
func (h *NobuConnectorHandler) HandleNotify(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeNobuSNAPResponse(c, http.StatusBadRequest, "4005200", "Invalid request body")
		return
	}

	if err := h.connectorService.HandleNotify(c.Request.Context(), c.Request.Header, c.Request.URL.Path, body); err != nil {
		h.handleError(c, err, "5005200", "Internal server error")
		return
	}

	writeNobuSNAPResponse(c, http.StatusOK, "2005200", "Request has been processed successfully")
}

func (h *NobuConnectorHandler) handleError(c *gin.Context, err error, fallbackCode, fallbackMessage string) {
	var connectorErr *service.NobuConnectorError
	if errors.As(err, &connectorErr) {
		writeNobuSNAPResponse(c, connectorErr.HTTPStatus, connectorErr.ResponseCode, connectorErr.ResponseMessage)
		return
	}
	writeNobuSNAPResponse(c, http.StatusInternalServerError, fallbackCode, fallbackMessage)
}

func writeNobuSNAPResponse(c *gin.Context, httpStatus int, responseCode, responseMessage string) {
	c.JSON(httpStatus, service.NobuAckResponse{
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
		AdditionalInfo:  map[string]any{},
	})
}
