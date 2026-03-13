package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/service"
)

type BNCConnectorHandler struct {
	connectorService *service.BNCConnectorService
}

func NewBNCConnectorHandler(connectorService *service.BNCConnectorService) *BNCConnectorHandler {
	return &BNCConnectorHandler{connectorService: connectorService}
}

func (h *BNCConnectorHandler) CreateAccessToken(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeBNCSNAPResponse(c, http.StatusBadRequest, "4007300", "Invalid request body")
		return
	}

	resp, err := h.connectorService.IssueAccessToken(c.Request.Context(), c.Request.Header, body)
	if err != nil {
		h.handleError(c, err, "5007300", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *BNCConnectorHandler) HandleTransferNotify(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeBNCSNAPResponse(c, http.StatusBadRequest, "4002500", "Invalid request body")
		return
	}

	if err := h.connectorService.HandleTransferNotify(c.Request.Context(), c.Request.Header, c.Request.URL.Path, body); err != nil {
		h.handleError(c, err, "5002500", "Internal server error")
		return
	}

	writeBNCSNAPResponse(c, http.StatusOK, "2002500", "Successful")
}

func (h *BNCConnectorHandler) handleError(c *gin.Context, err error, fallbackCode, fallbackMessage string) {
	var connectorErr *service.BNCConnectorError
	if errors.As(err, &connectorErr) {
		writeBNCSNAPResponse(c, connectorErr.HTTPStatus, connectorErr.ResponseCode, connectorErr.ResponseMessage)
		return
	}

	writeBNCSNAPResponse(c, http.StatusInternalServerError, fallbackCode, fallbackMessage)
}

func writeBNCSNAPResponse(c *gin.Context, httpStatus int, responseCode, responseMessage string) {
	c.JSON(httpStatus, service.BNCAckResponse{
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
	})
}
